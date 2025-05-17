package autolearn

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/k8s"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/watcher"
	outerClient "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	alUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	aisclient "go.megvii-inc.com/brain/brainpp/projects/aiservice/components/pkg/client/clientset/versioned"
	datahubV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	noriUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/nori"
	publicService "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg"
	schedulerv1alpha1 "go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/client/clientset/versioned/typed/gang/v1alpha1"
)

type Controller struct {
	StateMachine        *watcher.StateMachine
	MongoClient         *mgolib.MgoClient
	NoriClient          noriUtils.Client
	OSSClient           *oss.Client
	SchedulerClient     *schedulerv1alpha1.SchedulerV1alpha1Client
	KubeClient          *kubernetes.Clientset
	DatahubClient       datahubV1.Client
	PublicServiceClient publicService.Client
}

func NewController(
	mongoClient *mgolib.MgoClient,
	ossClient *oss.Client,
	st *watcher.StateMachine,
	scheduler *schedulerv1alpha1.SchedulerV1alpha1Client,
	kubeClient *kubernetes.Clientset,
	noriControllerClient noriUtils.Client,
	datahubClient datahubV1.Client,
	publicClient publicService.Client,
) *Controller {
	return &Controller{
		StateMachine:        st,
		MongoClient:         mongoClient,
		OSSClient:           ossClient,
		SchedulerClient:     scheduler,
		KubeClient:          kubeClient,
		NoriClient:          noriControllerClient,
		DatahubClient:       datahubClient,
		PublicServiceClient: publicClient,
	}
}

// CreatePreCheckAndGetDatasetInfo 创建自动学习版本前的综合校验，以及填充数据集信息
func (ctr *Controller) CreatePreCheckAndGetDatasetInfo(
	autolearn *types.AutoLearn,
	gc *ginlib.GinContext,
	request *types.SameCreateRequest,
	datasets *[]*types.Dataset,
	debug bool) error {
	// 0 场景类型校验
	if !alUtils.AppTypeAndApproachIsValid(request) {
		log.Errorf("autoLearn not support type %s now", request.Type)
		return errors.ErrorInvalidParams
	}

	// 校验模式信息
	if err := preCheckMode(request); err != nil {
		log.WithError(err).Error("failed to preCheckMode")
		return errors.ErrorInvalidParams
	}

	// 校验评测指标信息
	if err := preCheckEvalMetric(request); err != nil {
		log.WithError(err).Error("failed to preCheckEvalMetric")
		return errors.ErrorInvalidParams
	}

	// 校验是否符合继续学习要求
	if err := preCheckContinuedLearn(autolearn, request); err != nil {
		log.WithError(err).Error("failed to preCheckContinuedLearn")
		return errors.ErrorInvalidParams
	}

	// 校验算法信息
	if err := preCheckAlgorithm(request); err != nil {
		log.WithError(err).Error("failed to preCheckAlgorithm")
		return errors.ErrorInvalidParams
	}

	// 5 校验数据来源、补充信息
	err := ctr.PreCheckAndWrapDatasetsV2(gc, request.Datasets, datasets)
	if err != nil {
		log.WithError(err).Error("failed to preCheckAndWrapDatasets")
		return err
	}

	// 校验可等待调度时长
	request.ScheduleTimeLimit, _ = decimal.NewFromFloat(request.ScheduleTimeLimit).Round(config.ScheduleTimeLimitPrecision).Float64()
	if request.ScheduleTimeLimit < config.MinScheduleTime || request.ScheduleTimeLimit > config.MaxScheduleTime {
		log.Errorf("invalid scheduleTimeLimit: %f", request.ScheduleTimeLimit)
		return errors.ErrorInvalidParams
	}

	if request.Type == aisConst.AppTypeDetection {
		// 校验速度指标信息
		if err := CheckSpeedMetricValid(request.SpeedMetric); err != nil {
			log.WithError(err).Error("failed to check speed metric")
			return errors.ErrorInvalidParams
		}

		// 校验目标适用参数
		if !checkDetectedObjectType(request.DetectedObjectType) {
			log.Errorf("invalid detected object type: %s", request.DetectedObjectType)
			return errors.ErrorInvalidParams
		}
	}

	// 填充GPU类型信息，使solution在指定GPU机器上运行
	if len(request.Algorithms) != 0 && len(request.GPUTypes) != 0 {
		for i := range request.Algorithms {
			alg := make(map[string]interface{})
			if err = json.Unmarshal([]byte(request.Algorithms[i].OriginalInfo), &alg); err != nil {
				log.WithError(err).Error("unmarshal algorithms originalInfo into map failed")
				return errors.ErrorInvalidParams
			}

			var ok bool
			envs := make(map[string]string)
			if alg["envs"] != nil {
				envs, ok = alg["envs"].(map[string]string)
				if !ok {
					log.Error("algorithms envs field is not map")
					return errors.ErrorInvalidParams
				}
			}
			envs[config.AvailableGPUTypeEnv] = strings.Join(request.GPUTypes, ",")
			alg["envs"] = envs

			byteArr, err := json.Marshal(alg)
			if err != nil {
				log.WithError(err).Error("failed to marshal modify alglrithms")
				return errors.ErrorInternal
			}
			request.Algorithms[i].OriginalInfo = string(byteArr)
		}
	}

	return nil
}

