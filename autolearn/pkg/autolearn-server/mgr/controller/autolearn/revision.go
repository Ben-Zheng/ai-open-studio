package autolearn

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	aisclient "go.megvii-inc.com/brain/brainpp/projects/aiservice/components/pkg/client/clientset/versioned"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	solution "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/solution"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	publicTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types"
	resourceSenseClient "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg"
	schedulerv1alpha1 "go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/client/clientset/versioned/typed/gang/v1alpha1"
)

// newDefaultAutoLearnRevision 构建默认的revision document
func (ctr *Controller) newDefaultAutoLearnRevision(gc *ginlib.GinContext, gin *ginlib.Gin,
	request *types.AutoLearnCreateRequest, datasets []*types.Dataset,
) (*types.AutoLearnRevision, error) {
	return ctr.newAutoLearnRevision(gc, gin,
		&types.RevisionCreateRequest{
			RevisionName: config.DefaultRevisionName,
			SameCreateRequest: &types.SameCreateRequest{
				Approach:           request.Approach,
				Type:               request.Type,
				CreateMode:         request.CreateMode,
				SnapdetMode:        request.SnapdetMode,
				EvalMetric:         request.EvalMetric,
				ClfEvalMetric:      request.ClfEvalMetric,
				SpeedMetric:        request.SpeedMetric,
				QuotaGroupID:       request.QuotaGroupID,
				TimeLimit:          request.TimeLimit,
				WorkerNum:          request.WorkerNum,
				GPUPerWorker:       request.GPUPerWorker,
				Algorithms:         request.Algorithms,
				OriginalAlgorithms: request.OriginalAlgorithms,
				ScheduleTimeLimit:  request.ScheduleTimeLimit,
				ModelType:          request.ModelType,
				QuotaPreemptible:   request.QuotaPreemptible,
				SnapXImageURI:      request.SnapXImageURI,
				SolutionType:       request.SolutionType,
				SnapEnvs:           request.SnapEnvs,
				ManagedBy:          request.ManagedBy,
				SnapSampler:        request.SnapSampler,
				Continued:          request.Continued,
				ContinuedMeta:      request.ContinuedMeta,
				GPUTypes:           request.GPUTypes,
			},
		},
		datasets, &types.AutoLearn{TenantID: gc.GetAuthTenantID(), ProjectID: gc.GetAuthProjectID(), AutoLearnID: gin.GenerateStrID()})
}

// NewOpAutoLearnRevision 构建优化 revision document
func (ctr *Controller) NewOpAutoLearnRevision(gc *ginlib.GinContext, gin *ginlib.Gin, request *types.RevisionCreateRequest,
	datasets []*types.Dataset, autoLearn *types.AutoLearn,
) (*types.AutoLearnRevision, string) {
	rv, _ := ctr.newAutoLearnRevision(gc, gin, request, datasets, autoLearn)
	rv.IsOptimized = true
	rv.Algorithms = request.Algorithms
	rv.OriginalAlgorithms = request.OriginalAlgorithms
	rv.AutoLearnState = types.AutoLearnStateOptimizing
	rv.AutoLearnStateStream[0].AutoLearnState = types.AutoLearnStateOptimizing

	// 目前datasets长度只为2，只保留验证集数据
	var trainSDS string
	var valDatasets []*types.Dataset
	for i := range rv.Datasets {
		if rv.Datasets[i].Type == aisConst.DatasetTypeTrain {
			trainSDS = datasets[i].SDSPath
			continue
		}
		valDatasets = append(valDatasets, rv.Datasets[i])
	}
	rv.Datasets = valDatasets

	return rv, trainSDS
}

