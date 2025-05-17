package autolearn

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/solution"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	resourceSenseClient "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg"
)

func (ctr *Controller) GetAndUpdateRecommendAlgorithm(autoLearn *types.AutoLearn, rv *types.AutoLearnRevision) error {
	// 高级模式且非优化版、重试版均已有推荐算法，其他版需要获取
	d := dao.NewDAO(ctr.MongoClient, autoLearn.TenantID)

	if rv.IsOptimized || rv.CreateMode == types.CreateModeAdvance || rv.SolutionType == types.SolutionTypeCustom {
		return nil
	}

	eventCode := types.StateEventCodeNothing
	t := types.StateEventTypeRecommend
	event := &types.StateEvent{T: t, A: types.EventActionDoing}
	if err := ctr.StateMachine.Action(d, event, rv.RevisionID, eventCode); err != nil {
		return err
	}

	logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, rv.RevisionID))
	logs.Info("step5: start to get and update recommend algorithms")

	basicInfo := &dao.BasicInfo{
		Project:     autoLearn.ProjectID,
		AutoLearnID: rv.AutoLearnID,
		RevisionID:  rv.RevisionID,
	}

	req := &types.RecommendSolutionRequest{
		Type:        rv.Type,
		TimeLimit:   rv.TimeLimit,
		WorkerNum:   rv.Resource.WorkerNum,
		SpeedMetric: rv.SpeedMetric,
		ModelType:   rv.ModelType,
	}

	if rv.Continued && rv.ContinuedMeta != nil {
		req.SolutionName = rv.ContinuedMeta.SolutionName
	}

	algorithms, err := GetRecommendAlgorithm(rv.Datasets, req)
	if algorithms == nil || len(algorithms) < int(rv.Resource.WorkerNum) {
		eventCode = types.StateEventCodeCustom
		if len(algorithms) < int(rv.Resource.WorkerNum) && err.Error() == "" {
			eventCode = types.StateEventCodeRecommendFailedByNotFulfill
			err = errors.ErrorGetAlgorithmFailedByNotFulfill
		}
		d := dao.NewDAO(ctr.MongoClient, autoLearn.TenantID)
		event.A = types.EventActionFailed
		if err := ctr.StateMachine.Action(d, event, rv.RevisionID, eventCode, err.Error()); err != nil {
			return err
		}

		utils.MetricEventDump(basicInfo.AutoLearnID,
			basicInfo.RevisionID,
			rv.Type,
			config.MetricAutoLearnFailedReason, map[string]any{
				utils.LogFieldFailedReason: types.EventCodeMessage[eventCode],
				utils.LogFieldRvCreatedAt:  time.Unix(rv.CreatedAt, 0).Format(time.RFC3339),
			})

		logs.WithError(err).Error("step6: preparatory work ended due to get recommend algorithms failed")
		return err
	}

	// 简单模式需要填充 algorithm.actualAlgorithms
	for i := 0; i < int(rv.Resource.WorkerNum); i++ {
		define := algorithms[i].HyperParamDefine
		if define == nil || define.Properties == nil {
			continue
		}

		for _, hyperParm := range algorithms[i].HyperParam {
			if v, ok := define.Properties[hyperParm.SnapdetKey]; ok {
				if d, ok := v["description"].(string); ok {
					hyperParm.Description = d
				}

				if t, ok := v["title"].(string); ok {
					hyperParm.Key = t
				}

				hyperParm.Value = v["default"]
			}
		}
	}

	apuConfigs, err := resourceSenseClient.NewRSClient(config.Profile.AisEndPoint.Resourcesense).GetClusterAPU()
	if err != nil {
		log.WithError(err).Error("failed to GetClusterAPU")
		return err
	}

	// 更新每个算法的 proposedResouces
	for index, alg := range algorithms {
		if index >= int(rv.Resource.WorkerNum) {
			break
		}
		query := map[string]any{}
		for _, hy := range alg.HyperParam {
			if hy.SnapdetKey == types.SnapxFinetuningKey {
				query[types.SnapxHyperParamsKey] = fmt.Sprintf("%s=%s", types.SnapxFinetuningKey, hy.Value)
			}
		}
		log.Infof("solution resource request: %s %+v", alg.Name, query)
		resourceClient := solution.NewSolutionClient(config.Profile.AisEndPoint.SolutionHub)
		solutionResource, err := resourceClient.GetSolutionResources(alg.Name, query)
		if err != nil {
			logs.WithError(err).Error("get solution resource failed")
			return err
		}
		if features.IsQuotaSupportGPUGroupEnabled() {
			solutionResource = gpuOptionsFilter(rv.GPUTypes, apuConfigs, solutionResource)
		}
		alg.ProposedResource = proposedResources(solutionResource)
		if alg.ProposedResource.InferRequestsResources.GPU == 0 || alg.ProposedResource.TrainRequestsResources.GPU == 0 {
			eventCode = types.StateEventCodeGPUNotMatched
			d := dao.NewDAO(ctr.MongoClient, autoLearn.TenantID)
			event.A = types.EventActionFailed
			if err := ctr.StateMachine.Action(d, event, rv.RevisionID, eventCode, errors.ErrorAPUTypeNotMatch.Error()); err != nil {
				return err
			}
			log.Infof("ProposedResource: %+v", alg.ProposedResource)
			return errors.ErrorAPUTypeNotMatch
		}
	}

	if err := dao.UpdateAlgorithm(ctr.MongoClient, autoLearn.TenantID,
		&dao.BasicInfo{
			Project:     autoLearn.ProjectID,
			AutoLearnID: autoLearn.AutoLearnID,
			RevisionID:  rv.RevisionID,
		},
		&types.AutoLearnAlgorithm{
			AutoLearnID:        autoLearn.AutoLearnID,
			RevisionID:         rv.RevisionID,
			ActualAlgorithms:   algorithms[:rv.Resource.WorkerNum],
			OriginalAlgorithms: algorithms,
		}); err != nil {
		logs.WithError(err).Error("step6: preparatory work ended due to update algorithms failed")
		return err
	}

	event.A = types.EventActionSuccess
	if err := ctr.StateMachine.Action(d, event, rv.RevisionID, eventCode); err != nil {
		return err
	}

	return nil
}

