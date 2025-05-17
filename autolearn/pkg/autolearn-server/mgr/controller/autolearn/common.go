package autolearn

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	datahubType "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/types/dataset"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/sds"
	publicService "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg"
	psSpeedupType "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types/speed"
)

// CheckResourceValid  检查高级模式下选择的训练资源是否符合要求
func CheckResourceValid(timeLimit float64, workerNum int32) error {
	if timeLimit < config.SnapdetMinTimeLimit {
		return fmt.Errorf("invalid timeLimit: %f", timeLimit)
	}

	if workerNum < config.SnapdetMinWorkerNum || workerNum > config.SnapdetMaxWorkerNum {
		return fmt.Errorf("invalid workerNum: %d", workerNum)
	}

	return nil
}

func (ctr *Controller) PreCheckAndWrapDatasetsV2(gc *ginlib.GinContext, datasets []*types.DataSource, wrapDatasets *[]*types.Dataset) error {
	// 1 数据源数量及类型一致性校验，来源为数据集可以与SDS混合，必须包含多个train和一个val；来源为训验对不能与其他混合，去掉限制
	if len(datasets) == 0 {
		log.Error("datasets is null")
		return errors.ErrorInvalidParams
	}

	switch datasets[0].SourceType {
	case types.SourceTypeDataset, types.SourceTypeSDS:
		var trainCount, valCount int64
		for i := range datasets {
			if datasets[i].SourceType != types.SourceTypeDataset && datasets[i].SourceType != types.SourceTypeSDS {
				log.Error("inconsistent dataset types")
				return errors.ErrorInvalidParams
			}
			if datasets[i].Type == aisConst.DatasetTypeTrain {
				trainCount++
			}
			if datasets[i].Type == aisConst.DatasetTypeValid {
				valCount++
			}
		}
		if trainCount == 0 || valCount != 1 {
			log.Error("datasets must have train and validation type")
			return errors.ErrorInvalidParams
		}
	case types.SourceTypePair:
		if len(datasets) == 0 {
			log.Errorf("invalid dataset numbers: valid-1, invalid-%d", len(datasets))
			return errors.ErrorInvalidParams
		}
	default:
		log.Errorf("invalid dataset source type: %s", datasets[0].SourceType)
		return errors.ErrorInvalidParams
	}

	// 本地调试请使用SDS创建方式，否则下面验证不通过

	// 2 获取数据源相关信息，重新组装
	for i := range datasets {
		if err := ctr.WrapDataset(gc, datasets[i], wrapDatasets); err != nil {
			return err
		}
	}

	return nil
}