// NewAutoLearnRevision 构建revision document
func (ctr *Controller) newAutoLearnRevision(gc *ginlib.GinContext, gin *ginlib.Gin, request *types.RevisionCreateRequest,
	datasets []*types.Dataset, autoLearn *types.AutoLearn,
) (*types.AutoLearnRevision, error) {
	rs := &types.Resource{
		WorkerNum: request.WorkerNum,
	}

	if request.WorkerNum == 0 {
		rs.WorkerNum = 2
	}

	if request.CreateMode == types.CreateModeSimple {
		// 当前只有一个算法
		if request.Type == aisConst.AppTypeClassificationAndRegression ||
			request.Type == aisConst.AppTypeRegression || request.Continued || request.Approach == aisConst.ApproachAIGC {
			rs.WorkerNum = 1
		}
	}

	// 初始化指标和算法
	if request.EvalMetric != nil {
		request.EvalMetric.MaxScore = config.EvalMetricDefaultMaxScore
	}
	if request.ClfEvalMetric != nil {
		request.ClfEvalMetric.MaxScore = config.EvalMetricDefaultMaxScore
	}

	apuConfigs, err := resourceSenseClient.NewRSClient(config.Profile.AisEndPoint.Resourcesense).GetClusterAPU()
	if err != nil {
		log.WithError(err).Error("failed to GetClusterAPU")
		return nil, err
	}

	for i := range request.Algorithms {
		request.Algorithms[i].Score = []*types.Score{}
		query := map[string]any{}
		algorName := request.Algorithms[i].Name
		for _, hy := range request.Algorithms[i].HyperParam {
			if hy.SnapdetKey == types.SnapxFinetuningKey {
				query[types.SnapxHyperParamsKey] = fmt.Sprintf("%s=%s", types.SnapxFinetuningKey, hy.Value)
			}
		}
		if request.SolutionType == types.SolutionTypeCustom {
			algorName = fmt.Sprintf("cst_%s", strings.ReplaceAll(algorName, "/", ""))
			query[types.SnapxSolTypeKey] = request.Type
		}
		resourceClient := solution.NewSolutionClient(config.Profile.AisEndPoint.SolutionHub)
		solutionResource, err := resourceClient.GetSolutionResources(algorName, query)
		if err != nil {
			log.WithError(err).Error("get solution resource failed")
			return nil, err
		}
		if features.IsQuotaSupportGPUGroupEnabled() {
			solutionResource = gpuOptionsFilter(request.GPUTypes, apuConfigs, solutionResource)
		}
		request.Algorithms[i].ProposedResource = proposedResources(solutionResource)
		if request.Algorithms[i].ProposedResource.InferRequestsResources.GPU == 0 || request.Algorithms[i].ProposedResource.TrainRequestsResources.GPU == 0 {
			log.WithError(errors.ErrorAPUTypeNotMatch).Error("apu not match")
			return nil, errors.ErrorAPUTypeNotMatch
		}
	}

	// 保存snapX镜像信息
	snapXImageURI := config.Profile.DependentImage.Snapx
	if request.SnapXImageURI != "" {
		snapXImageURI = request.SnapXImageURI
	}

	managedBy := publicTypes.AISAdmissionObjectSelectorLabelValue
	if request.ManagedBy != "" {
		managedBy = request.ManagedBy
	}

	if request.SolutionType == types.SolutionTypeCustom {
		if err := ctr.SetAlgorithmEnvs(autoLearn.TenantID, autoLearn.ProjectID, request.Algorithms); err != nil {
			log.WithError(err).Error("set custom algorithm envs failed")
			return nil, err
		}
	}

	rvID := gin.GenerateStrID()
	return &types.AutoLearnRevision{
		AutoLearnID:          autoLearn.AutoLearnID,
		RevisionID:           rvID,
		RevisionName:         request.RevisionName,
		Approach:             request.Approach,
		Type:                 request.Type,
		Datasets:             datasets,
		SnapSampler:          request.SnapSampler,
		TimeLimit:            request.TimeLimit,
		ScheduleTimeLimit:    request.ScheduleTimeLimit,
		CreateMode:           request.CreateMode,
		SnapdetMode:          request.SnapdetMode,
		ModelType:            request.ModelType,
		QuotaGroup:           request.QuotaGroupID,
		Resource:             rs,
		EvalMetric:           request.EvalMetric,
		ClfEvalMetric:        request.ClfEvalMetric,
		SpeedMetric:          request.SpeedMetric,
		EvaluationDetails:    []*types.EvaluationDetail{},
		Algorithms:           request.Algorithms,
		OriginalAlgorithms:   request.OriginalAlgorithms,
		AutoLearnState:       types.AutoLearnStateInitial,
		AutoLearnStateStream: []*types.AutoLearnStateStream{{AutoLearnState: types.AutoLearnStateInitial, CreatedAt: time.Now().Unix()}},
		CreatedBy:            &types.User{UserID: gc.GetUserID(), UserName: gc.GetUserName()},
		UpdatedBy:            &types.User{UserID: gc.GetUserID(), UserName: gc.GetUserName()},
		CreatedAt:            time.Now().Unix(),
		UpdatedAt:            time.Now().Unix(),
		SnapXImageURI:        snapXImageURI,
		SolutionType:         request.SolutionType,
		LogsPodName:          utils.GetAutoLearnMasterPodName(autoLearn.AutoLearnID, rvID),
		IsDeleted:            false,
		SnapEnvs:             request.SnapEnvs,
		Continued:            request.Continued,
		ContinuedMeta:        request.ContinuedMeta,
		QuotaPreemptible:     request.QuotaPreemptible,
		ManagedBy:            managedBy,
		DetectedObjectType:   request.DetectedObjectType,
		GPUTypes:             request.GPUTypes,
	}, nil
}

