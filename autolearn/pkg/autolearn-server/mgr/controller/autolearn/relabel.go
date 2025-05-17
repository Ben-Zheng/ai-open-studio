package autolearn

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	reLabelAPI "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/relabel/api/v1"
	reLabelType "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/relabel/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

func CreateAndCheckReLabelTask(gc *ginlib.GinContext, mgoClient *mgolib.MgoClient, ossClient *oss.Client, rv *types.AutoLearnRevision, autoLearnName, trainSDS string, evalDetail *types.EvaluationDetail) error {
	logger := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", rv.AutoLearnID, rv.RevisionID))
	basicInfo := &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: rv.AutoLearnID,
		RevisionID:  rv.RevisionID,
	}

	// 1 创建
	reLabelTaskID, err := CreateReLabelTask(gc, rv, autoLearnName, trainSDS, evalDetail)
	if err != nil {
		logger.WithError(err).Error("step2: preparatory work ended due to creating reLabel task failed")
		// TODO 更新状态为失败
		if e := dao.UpdateALRevisionState(dao.NewDAO(mgoClient, gc.GetAuthTenantID()), basicInfo, types.AutoLearnStateOptimizeFailed, true); e != nil {
			logger.WithError(err).Error("failed to update state to AutoLearnStateOptimizeFailed")
		}
		if e := UpdateRevisionReasonAndSendMetricEvent(dao.NewDAO(mgoClient, gc.GetAuthTenantID()), basicInfo, "创建数据集优化任务失败，请咨询客服", rv.Type, rv.CreatedAt); e != nil {
			logger.WithError(e).Error("failed to update failed reason")
		}
		return err
	}

	logger.Infof("[INFO]reLabelTaskID: %s", reLabelTaskID.Hex())

	if err := dao.UpdateALRevisionReLabelTaskID(dao.NewDAO(mgoClient, gc.GetAuthTenantID()), basicInfo, reLabelTaskID); err != nil {
		logger.WithError(err).Error("step2: preparatory work ended due to updating reLabel taskID")
		return err
	}

	// 2 查询(FIXME: 后续修改为异步回调的形式)
	var dataset *types.Dataset
	state, reason, RelabeledTrainSDS := GetReLabelTaskStateAndTrainSDS(gc, ossClient, reLabelTaskID, logger)
	defer func() {
		if state == types.AutoLearnStateOptimizeFailed {
			logger.Info(config.MetricAutoLearnFailedReason)
			utils.MetricEventDump(basicInfo.AutoLearnID, basicInfo.RevisionID, rv.Type, config.MetricAutoLearnFailedReason, map[string]any{
				utils.LogFieldFailedReason:  reason,
				utils.LogFieldSnapXImageURI: rv.SnapXImageURI,
				utils.LogFieldManagedBy:     rv.ManagedBy,
				utils.LogFieldRvCreatedAt:   time.Unix(rv.CreatedAt, 0).Format(time.RFC3339),
			})
		}
	}()
	if state == types.AutoLearnStateOptimizeSucceeded {
		dataset = &types.Dataset{
			SourceType:  types.SourceTypeSDS,
			Type:        aisTypes.DatasetTypeTrain,
			SDSPath:     RelabeledTrainSDS,
			DatasetMeta: &types.DatasetMeta{Name: RelabeledTrainSDS},
		}
		utils.MetricEventDump(rv.AutoLearnID, rv.RevisionID, rv.Type, config.MetricRelabelSucceeded, nil)
	}

	// 3 更新
	if err := dao.UpdateALRevisionStateAndTrainData(mgoClient, gc.GetAuthTenantID(), basicInfo, state, dataset); err != nil {
		logger.WithError(err).Errorf("step2: preparatory work ended due to update ALRevision state and trainData: %d(%s)", int8(state), RelabeledTrainSDS)
		return err
	}
	if state == types.AutoLearnStateOptimizeFailed {
		_ = dao.UpdateALRevisionReason(dao.NewDAO(mgoClient, gc.GetAuthTenantID()), basicInfo, reason)
		logger.Error("step2: preparatory work ended due to reLabel task is failed")
		return fmt.Errorf("reLabel task is failed")
	}

	// 4 回填
	rv.Datasets = append(rv.Datasets, dataset)

	return nil
}