// WrapDataset 根据不同的数据类型获取数据源信息并填充
func (ctr *Controller) WrapDataset(gc *ginlib.GinContext,
	dataset *types.DataSource, wrapDatasets *[]*types.Dataset,
) error {
	switch dataset.SourceType {
	case types.SourceTypeDataset:
		ids := strings.Split(dataset.DataURI, ",")
		if len(ids) != 2 {
			return errors.ErrorInvalidParams
		}
		meta, respCode, err := ctr.DatahubClient.GetDataset(gc, ids[0], ids[1], dataset.OriginLevel)
		if err != nil {
			log.WithError(err).WithField("dataset", dataset).Error("failed to get dataset")
			if respCode == http.StatusNotFound {
				return errors.ErrorDatasetNotFound
			}
			return errors.ErrorDataHubService
		}
		if meta.Revisions[0].PublishState != datahubType.PublishStatePublished {
			log.WithError(err).WithField("dataset", dataset).Error("dataset revision is not published")
			return errors.ErrorInvalidDataset
		}
		if err := utils.CheckOssPathExist(ctr.OSSClient, meta.Revisions[0].SdsURI); err != nil {
			log.WithError(err).Error("failed to check SDS URI")
			return errors.ErrorInvalidDataset
		}

		classes := getRevisionClasses(meta.Revisions[0].MetaStat)

		*wrapDatasets = append(*wrapDatasets, &types.Dataset{
			SourceType: dataset.SourceType,
			Type:       dataset.Type,
			SDSPath:    meta.Revisions[0].SdsURI,
			DatasetMeta: &types.DatasetMeta{
				ID:           ids[0],
				Name:         meta.Name,
				RevisionID:   ids[1],
				RevisionName: meta.Revisions[0].RevisionName,
				OriginLevel:  dataset.OriginLevel,
				MSGPath:      meta.Revisions[0].MetaEntityURI,
			},
			Classes: classes,
		})
	case types.SourceTypePair:
		ids := strings.Split(dataset.DataURI, ",")
		if len(ids) != 2 {
			return errors.ErrorInvalidParams
		}
		pair, err := client.GetPairRevision(gc, ids[0], ids[1], dataset.OriginLevel)
		if err != nil {
			log.WithError(err).WithField("Pair", dataset).Error("failed to get pair")
			return errors.ErrorDataHubService
		}

		if pair.Revisions[0].PublishState != datahubType.PublishStatePublished {
			log.WithError(err).WithField("pair", dataset).Error("pair revision is not published")
			return errors.ErrorInvalidDataset
		}
		if err := utils.CheckOssPathExist(ctr.OSSClient, pair.Revisions[0].TrainDatasetRef.SDSURI); err != nil {
			log.WithError(err).Error("failed to check TrainDatasetRef SDS URI")
			return errors.ErrorInvalidDataset
		}
		if err := utils.CheckOssPathExist(ctr.OSSClient, pair.Revisions[0].ValDatasetRef.SDSURI); err != nil {
			log.WithError(err).Error("failed to check ValDatasetRef SDS URI")
			return errors.ErrorInvalidDataset
		}

		trainMeta, respCode, err := ctr.DatahubClient.GetDataset(gc,
			pair.Revisions[0].TrainDatasetRef.DatasetID.Hex(),
			pair.Revisions[0].TrainDatasetRef.RevisionID.Hex(),
			dataset.OriginLevel)
		if err != nil {
			log.WithError(err).WithField("dataset", dataset).Error("failed to get pair dataset")
			if respCode == http.StatusNotFound {
				return errors.ErrorDatasetNotFound
			}
			return errors.ErrorDataHubService
		}

		valMeta, respCode, err := ctr.DatahubClient.GetDataset(gc,
			pair.Revisions[0].ValDatasetRef.DatasetID.Hex(),
			pair.Revisions[0].ValDatasetRef.RevisionID.Hex(),
			dataset.OriginLevel)
		if err != nil {
			log.WithError(err).WithField("dataset", dataset).Error("failed to get pair dataset")
			if respCode == http.StatusNotFound {
				return errors.ErrorDatasetNotFound
			}
			return errors.ErrorDataHubService
		}

		trainClasses := getRevisionClasses(trainMeta.Revisions[0].MetaStat)
		valClasses := getRevisionClasses(valMeta.Revisions[0].MetaStat)

		// 确保第一个为训练集， 第二个为验证集
		*wrapDatasets = append(*wrapDatasets,
			&types.Dataset{
				SourceType: dataset.SourceType,
				Type:       aisConst.DatasetTypeTrain,
				SDSPath:    pair.Revisions[0].TrainDatasetRef.SDSURI,
				DatasetMeta: &types.DatasetMeta{
					ID:               pair.Revisions[0].TrainDatasetRef.DatasetID.Hex(),
					RevisionID:       pair.Revisions[0].TrainDatasetRef.RevisionID.Hex(),
					PairID:           pair.ID.Hex(),
					PairName:         pair.Name,
					PairRevisionID:   pair.Revisions[0].ID.Hex(),
					PairRevisionName: pair.Revisions[0].Name,
					OriginLevel:      dataset.OriginLevel,
				},
				Classes: trainClasses,
			},
			&types.Dataset{
				SourceType: dataset.SourceType,
				Type:       aisConst.DatasetTypeValid,
				SDSPath:    pair.Revisions[0].ValDatasetRef.SDSURI,
				DatasetMeta: &types.DatasetMeta{
					ID:               pair.Revisions[0].ValDatasetRef.DatasetID.Hex(),
					RevisionID:       pair.Revisions[0].ValDatasetRef.RevisionID.Hex(),
					PairID:           pair.ID.Hex(),
					PairName:         pair.Name,
					PairRevisionID:   pair.Revisions[0].ID.Hex(),
					PairRevisionName: pair.Revisions[0].Name,
					OriginLevel:      dataset.OriginLevel,
				},
				Classes: valClasses,
			})
	case types.SourceTypeSDS:
		if path.Ext(dataset.DataURI) != config.SdsSuffix {
			log.WithField("dataURI", dataset.DataURI).Error("only support file with extension sds")
			return errors.ErrorFileType
		}

		if err := utils.CheckOssPathExist(ctr.OSSClient, dataset.DataURI); err != nil {
			log.WithError(err).Error("failed to check oss URI")
			return err
		}

		*wrapDatasets = append(*wrapDatasets, &types.Dataset{
			SourceType: dataset.SourceType,
			Type:       dataset.Type,
			SDSPath:    dataset.DataURI,
			DatasetMeta: &types.DatasetMeta{
				Name: dataset.DataURI,
			},
		})
	default:
		log.Errorf("invalid source type: %s", dataset.SourceType)
		return errors.ErrorInvalidParams
	}

	return nil
}