// preCheckMode 检查创建模式及相关的参数值
func preCheckMode(request *types.SameCreateRequest) error {
	switch request.CreateMode {
	case types.CreateModeSimple:
		// 防止复制版本时老版算法信息污染
		request.Algorithms = nil
		request.OriginalAlgorithms = nil
		if request.Approach == aisConst.ApproachCV {
			request.TimeLimit = types.CVSimpleDefaultTimeout
		}
		if request.Approach == aisConst.ApproachAIGC {
			request.TimeLimit = types.AIGCSimpleDefaultTimeout
		}
	case types.CreateModeAdvance:
		request.TimeLimit, _ = decimal.NewFromFloat(request.TimeLimit).Round(config.TimeLimitPrecision).Float64()
		return CheckResourceValid(request.TimeLimit, request.WorkerNum)
	default:
		return fmt.Errorf("invalid createMode: %s", request.CreateMode)
	}

	return nil
}

// preCheckEvalMetric 校验评测指标信息
func preCheckEvalMetric(request *types.SameCreateRequest) error {
	switch request.Type {
	case aisConst.AppTypeClassification:
		request.EvalMetric = nil
		if err := checkClfEvalMetric(request); err != nil {
			return err
		}
	case aisConst.AppTypeDetection:
		request.ClfEvalMetric = nil
		if err := checkDetEvalMetric(request); err != nil {
			return err
		}
	default:
		if request.EvalMetric == nil && request.ClfEvalMetric == nil {
			return fmt.Errorf("eval metric is nil")
		}
	}

	return nil
}

func preCheckContinuedLearn(autolearn *types.AutoLearn, request *types.SameCreateRequest) error {
	if !request.Continued {
		return nil
	}

	if request.Continued && autolearn != nil && !autolearn.Continued {
		return errors.ErrorInvalidParams
	}

	if request.ContinuedMeta == nil {
		return errors.ErrorInvalidParams
	}

	if request.ContinuedMeta.ModelPath == "" || request.ContinuedMeta.SolutionName == "" {
		return errors.ErrorInvalidParams
	}

	if request.CreateMode == types.CreateModeAdvance {
		// 继续学习只能有一个算法
		if len(request.Algorithms) > 1 || request.WorkerNum > 1 {
			return errors.ErrorInvalidParams
		}
	}

	return nil
}

func PreCheckContinuedLearnClasses(autolearn *types.AutoLearn, datasets []*types.Dataset, revisionID string) error {
	var preRevision *types.AutoLearnRevision
	for _, revision := range autolearn.AutoLearnRevisions {
		if revision.RevisionID == revisionID {
			preRevision = revision
		}
	}

	preTrainClasses := map[string]bool{}
	preValClasses := map[string]bool{}

	for _, dataset := range preRevision.Datasets {
		if dataset.Type == aisConst.DatasetTypeTrain {
			for _, name := range dataset.Classes {
				preTrainClasses[name] = true
			}
		}

		if dataset.Type == aisConst.DatasetTypeValid {
			for _, name := range dataset.Classes {
				preValClasses[name] = true
			}
		}
	}
	for _, dataset := range datasets {
		if dataset.Type == aisConst.DatasetTypeTrain {
			for _, name := range dataset.Classes {
				if !preTrainClasses[name] {
					return errors.ErrorDatasetClassesNotMatch
				}
			}
		}

		if dataset.Type == aisConst.DatasetTypeValid {
			for _, name := range dataset.Classes {
				if !preValClasses[name] {
					return errors.ErrorDatasetClassesNotMatch
				}
			}
		}
	}

	return nil
}