func GetRecommendAlgorithm(datasets []*types.Dataset, reqParams *types.RecommendSolutionRequest) ([]*types.Algorithm, error) {
	solutionhubRecommendBody := makeRecommendRequestBody(datasets)
	utils.MetricEventDump("", "", reqParams.Type, config.MetricRecommendRequest, nil)
	algorithms, err := client.GetRecommendAlgorithm(&client.RecommendAlgorithmReq{
		TrainType:                reqParams.Type,
		SolutionNum:              reqParams.WorkerNum,
		SpeedMetric:              reqParams.SpeedMetric,
		ModelType:                reqParams.ModelType,
		SolutionName:             reqParams.SolutionName,
		SolutionhubRecommendBody: solutionhubRecommendBody,
	})
	utils.MetricEventDump("", "", reqParams.Type, config.MetricRecommendSucceeded, nil)

	for i := range algorithms {
		algorithms[i].Score = []*types.Score{}
	}

	return algorithms, err
}

func makeRecommendRequestBody(datasets []*types.Dataset) *types.RecommendRequestBody {
	recommnedDatasetsTrain := []*types.RecommendDatasetBody{}
	recommendDatasetsVal := []*types.RecommendDatasetBody{}
	for _, dataset := range datasets {
		boxesArr := []*types.RecommendClassCount{}
		classesArr := []*types.RecommendClassCount{}
		regressorArr := []*types.RecommendClassCount{}
		keypointArr := []*types.RecommendClassCount{}
		segmentaionArr := []*types.RecommendClassCount{}
		conversationArr := []*types.RecommendClassCount{}
		if dataset.RecommendClasses != nil && len(dataset.RecommendClasses.Boxes) > 0 {
			for k, v := range dataset.RecommendClasses.Boxes {
				boxesArr = append(boxesArr, &types.RecommendClassCount{
					Name:        k,
					ImageCount:  v,
					EntityCount: v,
				})
			}
		}
		if dataset.RecommendClasses != nil && len(dataset.RecommendClasses.Classes) > 0 {
			for k, v := range dataset.RecommendClasses.Classes {
				classesArr = append(classesArr, &types.RecommendClassCount{
					Name:        k,
					ImageCount:  v,
					EntityCount: v,
				})
			}
		}
		if dataset.RecommendClasses != nil && len(dataset.RecommendClasses.Regressors) > 0 {
			for k, v := range dataset.RecommendClasses.Regressors {
				regressorArr = append(regressorArr, &types.RecommendClassCount{
					Name:        k,
					ImageCount:  v,
					EntityCount: v,
				})
			}
		}
		if dataset.RecommendClasses != nil && len(dataset.RecommendClasses.Keypoints) > 0 {
			for k, v := range dataset.RecommendClasses.Keypoints {
				keypointArr = append(keypointArr, &types.RecommendClassCount{
					Name:        k,
					ImageCount:  v,
					EntityCount: v,
				})
			}
		}
		if dataset.RecommendClasses != nil && len(dataset.RecommendClasses.Segmentations) > 0 {
			for k, v := range dataset.RecommendClasses.Segmentations {
				segmentaionArr = append(segmentaionArr, &types.RecommendClassCount{
					Name:        k,
					ImageCount:  v,
					EntityCount: v,
				})
			}
		}
		var total = 0
		if dataset.RecommendClasses != nil {
			total = dataset.RecommendClasses.Count
		}
		if dataset.IsConverstation {
			total = 5 // LLM 推荐门槛
			conversationArr = append(conversationArr, &types.RecommendClassCount{
				Name: "_",
			})
		}
		newRecommendDataset := &types.RecommendDatasetBody{
			Total:         total,
			SdsPath:       dataset.SDSPath,
			Boxes:         boxesArr,
			Classes:       classesArr,
			Segmentations: segmentaionArr,
			Keypoints:     keypointArr,
			Regressors:    regressorArr,
			Conversations: conversationArr,
		}

		if dataset.Type == aisConst.DatasetTypeTrain {
			recommnedDatasetsTrain = append(recommnedDatasetsTrain, newRecommendDataset)
		}

		if dataset.Type == aisConst.DatasetTypeValid {
			recommendDatasetsVal = append(recommendDatasetsVal, newRecommendDataset)
		}
	}

	return &types.RecommendRequestBody{
		TrainDatasets: &types.RecommendTypedDatasets{
			Datasets: recommnedDatasetsTrain,
		},
		ValDatasets: &types.RecommendTypedDatasets{
			Datasets: recommendDatasetsVal,
		},
	}
}