// TransformRevision 转换revision信息进行前端展示
func TransformRevision(rv *types.AutoLearnRevision) {
	// 1 组装状态事件流
	var stateStream []*types.AutoLearnStateStream
	var isScheduling bool
	for i, state := range rv.AutoLearnStateStream {
		if state.AutoLearnState <= types.AutoLearnStateSpeedSucceeded ||
			state.AutoLearnState == types.AutoLearnStateRecommending ||
			state.AutoLearnState == types.AutoLearnStateLearnFailed ||
			state.AutoLearnState == types.AutoLearnStateOptimizing ||
			state.AutoLearnState == types.AutoLearnStateOptimizeFailed ||
			state.AutoLearnState == types.AutoLearnStateOptimizeSucceeded {
			stateStream = append(stateStream, state)
		}

		if state.AutoLearnState == types.AutoLearnStateMasterPodCreateSucceeded ||
			(state.AutoLearnState >= types.AutoLearnStateMasterPodCreating &&
				state.AutoLearnState <= types.AutoLearnStateWorkerAJobCreated &&
				state.AutoLearnState != types.AutoLearnStateMasterPodRunFailed) {
			if !isScheduling {
				stateStream = append(stateStream, &types.AutoLearnStateStream{AutoLearnState: types.DisplayStateScheduling, CreatedAt: state.CreatedAt})
				isScheduling = true
			}

			if len(rv.AutoLearnStateStream) == (i + 1) {
				rv.AutoLearnState = types.DisplayStateScheduling
			}

			if len(rv.AutoLearnStateStream) > (i+1) && state.AutoLearnState == types.AutoLearnStateWorkerAJobCreated {
				stateStream = append(stateStream, &types.AutoLearnStateStream{AutoLearnState: types.DisplayStateScheduleSuccess, CreatedAt: state.CreatedAt})
			}
		}

		if state.AutoLearnState == types.AutoLearnStateMasterPodRunFailed ||
			state.AutoLearnState == types.AutoLearnStateWorkerAJobFailed ||
			state.AutoLearnState == types.AutoLearnStateMasterPodCreateFailed ||
			state.AutoLearnState == types.AutoLearnStatePodsCreateFailed {
			stateStream = append(stateStream, &types.AutoLearnStateStream{AutoLearnState: types.DisplayStateScheduleFailed, CreatedAt: state.CreatedAt})
			rv.AutoLearnState = types.DisplayStateScheduleFailed
		}
	}
	rv.AutoLearnStateStream = stateStream

	// 2 开始学习时间,由前端计算 剩余时间 = timeLimit - 开始学习时间
	for i := range rv.AutoLearnStateStream {
		if rv.AutoLearnStateStream[i].AutoLearnState == types.AutoLearnStateLearning {
			rv.ConvertToStateLearningTime = rv.AutoLearnStateStream[i].CreatedAt
			break
		}
	}

	// 3 任务耗时 = 终态时间-创建时间
	if CheckAutoLearnStateInFinal(rv.AutoLearnState) && len(rv.AutoLearnStateStream) != 0 {
		rv.TimeConsumed = getSecondsDiffToHMS(rv.CreatedAt, rv.AutoLearnStateStream[len(rv.AutoLearnStateStream)-1].CreatedAt)
	}

	// 4 组装处于评测中评测job，并且将内部状态转为对外状态;去除已删除的job
	rv.EvaluationJobsStateInRunning = []int32{}
	var arr []*types.EvaluationDetail
	for i := range rv.EvaluationDetails {
		if checkEvalJobStateInRunning(rv.EvaluationDetails[i].EvaluationJobState) {
			rv.EvaluationJobsStateInRunning = append(rv.EvaluationJobsStateInRunning, rv.EvaluationDetails[i].IterationNumber)
			rv.EvaluationDetails[i].EvaluationJobState = evalTypes.EvalJobResultStateRunning
		}
		if !rv.EvaluationDetails[i].IsDeleted {
			arr = append(arr, rv.EvaluationDetails[i])
		}
	}
	rv.EvaluationDetails = arr

	// 5 组装BestDetector曲线
	rv.BestDetector = GetBestDetector(rv.Type, rv.Algorithms)

	// 6 转为待学习(调度中)时间，用于计算剩余调度时长 =
	for i := range rv.AutoLearnStateStream {
		if rv.AutoLearnStateStream[i].AutoLearnState == types.AutoLearnStateToBeLearned {
			rv.ConvertToStateSchedulingTime = rv.AutoLearnStateStream[i].CreatedAt
			break
		}
	}

	// 7 兼容V2.0: algorithms.property >>> algorithms.properties
	for i := range rv.Algorithms {
		if properties := convertAlgorithmProperty(rv.Algorithms[i].Property); properties != nil {
			rv.Algorithms[i].Properties = properties
		}
	}
	for i := range rv.OriginalAlgorithms {
		if properties := convertAlgorithmProperty(rv.OriginalAlgorithms[i].Property); properties != nil {
			rv.OriginalAlgorithms[i].Properties = properties
		}
	}

	// 8 填充算法属性描述信息
	WrapRecommendAlgorithm(rv.Algorithms)

	// 10 evaluationDetail 处理
	fillEvaluationDetailAlgorithmName(rv.EvaluationDetails, rv.Algorithms)
	sort.Slice(rv.EvaluationDetails, func(i, j int) bool {
		return rv.EvaluationDetails[i].IterationNumber < rv.EvaluationDetails[j].IterationNumber
	})

	if rv.AutoLearnState == types.AutoLearnStateRecommendFailed &&
		rv.Reason != types.EventCodeMessage[types.StateEventCodeRecommendFailed] &&
		rv.Reason != types.EventCodeMessage[types.StateEventCodeRecommendFailedByInvalidSDS] &&
		rv.Reason != types.EventCodeMessage[types.StateEventCodeRecommendFailedByNotFulfill] &&
		rv.Reason != types.EventCodeMessage[types.StateEventCodeRecommendFailedByNotFound] &&
		rv.Reason != types.EventCodeMessage[types.StateEventCodeRecommendFailedByTimeout] {
		rv.Reason = types.EventCodeMessage[types.StateEventCodeRecommendFailed]
	}
}