// checkDetEvalMetric 校验检测类型的评测指标
func checkDetEvalMetric(request *types.SameCreateRequest) error {
	if request.EvalMetric == nil {
		return fmt.Errorf("eval detection metric is nil")
	}

	if request.EvalMetric.Key != types.EvalMetricKeyRecall && request.EvalMetric.Key != types.EvalMetricKeyPrecision {
		return fmt.Errorf("invalid detection eval metric key: %s", request.EvalMetric.Key)
	}

	request.EvalMetric.IOU, _ = decimal.NewFromFloat(request.EvalMetric.IOU).Round(config.EvalMetricPrecision).Float64()
	if request.EvalMetric.IOU < config.EvalMetricMinValue || request.EvalMetric.IOU > 1 {
		return fmt.Errorf("invalid detection eval metric iou：%f", request.EvalMetric.IOU)
	}

	if !checkEvalMetricConditionValid(request.EvalMetric.Condition) {
		return fmt.Errorf("invalid detection eval metric condition: %+v", request.EvalMetric.Condition)
	}

	return nil
}

// checkEvalMetricConditionValid 校验检测评测指标参数是否正确
func checkEvalMetricConditionValid(c *types.Condition) bool {
	if c == nil {
		return false
	}

	switch c.Key {
	case types.EvalMetricKeyFP:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		if v < 1 {
			return false
		}
	case types.EvalMetricKeyRecall:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue || v > 1 {
			return false
		}
	case types.EvalMetricKeyFppi:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue {
			return false
		}
	case types.EvalMetricKeyPrecision:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue || v > 1 {
			return false
		}
	case types.EvalMetricKeyConfidence:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue || v > 1 {
			return false
		}
	default:
		return false
	}

	return true
}

// checkClfEvalMetric 校验分类类型的评测指标
func checkClfEvalMetric(request *types.SameCreateRequest) error {
	if request.ClfEvalMetric == nil {
		return fmt.Errorf("classification eval metric is nil")
	}

	temp := map[types.EvalMetricKey]bool{
		types.EvalMetricKeyMeanF1Score: true, types.EvalMetricKeyAccuracy: true,
		types.EvalMetricKeyMeanRecall: true, types.EvalMetricKeyMeanPrecision: true,
		types.EvalMetricKeyPrecisionRecall: true, types.EvalMetricKeyRecallPrecision: true,
		types.EvalMetricKeyRecallConfidence: true,
	}
	if !temp[request.ClfEvalMetric.Key] {
		return fmt.Errorf("invalid classification eval metric key: %s", request.EvalMetric.Key)
	}

	if !checkClfEvalMetricConditionValid(request.ClfEvalMetric) {
		return fmt.Errorf("invalid clf eval metric condition: %+v", request.ClfEvalMetric)
	}

	return nil
}

// checkClfEvalMetricConditionValid 校验分类评测指标参数是否正确
func checkClfEvalMetricConditionValid(c *types.ClfEvalMetric) bool {
	if c == nil {
		return false
	}

	switch c.Key {
	case types.EvalMetricKeyPrecisionRecall:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue || v > 1 {
			return false
		}
	case types.EvalMetricKeyRecallPrecision:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue || v > 1 {
			return false
		}
	case types.EvalMetricKeyRecallConfidence:
		v, ok := c.Value.(float64)
		if !ok {
			return false
		}
		c.Value, _ = decimal.NewFromFloat(v).Round(config.EvalMetricPrecision).Float64()
		if c.Value.(float64) < config.EvalMetricMinValue || v > 1 {
			return false
		}
	case types.EvalMetricKeyMeanF1Score, types.EvalMetricKeyMeanRecall, types.EvalMetricKeyMeanPrecision:
	}

	return true
}