func gpuOptionsFilter(apuTypes []string, apuConfigs []*aisConst.APUConfig, esources *solution.Resources) *solution.Resources {
	v1, _ := json.Marshal(apuTypes)
	v2, _ := json.Marshal(apuConfigs)
	v3, _ := json.Marshal(esources)
	log.WithField("apuTypes", string(v1)).WithField("apuConfigs", string(v2)).WithField("esources", string(v3)).Warnln("show gpu options")
	var isAisNormal bool
	if len(apuTypes) == 0 {
		isAisNormal = true
	}

	apuConfigMap := map[string]bool{}
	for _, apuConfig := range apuConfigs {
		if apuConfig.IsAisNormal != isAisNormal {
			continue
		}
		apuConfigMap[apuConfig.Product] = true
	}

	var newAPUTypes []string
	if isAisNormal {
		for key := range apuConfigMap {
			newAPUTypes = append(newAPUTypes, key)
		}
	} else {
		for _, apuType := range apuTypes {
			if apuConfigMap[apuType] {
				newAPUTypes = append(newAPUTypes, apuType)
			}
		}
	}

	apuTypeMap := map[string]bool{}
	for _, apuType := range newAPUTypes {
		apuTypeMap[apuType] = true
	}

	var inferGPUOptions, trainGPUOptions []*solution.GPUOptions
	for _, gpuOption := range esources.InferRequestsResources.GPUOptions {
		if apuTypeMap[gpuOption.GPUName] {
			inferGPUOptions = append(inferGPUOptions, gpuOption)
		}
	}
	for _, gpuOption := range esources.TrainRequestsResources.GPUOptions {
		if apuTypeMap[gpuOption.GPUName] {
			trainGPUOptions = append(trainGPUOptions, gpuOption)
		}
	}
	esources.InferRequestsResources.GPUOptions = inferGPUOptions
	esources.TrainRequestsResources.GPUOptions = trainGPUOptions

	return esources
}