// CheckAndSpeedDataset 校验数据集内容是否符合要求，并对数据集进行 nori 加速
func (ctr *Controller) CheckAndSpeedDataset(autoLearn *types.AutoLearn, rv *types.AutoLearnRevision) error {
	logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, rv.RevisionID))
	basicInfo := &dao.BasicInfo{
		Project:     autoLearn.ProjectID,
		AutoLearnID: rv.AutoLearnID,
		RevisionID:  rv.RevisionID,
	}

	var eventCode types.StateEventCode

	defer func() {
		if eventCode > types.StateEventCodeNothing {
			logs.Info(config.MetricAutoLearnFailedReason)
			utils.MetricEventDump(basicInfo.AutoLearnID, basicInfo.RevisionID, rv.Type, config.MetricAutoLearnFailedReason, map[string]any{
				utils.LogFieldFailedReason:  types.EventCodeMessage[eventCode],
				utils.LogFieldSnapXImageURI: rv.SnapXImageURI,
				utils.LogFieldManagedBy:     rv.ManagedBy,
				utils.LogFieldRvCreatedAt:   time.Unix(rv.CreatedAt, 0).Format(time.RFC3339),
			})
		}
	}()

	d := dao.NewDAO(ctr.MongoClient, autoLearn.TenantID)

	event := &types.StateEvent{T: types.StateEventTypeInit, A: types.EventActionDoing}

	// 如果为优化任务，先进入 initial
	if rv.IsOptimized {
		if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeNothing); err != nil {
			log.WithError(err).Error("change optimization task state failed")
			return err
		}
	}

	event.T = types.StateEventTypeDatasetCheck
	if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeNothing); err != nil {
		return err
	}

	// 1. check type and get volume
	// LLM 不需要检查
	if rv.Approach != aisConst.ApproachAIGC {
		var wg sync.WaitGroup
		logs.Info("step1: convert sds and parse volumeID")
		for i := range rv.Datasets {
			wg.Add(1)
			go ctr.ReadDatasetInfo(&wg, rv.Datasets[i], rv.Type)
		}
		wg.Wait()

		if rv.Datasets[0].SourceType == types.SourceTypeSDS {
			// 如果数据集classess 匹配，则需要将本次 sds  classes 刷入 db
			if err := dao.UpdateRevisionSDSDatasetClasses(dao.NewDAO(ctr.MongoClient, autoLearn.TenantID), rv); err != nil {
				event.A = types.EventActionFailed
				ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeContinuedLearnGenerateClassFailed)
				logs.WithError(err).Error("step1: save sds classess to db failed")
				return errors.ErrorInternal
			}
		}

		// [snapdet错误]: 数据校验失败，请检查sds格式
		// 2. checkDatasetParseIsFailed 检查解析过程是否出错
		if CheckDatasetParseIsFailed(rv.Datasets) {
			event.A = types.EventActionFailed
			ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeDatasetCheckFailed)
			logs.Error("step2: preparatory work ended due to parsing sds file failed")
			return fmt.Errorf("failed to parse dataset sds file")
		}

		// 3. checkDatasetTypeIsValid 校验数据类型与训练类型是否一致
		if !CheckDatasetTypeIsValid(rv.Type, rv.Datasets) {
			event.A = types.EventActionFailed
			if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeDatasetInvalidFormat); err != nil {
				return err
			}
			logs.Error("step2: preparatory work ended due to inconsistent dataset type")
			return fmt.Errorf("inconsistent dataset type")
		}
	} else {
		for i := range rv.Datasets {
			rv.Datasets[i].IsConverstation = true
		}
	}

	event.A = types.EventActionSuccess
	if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeNothing); err != nil {
		logs.WithError(err).Error("step2: preparatory work ended due to update autoLearn state to AutoLearnStateConvertSucceeded")
		return err
	}

	event = &types.StateEvent{T: types.StateEventTypeSpeed, A: types.EventActionDoing}
	if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeNothing); err != nil {
		logs.WithError(err).Error("step3: preparatory work ended due to update autoLearn state to AutoLearnStateSpeeding")
		return err
	}

	// 5. 数据开始加速
	// LLM 数据集不加速
	if rv.Approach != aisConst.ApproachAIGC {
		logs.Info("step3: speedup nori data")
		authParam := &publicService.AuthParam{
			AuthToken: authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey),
			ProjectID: autoLearn.ProjectID,
			TenantID:  autoLearn.TenantID,
		}
		speedupTaskIDs := make([]string, 0)
		for i := range rv.Datasets {
			spdTask, err := ctr.PublicServiceClient.CreateSpeedUp(rv.Datasets[i].SDSPath, authParam)
			if err != nil {
				event.A = types.EventActionFailed
				if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeSpeedFailed); err != nil {
					logs.WithError(err).Error("step3: preparatory work ended due to update autoLearn state to AutoLearnStateSpeeding")
					return err
				}

				logs.WithError(err).Error("step4: preparatory work ended due to call publicservice speed failed")
				return err
			}
			speedupTaskIDs = append(speedupTaskIDs, spdTask.ID.Hex())
		}

		// 7. 更新加速状态
		logs.Info("step4: update nori state periodical")
		state := querySpeedStateV2(ctr.PublicServiceClient, authParam, speedupTaskIDs, logs)
		if state == types.AutoLearnStateSpeedFailed {
			event.A = types.EventActionFailed
			if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeSpeedFailed); err != nil {
				logs.WithError(err).Error("step5: preparatory work ended due to update autoLearn state to AutoLearnStateSpeeding")
				return err
			}

			logs.Error("step5: preparatory work ended due to nori speedup failed")
			return fmt.Errorf("nori speed failed")
		}
		if state == types.AutoLearnStateSpeedSucceeded {
			event.A = types.EventActionSuccess
			if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeNothing); err != nil {
				logs.WithError(err).Error("step5: preparatory work ended due to update autoLearn state to AutoLearnStateSpeeding")
				return err
			}
		}
	} else {
		event.A = types.EventActionSuccess
		if err := ctr.StateMachine.Action(d, event, rv.RevisionID, types.StateEventCodeNothing); err != nil {
			logs.WithError(err).Error("step5: preparatory work ended due to update autoLearn state to AutoLearnStateSpeeding")
			return err
		}
	}

	return nil
}