// preCheckAlgorithm 校验算法的一致性
func preCheckAlgorithm(request *types.SameCreateRequest) error {
	if request.CreateMode == types.CreateModeSimple {
		return nil
	}

	if request.SolutionType != "" && request.SolutionType != types.SolutionTypeCustom && request.SolutionType != types.SolutionTypePlatform {
		return fmt.Errorf("autolearn solution type is invalid")
	}

	// 1 数量一致性
	// 1.1 使用自定义算法模式，可能出现选择算法数量和前置配置不同的情况，已选择为准
	if request.SolutionType == types.SolutionTypeCustom {
		request.WorkerNum = int32(len(request.Algorithms))
	}
	if len(request.Algorithms) != int(request.WorkerNum) {
		return fmt.Errorf("the length of algorithms is not equal to the number of workerNum")
	}

	return nil
}

// checkAutoLearnNameValid 任务名正则、重名校验
func (ctr *Controller) CheckAutoLearnNameValid(d *dao.DAO, projectName, autoLearnName string) *errors.AutoLearnError {
	// 1 正则校验
	valid, err := regexp.MatchString(config.AutoLearnAndRevisionNameRegex, autoLearnName)
	if err != nil {
		log.WithError(err).WithField("name", autoLearnName).Error("regexp failed")
		return errors.ErrorInternal
	}
	if !valid {
		log.Errorf("name does not conform to regular rules: %s", autoLearnName)
		return errors.ErrorInvalidParams.SetMessage("the name parameter needs to conform the regular expression" +
			config.AutoLearnAndRevisionNameRegex)
	}

	// 2 重名校验
	count, err := dao.CountAutoLearn(d, projectName, autoLearnName)
	if err != nil {
		log.WithError(err).Error("failed to DAOCountAutoLearn")
		return errors.ErrorInternal
	}
	if count > 0 {
		log.WithField("autoLearnName", fmt.Sprintf("%s-%s-%s", d.TenantID, projectName, autoLearnName)).
			Error("autoLearn already exists")
		return errors.ErrorDuplicateName
	}

	return nil
}

// LoadAndStoreAutoLearn 构建autoLearn document
func (ctr *Controller) LoadAndStoreAutoLearn(
	gc *ginlib.GinContext,
	gin *ginlib.Gin,
	request *types.AutoLearnCreateRequest,
	datasets []*types.Dataset,
) (*types.AutoLearn, *types.AutoLearnRevision, error) {
	defaultRevision, err := ctr.newDefaultAutoLearnRevision(gc, gin, request, datasets)
	if err != nil {
		return nil, nil, err
	}

	if defaultRevision.Resource == nil {
		return nil, nil, fmt.Errorf("not found available solution config")
	}

	autoLearn := &types.AutoLearn{
		TenantID:           gc.GetAuthTenantID(),
		ProjectID:          gc.GetAuthProjectID(),
		AutoLearnID:        defaultRevision.AutoLearnID,
		AutoLearnName:      request.Name,
		Approach:           request.Approach,
		Type:               request.Type,
		Desc:               request.Desc,
		Tags:               request.Tags,
		Continued:          request.Continued,
		CreatedBy:          &types.User{UserID: gc.GetUserID(), UserName: gc.GetUserName()},
		UpdatedBy:          &types.User{UserID: gc.GetUserID(), UserName: gc.GetUserName()},
		CreatedAt:          time.Now().Unix(),
		UpdatedAt:          time.Now().Unix(),
		IsDeleted:          false,
		AutoLearnRevisions: []*types.AutoLearnRevision{defaultRevision},
	}

	if err := dao.InsertAAE(ctr.MongoClient, autoLearn); err != nil {
		log.WithError(err).Error("failed to DAOCreateAutoLearn")
		return nil, nil, err
	}

	ctr.StateMachine.SetLock(defaultRevision.RevisionID)

	return autoLearn, defaultRevision, nil
}