func fillEvaluationDetailAlgorithmName(evals []*types.EvaluationDetail, algs []*types.Algorithm) {
	itermNumToAlg := make(map[int32]*types.Algorithm)

	for _, alg := range algs {
		for _, score := range alg.Score {
			if score.IterationNumber > 0 && itermNumToAlg[score.IterationNumber] == nil {
				itermNumToAlg[score.IterationNumber] = alg
			} else if itermNumToAlg[score.IterationNumber] != nil {
				log.Infof("duplicate alg, iter number : %d, alg: %s", score.IterationNumber, alg.Name)
			}
		}
	}

	for _, detail := range evals {
		if alg, ok := itermNumToAlg[detail.IterationNumber]; ok {
			detail.AlgorithmName = alg.Name
			detail.SolutionType = alg.SolutionType
			detail.SolutionImageURI = alg.ImageURI
		}
	}
}

// TransformRevisionState 转换revision状态
func TransformRevisionState(rv *types.AutoLearnRevision) {
	switch rv.AutoLearnState {
	case types.AutoLearnStateRecommendSucceeded, types.AutoLearnStateDatasetChecking, types.AutoLearnStateDatasetCheckSucceeded,
		types.AutoLearnStateSpeeding, types.AutoLearnStateSpeedSucceeded, types.AutoLearnStateOptimizeSucceeded:
		rv.AutoLearnState = types.AutoLearnStateInitial
	case types.AutoLearnStateMasterPodCreateSucceeded:
		rv.AutoLearnState = types.AutoLearnStateToBeLearned
	case types.AutoLearnStateRecommendFailed, types.AutoLearnStateDatasetCheckFailed, types.AutoLearnStateSpeedFailed, types.AutoLearnStateMasterPodCreateFailed:
		rv.AutoLearnState = types.AutoLearnStateInitialFailed
	case types.AutoLearnStatePodsCreateFailed:
		rv.AutoLearnState = types.AutoLearnStateException
	}
}