// ReadDatasetInfo 读取数据集(sds)文件内容
func (ctr *Controller) ReadDatasetInfo(wg *sync.WaitGroup, dataset *types.Dataset, trainType aisConst.AppType) {
	defer wg.Done()
	logs := log.WithField(config.DataSetPrefix, fmt.Sprintf("%s/%s", dataset.Type, dataset.SDSPath))

	fsf := sds.NewFastSDSFile(dataset.SDSPath, ctr.OSSClient.GetClientSession(), logs, ctr.NoriClient)
	defer fsf.Close()

	if err := fsf.ParseAndLocate(context.Background()); err != nil || len(fsf.GetItems()) == 0 {
		log.WithError(err).Error("failed to parser sds")
		dataset.ParseFailed = true
	}

	// 新逻辑判断
	// 对于分类任务：不能没有 classes 字段，也不能 classes 字段为空
	// 对于检测任务：不能没有 boxes 字段，但 boxes 字段可以为空
	// 对于关键点任务：得有 boxes 字段，可以部分为空数组，对于不为空的 box，不能没有 keypoint字段，class_name 不能为空,
	// 对于 SDS 类型的任务，由于继续学习需要 classes，所以需要统计
	// 对于回归任务，regressors 存在且不能为空
	recommendClasses := types.RecommendClasses{
		Boxes:         map[string]int64{},
		Classes:       map[string]int64{},
		Regressors:    map[string]int64{},
		Segmentations: map[string]int64{},
		Keypoints:     map[string]int64{},
		Texts:         map[string]int64{},
	}
	recommendClasses.Count = len(fsf.GetItems())
	classes := map[string]bool{}
	hasKeypoint := false
	for _, item := range fsf.GetItems() {
		if len(item.Classes) == 0 {
			dataset.HasNilOrEmptyClasses = true
		}

		if len(item.Regressors) > 0 {
			dataset.HasRegressor = true
			for _, reg := range item.Regressors {
				recommendClasses.Regressors[reg.ClassName]++
			}
		}

		if item.Boxes == nil {
			dataset.HasNilBoxes = true
		}

		if item.Texts != nil {
			dataset.HasTexts = true
		}

		if len(item.Boxes) > 0 {
			for _, box := range item.Boxes {
				var clName string
				if box.ClassName != nil && *box.ClassName != "" {
					if len(strings.Split(*box.ClassName, ".")) > 1 {
						classes[*box.ClassName] = true
						clName = *box.ClassName
					} else {
						clName = fmt.Sprintf("default.%s", *box.ClassName)
						classes[clName] = true
					}
				}

				if box.Type != sds.BoxTypeIgnore {
					if box.KeyPoints != nil {
						hasKeypoint = true
					}
					if len(box.KeyPoints) > 0 {
						recommendClasses.Keypoints[clName]++
					}

					recommendClasses.Boxes[clName]++
					if box.ClassName == nil || *(box.ClassName) == "" {
						dataset.NilKeypoints = true
					}
				}
			}
		}

		if len(item.Classes) > 0 {
			dataset.HasClasses = true
			for _, class := range item.Classes {
				if class.ClassName != "" {
					if len(strings.Split(class.ClassName, ".")) > 1 {
						classes[class.ClassName] = true
						recommendClasses.Classes[class.ClassName]++
					} else {
						cl := fmt.Sprintf("default.%s", class.ClassName)
						classes[cl] = true
						recommendClasses.Classes[cl]++
					}
				}
			}
		}

		if len(item.Segmentations) > 0 {
			for _, seg := range item.Segmentations {
				if seg.ClassName != nil {
					classes[*seg.ClassName] = true
					recommendClasses.Segmentations[*seg.ClassName]++
				}
			}
		}

		if len(item.Texts) > 0 {
			classes["text"] = true
			recommendClasses.Texts["text"]++
		}

		if dataset.SourceType != types.SourceTypeSDS &&
			trainType == aisConst.AppTypeClassification &&
			dataset.HasNilOrEmptyClasses {
			break
		}

		if dataset.SourceType != types.SourceTypeSDS &&
			trainType == aisConst.AppTypeDetection &&
			dataset.HasNilBoxes {
			break
		}

		if trainType == aisConst.AppTypeKPS && dataset.NilKeypoints {
			break
		}

		if trainType == aisConst.AppTypeRegression && dataset.HasRegressor {
			break
		}

		if trainType == aisConst.AppTypeClassificationAndRegression && dataset.HasRegressor && dataset.HasClasses {
			break
		}

		if trainType == aisConst.AppTypeOCR && dataset.HasTexts {
			break
		}
	}
	if trainType == aisConst.AppTypeKPS && !hasKeypoint {
		dataset.NilKeypoints = true
	}

	if dataset.SourceType == types.SourceTypeSDS {
		var classesArray []string
		for k := range classes {
			classesArray = append(classesArray, k)
		}
		dataset.Classes = classesArray
	}
	dataset.RecommendClasses = &recommendClasses
}