func (ctr *Controller) LoadAndStoreRevision(
	gc *ginlib.GinContext,
	gin *ginlib.Gin,
	request *types.RevisionCreateRequest,
	datasets []*types.Dataset,
	autoLearn *types.AutoLearn,
) (*types.AutoLearnRevision, error) {
	rv, err := ctr.newAutoLearnRevision(gc, gin, request, datasets, autoLearn)
	if err != nil {
		return nil, err
	}
	if err := dao.InsertRAE(ctr.MongoClient, rv, gc.GetAuthTenantID(), gc.GetAuthProjectID()); err != nil {
		log.WithError(err).Error("failed to create autoLearn revision")
		return nil, err
	}

	ctr.StateMachine.SetLock(rv.RevisionID)
	return rv, nil
}

// TransformAutoLearn 转换内部的信息，返回前端展示数据
func (ctr *Controller) TransformAutoLearn(autoLearn *types.AutoLearn) {
	stat := make(map[types.AutoLearnState]int32)
	for i := range autoLearn.AutoLearnRevisions {
		TransformRevision(autoLearn.AutoLearnRevisions[i])
		autoLearn.AutoLearnRevisions[i].OriginalAlgorithms = nil
		autoLearn.AutoLearnRevisions[i].Algorithms = nil
		autoLearn.AutoLearnRevisions[i].EvaluationDetails = nil
		stat[autoLearn.AutoLearnRevisions[i].AutoLearnState]++
	}

	for k := range stat {
		autoLearn.RevisionsStateStat = append(autoLearn.RevisionsStateStat, &types.RevisionsStateStat{
			State: k,
			Num:   stat[k],
		})
	}
}

// CreateAutoLearnMasterPod 创建一个autoLearn 任务
func (ctr *Controller) CreateAutoLearnMasterPod(autoLearn *types.AutoLearn, revision *types.AutoLearnRevision) error {
	logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, revision.RevisionID))
	basicInfo := &dao.BasicInfo{
		Project:     autoLearn.ProjectID,
		AutoLearnID: revision.AutoLearnID,
		RevisionID:  revision.RevisionID,
	}

	d := dao.NewDAO(ctr.MongoClient, autoLearn.TenantID)
	t := types.StateEventTypeMasterPodCreate
	if revision.CreateMode == types.CreateModeAdvance || revision.IsOptimized {
		t = types.StateEventTypeAdvancedMasterPodCreate
	}

	event := &types.StateEvent{T: t, A: types.EventActionDoing}
	ctr.StateMachine.Action(d, event, revision.RevisionID, types.StateEventCodeNothing)

	eventCode := types.StateEventCodeNothing
	defer func() {
		if eventCode != types.StateEventCodeNothing {
			utils.MetricEventDump(basicInfo.AutoLearnID, basicInfo.RevisionID, revision.Type, config.MetricAutoLearnFailedReason, map[string]any{
				utils.LogFieldFailedReason:  types.EventCodeMessage[eventCode],
				utils.LogFieldSnapXImageURI: revision.SnapXImageURI,
				utils.LogFieldManagedBy:     revision.ManagedBy,
				utils.LogFieldRvCreatedAt:   time.Unix(revision.CreatedAt, 0).Format(time.RFC3339),
			})
		} else {
			utils.MetricEventDump(autoLearn.AutoLearnID, revision.RevisionID, autoLearn.Type, config.MetricScheduleRequest, map[string]any{
				utils.LogFieldSnapXImageURI: revision.SnapXImageURI,
				utils.LogFieldManagedBy:     revision.ManagedBy,
			})
		}
	}()

	err := k8s.CreateAutoLearnMasterPod(ctr.SchedulerClient, ctr.KubeClient, autoLearn, revision)
	if err != nil {
		logs.WithError(err).Error("step6: create autoLearn master pod failed")
		eventCode = types.StateEventCodeCreateMasterPodFailed
		event.A = types.EventActionFailed
		ctr.StateMachine.Action(d, event, revision.RevisionID, eventCode)
		return err
	}

	logs.Info("step6: create autoLearn master pod succeeded")
	event.A = types.EventActionSuccess
	if err := ctr.StateMachine.Action(d, event, revision.RevisionID, eventCode); err != nil {
		return err
	}

	return nil
}