// checkAutoLearnStateInInitialFailed 检查是否为初始化失败状态
func checkAutoLearnStateInInitialFailed(state types.AutoLearnState) bool {
	return state == types.AutoLearnStateRecommendFailed || state == types.AutoLearnStateDatasetCheckFailed ||
		state == types.AutoLearnStateSpeedFailed || state == types.AutoLearnStateMasterPodCreateFailed
}

// 检查是否为对外展示状态
func checkAutoLearnStateInShow(state types.AutoLearnState) bool {
	return state == types.AutoLearnStateInitial || state == types.AutoLearnStateToBeLearned || state == types.AutoLearnStateLearning ||
		state == types.AutoLearnStateCompleted || state == types.AutoLearnStateStopped || state == types.AutoLearnStateInitialFailed ||
		state == types.AutoLearnStateException || state == types.AutoLearnStateOptimizing || state == types.AutoLearnStateOptimizeFailed
}

// 检查是否为终态
func CheckAutoLearnStateInFinal(state types.AutoLearnState) bool {
	return state == types.AutoLearnStateCompleted || state == types.AutoLearnStateStopped || state == types.AutoLearnStateException ||
		state == types.AutoLearnStateInitialFailed || state == types.AutoLearnStateOptimizeFailed
}

// getSecondsDiffToHMS 获取时间差值，并以时分秒方式展示
func getSecondsDiffToHMS(start, end int64) string {
	diff := int(time.Unix(end, 0).Sub(time.Unix(start, 0)).Seconds())

	return fmt.Sprintf("%dh:%dm:%ds", diff/3600, diff%3600/60, diff%60)
}

// 检查评测任务是否为评测中状态
func checkEvalJobStateInRunning(state evalTypes.EvalJobResultState) bool {
	return state == evalTypes.EvalJobResultStateNone || state == evalTypes.EvalJobResultStateCreated || state == evalTypes.EvalJobResultStatePending ||
		state == evalTypes.EvalJobResultStateStarting || state == evalTypes.EvalJobResultStateRunning
}

// GetBestDetector 获取bestDetector曲线
func GetBestDetector(trainType aisConst.AppType, algorithms []*types.Algorithm) *types.BestDetector {
	bestDetector := &types.BestDetector{Name: config.BestDetector}
	if trainType == aisConst.AppTypeClassification {
		bestDetector.Name = config.BestClasscifical
	}

	for i := range algorithms {
		for j := range algorithms[i].Score {
			if algorithms[i].Score[j].IterationNumber != 0 {
				bestDetector.BestDetectorScore = append(bestDetector.BestDetectorScore, &types.BestDetectorScore{
					AlgorithmName: algorithms[i].Name,
					Score:         algorithms[i].Score[j],
				})
			}
		}
	}

	sort.Slice(bestDetector.BestDetectorScore, func(i, j int) bool {
		return bestDetector.BestDetectorScore[i].Score.IterationNumber < bestDetector.BestDetectorScore[j].Score.IterationNumber
	})

	bestDetectorFilter := &types.BestDetector{Name: config.BestDetector}
	var currentMaxScore float64
	for _, score := range bestDetector.BestDetectorScore {
		if score.Score.Value >= currentMaxScore {
			currentMaxScore = score.Score.Value
			bestDetectorFilter.BestIterationNumber = score.IterationNumber
			bestDetectorFilter.BestDetectorScore = append(bestDetectorFilter.BestDetectorScore, score)
		}
	}

	return bestDetectorFilter
}

// convertAlgorithmProperty 为了兼容旧数据，进行数据转换
func convertAlgorithmProperty(property *types.PropertyV1) []*types.Property {
	if property != nil {
		arr := make([]*types.Property, 0, len(property.Performance))
		for j := range property.Performance {
			p := types.Property{
				Value: property.Performance[j].Value,
				Unit:  property.Performance[j].Unit,
			}
			switch property.Performance[j].Key {
			case types.AlgorithmPerformanceLatency:
				p.Key = string(types.AlgorithmPerformanceLatency) + "@" + property.ImageSpec + "@" + property.GPUModel
				p.GPU = property.GPUModel
				p.ImageSpec = property.ImageSpec
			case types.AlgorithmPerformanceLatencyOnnx:
				p.Key = string(types.AlgorithmPerformanceLatency) + "@" + property.ImageSpec + "@CPU@" + property.CPUModel
				p.CPU = property.CPUModel
				p.ImageSpec = property.ImageSpec
			case types.AlgorithmPerformanceMACs:
				p.Key = string(types.AlgorithmPerformanceMACs) + "@" + property.ImageSpec
				p.ImageSpec = property.ImageSpec
			case types.AlgorithmPerformanceModelSize:
				p.Key = "model_size" + "@" + property.ImageSpec
			}
			arr = append(arr, &p)
		}
		return arr
	}

	return nil
}