// CheckSpeedMetricValid 检查速度指标是否符合要求
func CheckSpeedMetricValid(metric *types.SpeedMetric) error {
	if metric == nil {
		return fmt.Errorf("speed metric is null")
	}

	if metric.ProcessTimePerImg != nil {
		roundUum, _ := decimal.NewFromFloat(*metric.ProcessTimePerImg).Round(config.ProcessTimePerImgPrecision).Float64()
		metric.ProcessTimePerImg = &roundUum
		if *metric.ProcessTimePerImg < 0 {
			return fmt.Errorf("invalid processTimePerImg: %f", *metric.ProcessTimePerImg)
		}
	}

	if metric.GPUModel != types.GPUModelType2080Ti && metric.GPUModel != types.GPUModelTypeT4 {
		return fmt.Errorf("invalid gpu model: %s", metric.GPUModel)
	}

	if metric.ImageSpec != types.ImageSpecType521 && metric.ImageSpec != types.ImageSpecType768 && metric.ImageSpec != types.ImageSpecType1080 {
		return fmt.Errorf("invalid image spec: %s", metric.ImageSpec)
	}

	return nil
}

func CheckDatasetParseIsFailed(datasets []*types.Dataset) bool {
	for i := range datasets {
		if datasets[i].ParseFailed {
			return true
		}
	}

	return false
}