func proposedResources(resources *solution.Resources) *types.ProposedResource {
	res := &types.ProposedResource{
		TrainRequestsResources: types.RequestResource{
			CPU:    resources.TrainRequestsResources.CPU,
			Memory: resources.TrainRequestsResources.Memory,
		},
		InferRequestsResources: types.RequestResource{
			CPU:    resources.InferRequestsResources.CPU,
			Memory: resources.InferRequestsResources.Memory,
		},
	}

	trainRequestGPUs := map[string]*solution.GPUOptions{}
	inferRequestGPUs := map[string]*solution.GPUOptions{}
	for _, option := range resources.TrainRequestsResources.GPUOptions {
		trainRequestGPUs[option.GPUName] = option
	}
	for _, option := range resources.InferRequestsResources.GPUOptions {
		inferRequestGPUs[option.GPUName] = option
	}

	trainDefaultGPUs := []string{types.DefaultTrainDevice}
	inferDefaultGPUs := []string{types.DefaultTrainDevice}
	if config.Profile.GPUOption != nil && len(config.Profile.GPUOption.TrainAlias) > 0 {
		trainDefaultGPUs = config.Profile.GPUOption.TrainAlias
	}
	if config.Profile.GPUOption != nil && len(config.Profile.GPUOption.InferAlias) > 0 {
		inferDefaultGPUs = config.Profile.GPUOption.InferAlias
	}

	trainDefaultGPULabels := []string{types.DefaultTrainDevice}
	for _, d := range trainDefaultGPUs {
		trainDeviceHelper := aisConst.NewDeviceHelper(d)
		trainDefaultGPULabels = append(trainDefaultGPULabels, trainDeviceHelper.Labels()...)
	}

	inferDefaultGPULabels := []string{types.DefaultTrainDevice}
	for _, d := range inferDefaultGPUs {
		inferDeviceHelper := aisConst.NewDeviceHelper(d)
		inferDefaultGPULabels = append(inferDefaultGPULabels, inferDeviceHelper.Labels()...)
	}
	for _, label := range trainDefaultGPULabels {
		trainOption := trainRequestGPUs[label]
		if trainOption != nil && trainOption.GPU != 0 {
			res.TrainRequestsResources.GPUName = label
			res.TrainRequestsResources.GPU = trainOption.GPU
			res.TrainRequestsResources.GPUTypeKey = trainOption.GPUTypeKey
			res.TrainRequestsResources.ResouceName = trainOption.ResourceName
			break
		}
	}
	for _, label := range inferDefaultGPULabels {
		inferOption := inferRequestGPUs[label]
		if inferOption != nil && inferOption.GPU != 0 {
			res.InferRequestsResources.GPUName = label
			res.InferRequestsResources.GPU = inferOption.GPU
			res.InferRequestsResources.GPUTypeKey = inferOption.GPUTypeKey
			res.InferRequestsResources.ResouceName = inferOption.ResourceName
			break
		}
	}

	return res
}