// WrapRecommendAlgorithm 转换推荐算法进行前端展示
func WrapRecommendAlgorithm(algorithms []*types.Algorithm) {
	for i := range algorithms {
		for j := range algorithms[i].Properties {
			arr := strings.Split(algorithms[i].Properties[j].Key, "@")
			if len(arr) == 0 {
				continue
			}
			p := algorithms[i].Properties[j]
			switch arr[0] {
			case string(types.AlgorithmPerformanceLatency):
				if p.GPU != "" {
					p.Description = "以" + p.GPU + "为推理后端时推理一张" + p.ImageSpec + "所需时间"
				} else {
					p.Description = "以" + p.CPU + "为推理后端时推理一张" + p.ImageSpec + "所需时间"
				}
			case string(types.AlgorithmPerformanceMACs):
				p.Description = "推理一张" + p.ImageSpec + "理论计算量"
			case "model_size":
				p.Description = "模型大小"
			case "memory":
				p.Description = "以" + p.GPU + "为推理后端时推理一张" + p.ImageSpec + "所需内存"
			case "flops":
				p.Description = "该模型的浮点数计算量"
			}
		}
	}
}

// checkRevisionNameValid 版本名合法性校验
func CheckRevisionNameValid(autoLearn *types.AutoLearn, revisionName string) error {
	// 1 正则校验
	valid, err := regexp.MatchString(config.AutoLearnAndRevisionNameRegex, revisionName)
	if err != nil {
		log.WithError(err).WithField("name", revisionName).Error("regexp failed")
		return errors.ErrorInternal
	}
	if !valid {
		log.Errorf("name does not conform to regular rules: %s", revisionName)
		return errors.ErrorInvalidParams.SetMessage("the name parameter needs to conform to the regular expression " +
			config.AutoLearnAndRevisionNameRegex)
	}

	// 2 重名校验
	for i := range autoLearn.AutoLearnRevisions {
		if !autoLearn.AutoLearnRevisions[i].IsDeleted && revisionName == autoLearn.AutoLearnRevisions[i].RevisionName {
			log.WithFields(log.Fields{"autoLearnName": autoLearn.AutoLearnName, "revisionName": revisionName}).
				Error("duplicate revision name")
			return errors.ErrorDuplicateRevisionName
		}
	}

	return nil
}

// InternalPreCheck InternalAgent服务更新检查
func InternalPreCheck(client *oss.Client, req *types.InternalAgentInfoReq, rv *types.AutoLearnRevision) error {
	// 2 sds文件路径校验
	if req.PredictSDS != "" {
		if req.IterationNumber == 0 {
			return fmt.Errorf("iterationNumber is zero while predictSds not null")
		}
	}

	// 3 model文件路径校验
	if req.ModelFilePath != "" {
		if req.IterationNumber == 0 {
			return fmt.Errorf("iterationNumber is zero while modelFilePaht not null")
		}

		if err := utils.CheckOssPathExist(client, req.ModelFilePath); err != nil {
			return err
		}
	}

	return nil
}

// GetTrainDatasetName 获取训练集的名字
func GetTrainDatasetName(datasets []*types.Dataset) string {
	for i := range datasets {
		if datasets[i].Type == aisConst.DatasetTypeTrain {
			return datasets[i].DatasetMeta.Name
		}
	}
	return ""
}

// GetValDatasetName 获取验证集名字
func GetValDatasetName(datasets []*types.Dataset) string {
	for i := range datasets {
		if datasets[i].Type == aisConst.DatasetTypeValid {
			return datasets[i].DatasetMeta.Name
		}
	}
	return ""
}