func CheckDatasetTypeIsValid(trainType aisConst.AppType, datasets []*types.Dataset) bool {
	switch trainType {
	case aisConst.AppTypeClassification:
		for i := range datasets {
			if datasets[i].Type == aisConst.DatasetTypeValid {
				if datasets[i].HasNilOrEmptyClasses {
					return false
				}
			}
		}
	case aisConst.AppTypeDetection:
		for i := range datasets {
			if datasets[i].HasNilBoxes {
				return false
			}
		}
	case aisConst.AppTypeKPS:
		for i := range datasets {
			if datasets[i].NilKeypoints {
				return false
			}
		}
	case aisConst.AppTypeOCR:
		for i := range datasets {
			if !datasets[i].HasTexts {
				return false
			}
		}
	}

	if trainType == aisConst.AppTypeRegression || trainType == aisConst.AppTypeClassificationAndRegression {
		var trainHasRegressor, valHasRegressor, trainHasClasses, valHasClasses bool
		for i := range datasets {
			if datasets[i].Type == aisConst.DatasetTypeTrain {
				if datasets[i].HasRegressor {
					trainHasRegressor = true
				}
				if datasets[i].HasClasses {
					trainHasClasses = true
				}
			}
			if datasets[i].Type == aisConst.DatasetTypeValid {
				if datasets[i].HasRegressor {
					valHasRegressor = true
				}
				if datasets[i].HasClasses {
					valHasClasses = true
				}
			}
		}
		if trainType == aisConst.AppTypeRegression {
			return trainHasRegressor && valHasRegressor
		}
		if trainType == aisConst.AppTypeClassificationAndRegression {
			return trainHasClasses && trainHasRegressor && valHasClasses && valHasRegressor
		}
	}

	return true
}