func (ctr *Controller) ActionToState(state types.ActionState, errCode int32) (types.AutoLearnState, *types.StateEvent, types.StateEventCode, bool) {
	nothing := types.StateEventCodeNothing
	switch state {
	case types.ActionStateLearning:
		return types.AutoLearnStateLearning, &types.StateEvent{
			T: types.StateEventTypeLearn,
			A: types.EventActionDoing,
		}, nothing, true
	case types.ActionStateCompleted:
		return types.AutoLearnStateCompleted, &types.StateEvent{
			T: types.StateEventTypeLearn,
			A: types.EventActionSuccess,
		}, nothing, true
	case types.ActionStateStopped:
		return types.AutoLearnStateStopped, &types.StateEvent{
			T: types.StateEventTypeStop,
			A: types.EventActionDoing,
		}, nothing, true
	case types.ActionStateFailed:
		code := ctr.TransformErrorCode(errCode)
		return types.AutoLearnStateLearnFailed, &types.StateEvent{
			T: types.StateEventTypeLearn,
			A: types.EventActionFailed,
		}, code, true
	case types.ActionMasterPodRunning:
		return types.AutoLearnStateMasterPodRunning, &types.StateEvent{
			T: types.StateEventTypeMasterPodRun,
			A: types.EventActionDoing,
		}, nothing, true
	case types.ActionAJobCreated:
		return types.AutoLearnStateWorkerAJobCreated, &types.StateEvent{
			T: types.StateEventTypeWorkerAJobCreate,
			A: types.EventActionDoing,
		}, nothing, true
	case types.ActionMasterPodCheckFailed:
		code := ctr.TransformErrorCode(errCode)
		return types.AutoLearnStateMasterPodRunFailed, &types.StateEvent{
			T: types.StateEventTypeMasterPodRun,
			A: types.EventActionStepinTimeout,
		}, code, true
	}

	return -1, nil, -1, false
}

// TransformErrorCode 对internalAgent上报的错误码进行转换
func (ctr *Controller) TransformErrorCode(errorCode int32) types.StateEventCode {
	switch errorCode {
	case 101:
		return types.StateEventCodeLearningFailed101
	case 201:
		return types.StateEventCodeLearningFailed201
	case 202:
		return types.StateEventCodeLearningFailed202
	case 301:
		return types.StateEventCodeLearningFailed301
	case 302:
		return types.StateEventCodeLearningFailed302
	case 303:
		return types.StateEventCodeLearningFailed303
	case 307:
		return types.StateEventCodeLearningFailed307
	case 401:
		return types.StateEventCodeLearningFailed401
	default:
		return types.StateEventCodeLearningFailed
	}
}

func (ctr *Controller) SetAlgorithmEnvs(tenantID, projectID string, algors []*types.Algorithm) error {
	for _, alg := range algors {
		if alg.SolutionType == types.SolutionTypeCustom {
			rv, err := outerClient.GetCodebaseRevision(tenantID, projectID, alg.ID)
			if err != nil || rv == nil {
				log.WithError(err).Error("get codebase revision failed")
				return err
			}
			soultion := rv.TaskClaims[aisConst.TaskTypeSolution]
			if soultion != nil && soultion.TaskSpec != nil {
				spec := soultion.TaskSpec["environment"]
				if spec != nil {
					if env, ok := spec.(map[string]any); ok {
						for _, alg := range algors {
							alg.Envs = env
						}
					}
				}
			}
		}
	}
	return nil
}

func (ctr *Controller) CheckCodebaseExist(tenantID, projectID string, revision *types.AutoLearnRevision) (bool, error) {
	var ids []string
	for _, alg := range revision.Algorithms {
		if alg.SolutionType == types.SolutionTypeCustom {
			ids = append(ids, alg.ID)
		}
	}

	for _, id := range ids {
		rv, err := outerClient.GetCodebaseRevision(tenantID, projectID, id)
		if err != nil || rv == nil {
			log.WithError(err).Error("get codebase revision failed")
			return false, err
		}
	}

	return true, nil
}