func CleanupAutoLearnPods(kubeclient *kubernetes.Clientset, brainclient *schedulerv1alpha1.SchedulerV1alpha1Client,
	aisClient *aisclient.Clientset, namespace, name string,
) {
	ajobs, err := aisClient.AjobV1alpha1().AJobs(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: types.AutoLearnNameKey + "=" + name,
	})
	if err != nil {
		log.WithError(err).Errorf("list autolearn %s related ajobs failed", name)
	}

	for i := range ajobs.Items {
		if err := aisClient.AjobV1alpha1().AJobs(namespace).Delete(context.TODO(), ajobs.Items[i].Name, metav1.DeleteOptions{}); err != nil {
			log.WithError(err).Errorf("delete autolearn %s ajobs runner %s failed", name, ajobs.Items[i].Name)
		}
	}

	if features.IsSupportGangSchedule() {
		if err := brainclient.PodGroups(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{}); err != nil {
			log.WithError(err).Errorf("delete autolearn %s podgroup failed", name)
		}
	}

	if err := kubeclient.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{}); err != nil {
		log.WithError(err).Errorf("delete autolearn %s pod failed", name)
	}
}

func PreCheckAndGetOptimization(ossClient *oss.Client, rv *types.AutoLearnRevision) (*types.EvaluationDetail, error) {
	if rv.Type != aisConst.AppTypeDetection {
		log.Errorf("unsupported train type %s", rv.Type)
		return nil, errors.ErrorInvalidParams
	}

	if rv.AutoLearnState != types.AutoLearnStateCompleted && rv.AutoLearnState != types.AutoLearnStateStopped {
		log.Error("only supported state completed and stopped")
		return nil, errors.ErrorInvalidParams
	}

	if len(rv.EvaluationDetails) == 0 {
		log.Errorf("box-detector file not exists")
		return nil, errors.ErrorBoxDetectorNotFound
	}

	boxDetectorPath := rv.EvaluationDetails[len(rv.EvaluationDetails)-1].ModelFilePath
	if strings.HasSuffix(boxDetectorPath, utils.MODEL_ZIP) {
		log.Errorf("old data type, no box-detector file")
		return nil, errors.ErrorBoxDetectorNotFound
	}
	log.Infof("[INFO] box-detector: %s", boxDetectorPath)
	if err := utils.CheckOssPathExist(ossClient, boxDetectorPath); err != nil {
		log.WithError(err).Errorf("invalid box-detector path: %s", boxDetectorPath)
		return nil, errors.ErrorBoxDetectorNotFound
	}

	return rv.EvaluationDetails[len(rv.EvaluationDetails)-1], nil
}

func GetRebuildRevisionCreateRequest(rv *types.AutoLearnRevision) *types.RevisionCreateRequest {
	newReq := &types.RevisionCreateRequest{
		RevisionName: fmt.Sprintf("re-%s-%s", rv.RevisionName, utils.RandStringRunesWithLower(4)),
		SameCreateRequest: &types.SameCreateRequest{
			Approach:           rv.Approach,
			Type:               rv.Type,
			CreateMode:         rv.CreateMode,
			SnapdetMode:        rv.SnapdetMode,
			EvalMetric:         rv.EvalMetric,
			ClfEvalMetric:      rv.ClfEvalMetric,
			QuotaGroupID:       rv.QuotaGroup,
			QuotaGroupName:     rv.QuotaGroupName,
			PrivateMachine:     false, // TODO: check
			TimeLimit:          rv.TimeLimit,
			ScheduleTimeLimit:  rv.ScheduleTimeLimit,
			WorkerNum:          rv.Resource.WorkerNum,
			GPUPerWorker:       rv.Resource.GPU,
			Algorithms:         rv.Algorithms,
			OriginalAlgorithms: rv.OriginalAlgorithms,
			SpeedMetric:        rv.SpeedMetric,
			ModelType:          rv.ModelType,
			SolutionType:       rv.SolutionType,
			Continued:          rv.Continued,
			ContinuedMeta:      rv.ContinuedMeta,
			DetectedObjectType: rv.DetectedObjectType,
			SnapSampler:        rv.SnapSampler,
			GPUTypes:           rv.GPUTypes,
		},
	}

	if newReq.ModelType == string(types.CreateModeSimple) {
		newReq.Algorithms = nil
		newReq.OriginalAlgorithms = nil
	}

	if newReq.Type == aisConst.AppTypeDetection && newReq.DetectedObjectType == "" {
		newReq.DetectedObjectType = aisConst.DetectedObjectTypeNormal
	}

	return newReq
}