func querySpeedStateV2(pc publicService.Client, authParam *publicService.AuthParam, speedUpTaskIDs []string, logs *log.Entry) types.AutoLearnState {
	allCompleted := false
	for i := 0; i < int(config.Profile.NoriServer.StateQueryMaxRetry); i++ {
		for j := range speedUpTaskIDs {
			detail, err := pc.GetSpeedUpDetail(speedUpTaskIDs[j], authParam)
			if err != nil {
				allCompleted = false
				logs.WithError(err).Error("failed to query speed state, continue with the next attempt")
				continue
			}

			if detail.State == psSpeedupType.StateFailed {
				logs.Infof("at least one speed failed, speedup taskID %s", speedUpTaskIDs[j])
				return types.AutoLearnStateSpeedFailed
			}

			if detail.State == psSpeedupType.StateSuccess {
				allCompleted = true
			} else {
				allCompleted = false
			}

			logs.Infof("speed in process, speedup taskID %s: %d", speedUpTaskIDs[j], detail.GetProgress())
		}

		if allCompleted {
			return types.AutoLearnStateSpeedSucceeded
		}
		time.Sleep(time.Second * time.Duration(config.Profile.NoriServer.StateQueryInterval))
	}

	return types.AutoLearnStateSpeedFailed
}

// GetDatasetAttributeType 获取分类数据集属性类型，单属性or多属性
func GetDatasetAttributeType(attributes [][]string) string {
	// 属性可能格式为a.c1, a.c2, b.c1, b.c2, c1, c2中的一种或多种，不会为空
	attributeType := types.DatasetAttributeTypeSingleAttr
	for i := range attributes {
		temp := make(map[string]bool)
		for j := range attributes[i] {
			if strings.TrimSpace(attributes[i][j]) == "" {
				continue
			}

			arr := strings.Split(attributes[i][j], ".")
			if temp[arr[0]] {
				continue
			}
			if len(arr) == 1 {
				temp[""] = true
			} else {
				temp[arr[0]] = true
			}
		}
		if len(temp) > 1 {
			attributeType = types.DatasetAttributeTypeMultiAttribute
			break
		}
	}

	return attributeType
}

func UpdateRevisionReasonAndSendMetricEvent(d *dao.DAO, basicInfo *dao.BasicInfo, errReason string, appType aisConst.AppType, rvCreatedAt int64) error {
	log.WithFields(log.Fields{"autoLearnID": basicInfo.AutoLearnID, "revisionID": basicInfo.RevisionID}).Info(config.MetricAutoLearnFailedReason)
	utils.MetricEventDump(basicInfo.AutoLearnID, basicInfo.RevisionID, appType, config.MetricAutoLearnFailedReason, map[string]any{
		utils.LogFieldFailedReason: errReason,
		utils.LogFieldRvCreatedAt:  time.Unix(rvCreatedAt, 0).Format(time.RFC3339),
	})
	return dao.UpdateALRevisionReason(d, basicInfo, errReason)
}

func getRevisionClasses(metaStat datahubType.MetaStat) []string {
	var classes []string
	if metaStat.Classes != nil {
		for _, attr := range metaStat.Classes {
			if attr.AttributeName != "" && attr.AttributeName != "unlabel" {
				for _, attrValue := range attr.AttributeValueStats {
					if attrValue.AttributeValue != "null" && attrValue.AttributeValue != "" {
						if len(strings.Split(attrValue.AttributeValue, ".")) > 1 {
							classes = append(classes, attrValue.AttributeValue)
						} else {
							classes = append(classes, fmt.Sprintf("%s.%s", attr.AttributeName, attrValue.AttributeValue))
						}
					}
				}
			}
		}
	}

	if metaStat.Boxes != nil {
		for _, attr := range metaStat.Boxes {
			if attr.AttributeName != "" && attr.AttributeName != "unlabel" {
				for _, attrValue := range attr.AttributeValueStats {
					if attrValue.AttributeValue != "null" && attrValue.AttributeValue != "" {
						if len(strings.Split(attrValue.AttributeValue, ".")) > 1 {
							classes = append(classes, attrValue.AttributeValue)
						} else {
							classes = append(classes, fmt.Sprintf("%s.%s", attr.AttributeName, attrValue.AttributeValue))
						}
					}
				}
			}
		}
	}

	return classes
}