func CreateReLabelTask(gc *ginlib.GinContext, rv *types.AutoLearnRevision, autoLearnName, trainSDS string, evalDetail *types.EvaluationDetail) (primitive.ObjectID, error) {
	req := &reLabelAPI.CreateTaskRequest{
		Name:        utils.GenerateNameForOtherService(autoLearnName, rv.RevisionName, rv.RevisionID),
		AppType:     rv.Type,
		Description: "I come from autoLearn",
		Tags:        []string{"AutoLearn"},
		SourceType:  reLabelType.SourceTypeModel,
		ReLabelType: reLabelType.ReLabelTypeAuto,
		Dataset: &reLabelType.BriefDataset{
			SourceType: reLabelType.DatasetSourceTypeSDS,
			SDSPath:    trainSDS,
		},
		Model: &reLabelType.BriefModel{
			SourceType: reLabelType.ModelSourceTypeS3,
			S3Path:     evalDetail.ModelFilePath,
		},
		Codebase: &reLabelType.BriefCodebase{
			OriginLevel: aisTypes.PublicLevel,
			TenantID:    aisTypes.SystemLevelSourceTenantID,
			CodeID:      config.Profile.InferCodebase.CodeID,
			CodeName:    config.Profile.InferCodebase.CodeName,
		},
		AutoLearnTrainFrom: &reLabelType.AutoLearnTrainFrom{
			AutoLearnID:      rv.AutoLearnID,
			RevisionID:       rv.RevisionID,
			AutoLearnName:    rv.AutoLearnName,
			RevisionName:     rv.RevisionName,
			SolutionType:     string(rv.SolutionType),
			IterationNumber:  evalDetail.IterationNumber,
			SolutionImageURI: evalDetail.SolutionImageURI,
			AlgorithmName:    evalDetail.AlgorithmName,
			SnapXImageURI:    rv.SnapXImageURI,
		},
	}
	// 这里不指定资源了，Relabel内部配置决定

	if rv.QuotaGroup != "" {
		temp := strings.Split(rv.QuotaGroup, ":")
		req.QuotaConfig = &reLabelType.QuotaConfig{QuotaGroup: temp[len(temp)-1], QuotaBestEffort: "no", QuotaPreemptible: "no"}
	}

	utils.MetricEventDump(rv.AutoLearnID, rv.RevisionID, rv.Type, config.MetricRelabelRequest, nil)

	task, err := reLabelAPI.NewClient(config.Profile.AisEndPoint.DatahubRelabel).CreateReLabelTask(gc, req)
	if err != nil {
		return primitive.NilObjectID, err
	}

	return task.ID, nil
}

func GetReLabelTaskStateAndTrainSDS(gc *ginlib.GinContext, ossClient *oss.Client, taskID primitive.ObjectID, logger *log.Entry) (types.AutoLearnState, string, string) {
	ticker := time.NewTicker(config.ReLabelTaskStateCheckInterval * time.Second)
	defer ticker.Stop()

	c := reLabelAPI.NewClient(config.Profile.AisEndPoint.DatahubRelabel)

	state := types.AutoLearnStateOptimizeFailed
	trainSDS := ""
	reason := ""
	stopChan := make(chan struct{})

	gc = ginlib.Copy(gc)
	go func() {
		for range ticker.C {
			task, err := c.GetReLabelTask(gc, authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey), taskID)
			if err != nil {
				logger.WithError(err).Error("failed to get reLabel task")
				continue
			}

			logger.Infof("[INFO]reLabel task state is %s(%s)", task.State, task.RelabeledSDSURI)

			if reLabelTaskInFailedState(task.State) {
				state = types.AutoLearnStateOptimizeFailed
				reason = task.Reason
				close(stopChan)
			}
			if task.State == reLabelType.RelabelTaskStateRelabelSuccessed {
				state = types.AutoLearnStateOptimizeSucceeded
				trainSDS = task.RelabeledSDSURI
				if err := utils.CheckOssPathExist(ossClient, trainSDS); err != nil {
					logger.WithError(err).Errorf("reLabelSDS not found: %s", trainSDS)
					state = types.AutoLearnStateOptimizeFailed
					reason = "优化后的训练集不存在，请咨询客服"
				}
				close(stopChan)
			}
		}
	}()

	select {
	case <-time.After(config.ReLabelTaskStateCheckTime * time.Second):
		logger.Warnf("query reLabel task timeout: %s", taskID.Hex())
		reason = "查询超时，请咨询客服"
		return state, reason, trainSDS
	case <-stopChan:
		return state, reason, trainSDS
	}
}

// reLabelTaskInFailedState 该方法应该由reLabel服务提供 TODO
func reLabelTaskInFailedState(state reLabelType.RelabelTaskStateType) bool {
	return state == reLabelType.RelabelTaskStateInferFailed || state == reLabelType.RelabelTaskStateRelabelFailed
}