func GetOpRevisionCreateRequest(rv *types.AutoLearnRevision) *types.RevisionCreateRequest {
	var isExistValid bool

	data := make([]*types.DataSource, 0, 2)
	for i := range rv.Datasets {
		// 可能是 pair，存在多个验证集，只取其中一个
		if isExistValid && rv.Datasets[i].Type == aisConst.DatasetTypeValid {
			continue
		}

		data = append(data, &types.DataSource{
			SourceType: types.SourceTypeSDS,
			DataURI:    rv.Datasets[i].SDSPath,
			Type:       rv.Datasets[i].Type,
		})

		if rv.Datasets[i].Type == aisConst.DatasetTypeValid {
			isExistValid = true
		}
	}

	objectType := rv.DetectedObjectType
	if rv.Type == aisConst.AppTypeDetection && rv.DetectedObjectType == "" {
		objectType = aisConst.DetectedObjectTypeNormal
	}

	return &types.RevisionCreateRequest{
		RevisionName: utils.GenerateNameForOpRevision(rv.RevisionName), // TODO 长度需要优化
		SameCreateRequest: &types.SameCreateRequest{
			Type:               rv.Type,
			Datasets:           data,
			CreateMode:         rv.CreateMode, // 保持原有模式以及资源，但实际流程按简单模式
			SnapdetMode:        rv.SnapdetMode,
			EvalMetric:         rv.EvalMetric,
			ClfEvalMetric:      rv.ClfEvalMetric,
			QuotaGroupID:       rv.QuotaGroup,
			QuotaGroupName:     rv.QuotaGroupName,
			PrivateMachine:     false, // TODO: check
			TimeLimit:          rv.TimeLimit,
			ScheduleTimeLimit:  rv.ScheduleTimeLimit,
			WorkerNum:          rv.Resource.WorkerNum,
			GPUPerWorker:       rv.Resource.GPU,
			Algorithms:         rv.Algorithms,         // 默认原值以通过校验，后面重置
			OriginalAlgorithms: rv.OriginalAlgorithms, // 默认原值以通过校验，后面重置
			SpeedMetric:        rv.SpeedMetric,
			ModelType:          rv.ModelType,
			SolutionType:       rv.SolutionType,
			IsOptimized:        true,
			DetectedObjectType: objectType,
			GPUTypes:           rv.GPUTypes,
		},
	}
}

// 使用原有的版本名，推荐算法保留原版已有的推荐算法，其他所有信息都按原版本的创建参数重新训练
// 不会检查数据集的内部数据变化，snapX镜像使用当前配置文件中版本，对于给定的数据集内部发生变化和snapX镜像变化导致的训练差异或异常不负责
func GetRevisionRetryRequest(oldRevision *types.AutoLearnRevision) *types.RevisionCreateRequest {
	datasets := make([]*types.DataSource, 0, len(oldRevision.Datasets))
	for _, d := range oldRevision.Datasets {
		dataSource := types.DataSource{SourceType: d.SourceType}
		switch dataSource.SourceType {
		case types.SourceTypeDataset:
			dataSource.DataURI = d.DatasetMeta.ID + "," + d.DatasetMeta.RevisionID
			dataSource.Type = d.Type
			dataSource.OriginLevel = d.DatasetMeta.OriginLevel
		case types.SourceTypePair:
			dataSource.DataURI = d.DatasetMeta.PairID + "," + d.DatasetMeta.PairRevisionID
			dataSource.OriginLevel = d.DatasetMeta.OriginLevel
		case types.SourceTypeSDS:
			dataSource.DataURI = d.SDSPath
			dataSource.Type = d.Type
		}
		datasets = append(datasets, &dataSource)
	}
	return &types.RevisionCreateRequest{
		RevisionName: oldRevision.RevisionName,
		SameCreateRequest: &types.SameCreateRequest{
			Type:               oldRevision.Type,
			Datasets:           datasets,
			CreateMode:         oldRevision.CreateMode,
			SnapdetMode:        oldRevision.SnapdetMode,
			EvalMetric:         oldRevision.EvalMetric,
			ClfEvalMetric:      oldRevision.ClfEvalMetric,
			QuotaGroupID:       oldRevision.QuotaGroup,
			TimeLimit:          oldRevision.TimeLimit,
			ScheduleTimeLimit:  oldRevision.ScheduleTimeLimit,
			WorkerNum:          oldRevision.Resource.WorkerNum,
			GPUPerWorker:       oldRevision.Resource.GPU,
			Algorithms:         oldRevision.Algorithms,         // 保留
			OriginalAlgorithms: oldRevision.OriginalAlgorithms, // 保留
			SpeedMetric:        oldRevision.SpeedMetric,
			ModelType:          oldRevision.ModelType,
		},
	}
}