// DeleteAutoLearnMasterPod 删除一个autoLearn snapx master
func DeleteAutoLearnMasterPod(brainclient *schedulerv1alpha1.SchedulerV1alpha1Client, kubeclient *kubernetes.Clientset, aisClient *aisclient.Clientset, projectID, crdName string) error {
	logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", projectID, crdName))
	logs.Info("step-over: autoLearn task finished or stopped and to be deleted")

	namespace := features.GetProjectWorkloadNamespace(projectID)

	if features.IsSupportGangSchedule() {
		if err := brainclient.PodGroups(namespace).Delete(context.TODO(), crdName, metav1.DeleteOptions{}); err != nil {
			logs.WithError(err).Error("step-over-0: delete autoLearn task failed")
			return err
		}
	}

	if err := kubeclient.CoreV1().Pods(namespace).Delete(context.TODO(), crdName, metav1.DeleteOptions{}); err != nil {
		logs.WithError(err).Error("step-over-0: delete autoLearn master failed")
		return err
	}

	ajobs, err := aisClient.AjobV1alpha1().AJobs(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: types.AutoLearnNameKey + "=" + crdName,
	})
	logs.Infof("step-ajob: autoLearn task found %d ajobs", len(ajobs.Items))
	if err != nil {
		logs.WithError(err).Error("step-over-0: delete autoLearn task failed")
		return err
	}

	for i := range ajobs.Items {
		logs.Infof("step-ajob: autoLearn task try to delete ajob: %s", ajobs.Items[i].Name)
		if err := aisClient.AjobV1alpha1().AJobs(namespace).Delete(context.TODO(), ajobs.Items[i].Name, metav1.DeleteOptions{}); err != nil {
			logs.WithError(err).Error("step-over-0: delete autoLearn task failed")
			return err
		}
	}

	logs.Info("step-over-1: delete autoLearn task successfully")
	return nil
}

// TransformAutoLearnForOptions 转换内部的信息，返回前端进行列表选择
func TransformAutoLearnForOptions(ops []*types.AutoLearn) []*types.OptionsResp {
	res := make([]*types.OptionsResp, 0, len(ops))
	for i := range ops {
		op := types.OptionsResp{
			AutoLearnID:   ops[i].AutoLearnID,
			AutoLearnName: ops[i].AutoLearnName,
		}
		revisions := make([]*types.OptionsRevision, 0, len(ops[i].AutoLearnRevisions))

		for j := range ops[i].AutoLearnRevisions {
			if ops[i].AutoLearnRevisions[j].IsDeleted {
				continue
			}

			rv := ops[i].AutoLearnRevisions[j]

			rv.BestDetector = GetBestDetector(rv.Type, rv.Algorithms)

			fillEvaluationDetailAlgorithmName(rv.EvaluationDetails, rv.Algorithms)

			opRv := types.OptionsRevision{
				RevisionName: rv.RevisionName,
				RevisionID:   rv.RevisionID,
				Type:         rv.Type,
				SolutionType: rv.SolutionType,
				ModelFormat:  aisConst.ModelFormat(rv.ModelType),
				ModelType:    aisConst.ModelFormat(rv.ModelType),
			}
			iterations := make([]*types.OptionsIteration, 0, len(rv.EvaluationDetails))
			for k := range rv.EvaluationDetails {
				if !rv.EvaluationDetails[k].IsDeleted && rv.EvaluationDetails[k].ModelFilePath != "" {
					iterations = append(iterations, &types.OptionsIteration{
						AlgorithmName:   rv.EvaluationDetails[k].AlgorithmName,
						IterationNumber: rv.EvaluationDetails[k].IterationNumber,
						ModelFilePath:   rv.EvaluationDetails[k].ModelFilePath,
						BestDetector:    rv.BestDetector.BestIterationNumber == rv.EvaluationDetails[k].IterationNumber,
					})
				}
			}
			if len(iterations) != 0 {
				sort.Slice(iterations, func(i, j int) bool {
					return iterations[i].IterationNumber < iterations[j].IterationNumber
				})
				opRv.Iterations = iterations
				revisions = append(revisions, &opRv)
			}
		}

		if len(revisions) != 0 {
			op.Revisions = revisions
			res = append(res, &op)
		}
	}

	return res
}

func checkDetectedObjectType(objectType aisConst.DetectedObjectType) bool {
	return objectType == aisConst.DetectedObjectTypeNormal || objectType == aisConst.DetectedObjectTypeMultiMatch
}
