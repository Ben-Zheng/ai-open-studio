package watcher

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	autolearnUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	aisclient "go.megvii-inc.com/brain/brainpp/projects/aiservice/components/pkg/client/clientset/versioned"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	schedulerv1alpha1 "go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/client/clientset/versioned/typed/gang/v1alpha1"
)

// 提供如下功能
// 1. 依据 LingeringTime Table 杀死停留超时的训练任务
// 2. 初始化未完成任务的全局状态锁
// 3. 依据 master pod 状态清理完成的 POD+AJob (残留 > 1 day)
//    和 check 处于running 状态的 master pod 关联的任务是否已经终止
// 4. 清理 orphan worker pod

type CheckLoop struct {
	Lock         *LockMap
	StateMachine *StateMachine
	MigrateTable *LingeringTable

	MongoClient     *mgolib.MgoClient
	KubeClient      *kubernetes.Clientset
	AISClient       *aisclient.Clientset
	SchedulerClient *schedulerv1alpha1.SchedulerV1alpha1Client
}

func NewCheckLoop(
	lockMap *LockMap,
	sm *StateMachine,
	mgoClient *mgolib.MgoClient,
	kubeClient *kubernetes.Clientset,
	aisClient *aisclient.Clientset,
	schedulerClient *schedulerv1alpha1.SchedulerV1alpha1Client,
) *CheckLoop {
	mt := NewLingeringTable()
	return &CheckLoop{
		Lock:            lockMap,
		StateMachine:    sm,
		MigrateTable:    mt,
		MongoClient:     mgoClient,
		KubeClient:      kubeClient,
		AISClient:       aisClient,
		SchedulerClient: schedulerClient,
	}
}

func (ck *CheckLoop) Run() {
	go ck.watchTasks()
	go ck.watchJobs()
	go ck.watchWorkers()
}

func (ck *CheckLoop) watchTasks() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for ; true; <-ticker.C {
		tasks, evalutions, err := ck.listUnfinishedTask()
		if err != nil {
			log.WithError(err).Error()
		}
		for _, eval := range evalutions {
			if err := ck.evalHandle(eval); err != nil {
				log.WithError(err).Error("eavlHandle failed")
			}
		}

		for _, task := range tasks {
			log.Infof("checkLoop find revision: %s and to handle it", task.RevisionID)
			ck.StateMachine.Lock.SetLock(task.RevisionID)
			if err := ck.taskHandle(task); err != nil {
				log.WithError(err).Error("taskHandle failed")
			}
		}
	}
}

func (ck *CheckLoop) watchJobs() {
	ticker := time.NewTicker(time.Minute * 60)
	defer ticker.Stop()

	for ; true; <-ticker.C {
		pods, err := ck.KubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
			LabelSelector: types.AutolearnTypeKey + "=" + types.AutolearnTypeTaskValue,
		})
		if err != nil {
			log.WithError(err).Error("list all autolearn failed in periodicity clear up")
		}

		for i := range pods.Items {
			pod := pods.Items[i]
			if (time.Now().Unix()-pod.CreationTimestamp.Unix()) > 24*3600 &&
				(pods.Items[i].Status.Phase == coreV1.PodFailed || pods.Items[i].Status.Phase == coreV1.PodSucceeded) {
				ck.cleanJobs(pod.Name, pod.Namespace)
			}

			tenantID := pod.Labels[aisTypes.AISTenantID]
			projectID := pod.Labels[aisTypes.AISProject]
			autolearnIDs := strings.Split(pod.Name, "-")
			if len(autolearnIDs) != 3 {
				log.Warn("checkloop parse autolearn name failed")
				continue
			}
			if pods.Items[i].Status.Phase == coreV1.PodRunning {
				d := dao.NewDAO(ck.MongoClient, tenantID)
				revision, err := dao.GetAutoLearnRevisionBasicInfoByID(d, &dao.BasicInfo{Project: projectID, AutoLearnID: autolearnIDs[1], RevisionID: autolearnIDs[2]})
				if err != nil {
					log.WithError(err).Error("failed to GetAutoLearnRevisionBasicInfoByID")
					continue
				}
				if autolearnUtils.AutolearnStateInFinal(revision.AutoLearnState) {
					ck.cleanJobs(pod.Name, pod.Namespace)
				}
			}
		}
	}
}

func (ck *CheckLoop) watchWorkers() {
	ticker := time.NewTicker(time.Minute * 2 * 60)
	defer ticker.Stop()

	for ; true; <-ticker.C {
		ajobs, err := ck.AISClient.AjobV1alpha1().AJobs("").List(context.TODO(), metav1.ListOptions{
			LabelSelector: types.AutolearnTypeKey + "=" + types.AutolearnTypeWorkerValue,
		})
		if err != nil {
			log.WithError(err).Error("list all autolearn failed in periodicity clear up")
		}

		for i := range ajobs.Items {
			ajob := ajobs.Items[i]
			autolearnName := ajob.ObjectMeta.Labels[types.AutoLearnNameKey]
			if autolearnName == "" {
				continue
			}

			if _, err := ck.KubeClient.CoreV1().Pods(ajob.ObjectMeta.Namespace).Get(context.TODO(), autolearnName, metav1.GetOptions{}); err != nil {
				log.WithError(err).Errorf("get autolearn %s master pod failed", autolearnName)
				if errors.IsNotFound(err) {
					if err := ck.AISClient.AjobV1alpha1().AJobs(ajob.ObjectMeta.Namespace).Delete(context.TODO(), ajob.ObjectMeta.Name, metav1.DeleteOptions{}); err != nil {
						log.WithError(err).Errorf("delete autolearn %s ajobs runner %s failed", autolearnName, ajob.ObjectMeta.Namespace)
					}
				}
			}
		}
	}
}

func (ck *CheckLoop) listUnfinishedTask() ([]*types.AutoLearnRevision, []*types.AutoLearnEvaluation, error) {
	tenantNames, err := dao.GetAllTenantName(ck.MongoClient)
	if err != nil {
		log.WithError(err).Error("failed to get all tenant names")
	}

	var allRevisions []*types.AutoLearnRevision
	var allEvaluations []*types.AutoLearnEvaluation

	for i := range tenantNames {
		d := dao.NewDAO(ck.MongoClient, tenantNames[i])
		revisions, err := dao.GetRunningRevisions(d)
		if err != nil {
			log.WithError(err).Error("failed to GetAutoLearnWithoutFinalState")
		}

		if len(revisions) > 0 {
			allRevisions = append(allRevisions, revisions...)
		}

		evaluations, err := dao.GetEvaluationJobWithoutFinalState(d)
		if err != nil {
			log.WithError(err).Error("failed to get autoLearn eval job without final state")
		}

		if len(evaluations) > 0 {
			for _, evl := range evaluations {
				evl.TenantID = tenantNames[i]
			}
			allEvaluations = append(allEvaluations, evaluations...)
		}
	}
	return allRevisions, allEvaluations, nil
}

func (ck *CheckLoop) cleanTaskJobs(rv *types.AutoLearnRevision) error {
	namespace := features.GetProjectWorkloadNamespace(rv.ProjectID)
	name := utils.GetAutoLearnMasterPodName(rv.AutoLearnID, rv.RevisionID)
	if err := ck.cleanJobs(name, namespace); err != nil {
		log.WithError(err).Error("watcher try to clearn job failed")
		return err
	}

	return nil
}

func (ck *CheckLoop) cleanJobs(name, namespace string) error {
	ajobs, err := ck.AISClient.AjobV1alpha1().AJobs(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: types.AutoLearnNameKey + "=" + name,
	})
	if err != nil {
		log.WithError(err).Errorf("list autolearn %s related ajobs failed", name)
	}

	for i := range ajobs.Items {
		if err := ck.AISClient.AjobV1alpha1().AJobs(namespace).Delete(context.TODO(), ajobs.Items[i].Name, metav1.DeleteOptions{}); err != nil {
			log.WithError(err).Errorf("delete autolearn %s ajobs runner %s failed", name, ajobs.Items[i].Name)
		}
	}

	if features.IsSupportGangSchedule() {
		if err := ck.SchedulerClient.PodGroups(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{}); err != nil {
			log.WithError(err).Errorf("delete autolearn %s podgroup failed", name)
		}
	}

	if err := ck.KubeClient.CoreV1().Pods(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{}); err != nil {
		log.WithError(err).Errorf("delete autolearn %s pod failed", name)
	}
	return nil
}

func (ck *CheckLoop) taskHandle(task *types.AutoLearnRevision) error {
	logger := log.WithField("autoLearn", task.AutoLearnID+"/"+task.RevisionID)
	var startedAt int64
	for _, state := range task.AutoLearnStateStream {
		if state.AutoLearnState == task.AutoLearnState {
			startedAt = state.CreatedAt
		}
	}

	var reason string
	var hour float64 = 60 * 60
	d := dao.NewDAO(ck.MongoClient, task.TenantID)
	if ck.MigrateTable.HasBreakDown(task.AutoLearnState, startedAt) {
		event, eventCode := types.TimeoutFailedStateEvent(task.AutoLearnState, task.CreateMode == types.CreateModeAdvance)

		var msg []string
		if task.AutoLearnState == types.AutoLearnStateMasterPodCreateSucceeded ||
			task.AutoLearnState == types.AutoLearnStateMasterPodRunning {
			events, err := ck.getMsaterPodEvents(task)
			if err != nil {
				logger.WithError(err).Error("get master pod failed")
			}
			if events != nil {
				if len(events.Items) > 0 {
					eventCode = types.StateEventCodeCustom
				}
				if len(events.Items) > 1 {
					msg = append(msg, events.Items[len(events.Items)-2].Message, events.Items[len(events.Items)-1].Message)
				}
				if len(events.Items) == 1 {
					msg = append(msg, events.Items[len(events.Items)-1].Message)
				}
			}
		}

		if err := ck.StateMachine.Action(d, event, task.RevisionID, eventCode, msg...); err != nil {
			return err
		}

		if len(msg) > 0 {
			reason = strings.Join(msg, "")
		} else {
			reason = types.EventCodeMessage[eventCode]
		}
	}

	waitTime := time.Now().Unix() - startedAt
	if task.AutoLearnState == types.AutoLearnStateWorkerAJobCreated {
		if waitTime > int64(task.ScheduleTimeLimit*hour) {
			if err := ck.StateMachine.Action(d,
				&types.StateEvent{T: types.StateEventTypeLearn, A: types.EventActionStepinTimeout},
				task.RevisionID, types.StateEventCodeScheduleTimeout); err != nil {
				return err
			}
			reason = fmt.Sprintf(types.EventCodeMessage[types.StateEventCodeWatcherTimeout], task.AutoLearnState)
		}
	}

	if task.AutoLearnState == types.AutoLearnStateLearning {
		if waitTime > int64((task.TimeLimit+0.4)*hour) {
			if err := ck.StateMachine.Action(d,
				&types.StateEvent{T: types.StateEventTypeLearn, A: types.EventActionDoingTimeout},
				task.RevisionID, types.StateEventCodeLearningTimeout); err != nil {
				return err
			}
			if err := ck.cleanTaskJobs(task); err != nil {
				return err
			}
			reason = fmt.Sprintf(types.EventCodeMessage[types.StateEventCodeWatcherTimeout], task.AutoLearnState)
		}

		// 检查master pod是否存在
		_, err := ck.KubeClient.CoreV1().Pods(features.GetProjectWorkloadNamespace(task.ProjectID)).
			Get(context.TODO(), utils.GetAutoLearnMasterPodName(task.AutoLearnID, task.RevisionID), metav1.GetOptions{})
		if errors.IsNotFound(err) {
			logger.Error("autoLearn is running, but master pod is not exist, change state to failed")
			if err := ck.StateMachine.Action(d, &types.StateEvent{T: types.StateEventTypeLearn, A: types.EventActionFailed}, task.RevisionID, types.StateEventCodeLearningMasterPodKilled); err != nil {
				return err
			}
			if err := ck.cleanTaskJobs(task); err != nil {
				return err
			}
			reason = types.EventCodeMessage[types.StateEventCodeLearningMasterPodKilled]
		}
	}

	if reason != "" &&
		task.AutoLearnState >= types.AutoLearnStateMasterPodCreateSucceeded &&
		task.AutoLearnState != types.AutoLearnStateRecommending {
		if err := ck.cleanTaskJobs(task); err != nil {
			return err
		}
	}

	if reason != "" {
		utils.MetricEventDump(task.AutoLearnID, task.RevisionID, task.Type, config.MetricAutoLearnFailedReason, map[string]any{
			utils.LogFieldFailedReason: reason,
			utils.LogFieldRvCreatedAt:  time.Unix(task.CreatedAt, 0).Format(time.RFC3339),
		})
	}

	return nil
}

func (ck *CheckLoop) evalHandle(evaluation *types.AutoLearnEvaluation) error {
	logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", evaluation.AutoLearnID, evaluation.RevisionID))
	for i := range evaluation.EvaluationDetails {
		evalDetail := evaluation.EvaluationDetails[i]
		job, err := client.GetEvaluationJob(evaluation.TenantID, evaluation.ProjectID, evalDetail.EvaluationID, evalDetail.EvaluationJobID)
		if err != nil {
			logs.WithError(err).Error("failed to get evaluation job")
			return err
		}

		logs.Infof("CheckEvalJobState: current-%s/actual-%s", evalDetail.EvaluationJobState, job.State)
		if evalDetail.EvaluationJobState == job.State {
			continue
		}

		var jobs []*types.EvaluationJobDetail
		jobs = append(jobs, &types.EvaluationJobDetail{
			EvaluationJAobState: job.State,
			EvaluationRJobName:  job.RJobName,
			AppType:             job.AppType,
			EvaluationJobID:     job.ID.Hex(),
			Reason:              job.Reason,
		})
		if len(evalDetail.EvaluationJobs) > 1 {
			for i := 1; i < len(evalDetail.EvaluationJobs); i++ {
				jobDetail, err := client.GetEvaluationJob(evaluation.TenantID, evaluation.ProjectID, evalDetail.EvaluationID, evalDetail.EvaluationJobs[i].EvaluationJobID)
				if err != nil {
					logs.WithError(err).Error("failed to get evaluation job")
					return err
				}
				jobs = append(jobs, &types.EvaluationJobDetail{
					EvaluationJAobState: jobDetail.State,
					EvaluationRJobName:  jobDetail.RJobName,
					AppType:             jobDetail.AppType,
					EvaluationJobID:     jobDetail.ID.Hex(),
					Reason:              jobDetail.Reason,
				})
			}
		}

		if err := dao.UpdateEvaluationDetailStateAndReason(dao.NewDAO(ck.MongoClient, evaluation.TenantID),
			evaluation.RevisionID, job.Reason, job.State, evalDetail.IterationNumber, jobs...); err != nil {
			logs.WithError(err).Errorf("failed to update evaluation job state to %s", job.State)
			return err
		}
	}

	if config.Profile.AutoLearnDetectionF1Enabled {
		revision, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(ck.MongoClient, evaluation.TenantID), &dao.BasicInfo{
			Project:     evaluation.ProjectID,
			AutoLearnID: evaluation.AutoLearnID,
			RevisionID:  evaluation.RevisionID,
		})

		if revision.Type != aisTypes.AppTypeDetection {
			return nil
		}

		revision.BestDetector = GetBestDetector(revision.Type, revision.Algorithms)
		if err != nil {
			logs.WithError(err).Error("failed to GetAutoLearnRevisionByID")
			return err
		}
		if len(revision.EvaluationDetails) == 0 {
			return nil
		}
		bestIterationNumber := revision.BestDetector.BestIterationNumber

		var i int
		for i = range revision.EvaluationDetails {
			if revision.EvaluationDetails[i].IterationNumber == bestIterationNumber {
				break
			}
		}
		evaluationID := revision.EvaluationDetails[i].EvaluationID
		evaluationJobID := revision.EvaluationDetails[i].EvaluationJobID

		metaOptions, err := client.GetMetricsMetaOptions(evaluation.TenantID, evaluation.ProjectID, evaluationID, evaluationJobID)
		if err != nil {
			logs.WithError(err).Errorf("failed to GetMetricsMetaOptions")
			return err
		}
		chartData, err := client.GetMetricBasicAnalysisChart(evaluation.TenantID, evaluation.ProjectID, evaluationID, evaluationJobID,
			&client.MetricBasicAnalysisChart{
				IOU:       metaOptions.IOUs[0],
				Attribute: metaOptions.Attributes[0],
				Key:       "threshold",
			})
		if err != nil {
			logs.WithError(err).Errorf("failed to GetMetricBasicAnalysisChart")
			return err
		}

		var f1Values []map[string]float64
		for i := range chartData.Series {
			if chartData.Series[i].Name.Name != "f1" {
				continue
			}
			for j := range chartData.Series[i].DataValues {
				v, ok := chartData.Series[i].DataValues[j].(map[string]interface{})
				if !ok {
					continue
				}
				values, ok := v["values"].(map[string]interface{})
				if !ok {
					continue
				}
				parsedValues := make(map[string]float64)
				for key, val := range values {
					if floatVal, ok := val.(float64); ok {
						parsedValues[key] = floatVal
					}
				}

				f1Values = append(f1Values, parsedValues)
			}
		}
		if len(f1Values) == 0 {
			return nil
		}
		sort.Slice(f1Values, func(i, j int) bool {
			return f1Values[i]["f1"] > f1Values[j]["f1"]
		})
		updateMetric := types.EvalMetric{
			Key: "f1",
			IOU: metaOptions.IOUs[0],
			Condition: &types.Condition{
				Key:   "threshold",
				Value: f1Values[0]["threshold"],
			},
			MaxScore: f1Values[0]["f1"],
		}
		if err := dao.UpdateF1EvaluationMetric(dao.NewDAO(ck.MongoClient, evaluation.TenantID), evaluation.RevisionID, &updateMetric); err != nil {
			logs.WithError(err).Error("failed to UpdateF1EvaluationMetric")
			return err
		}
	}

	return nil
}

func (ck *CheckLoop) getMsaterPodEvents(task *types.AutoLearnRevision) (*coreV1.EventList, error) {
	namespace := features.GetProjectWorkloadNamespace(task.ProjectID)
	name := utils.GetAutoLearnMasterPodName(task.AutoLearnID, task.RevisionID)
	events, err := ck.KubeClient.CoreV1().Events(namespace).List(context.TODO(),
		metav1.ListOptions{FieldSelector: fmt.Sprintf("involvedObject.name=%s", name), TypeMeta: metav1.TypeMeta{Kind: "Pod"}})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// GetBestDetector 获取bestDetector曲线
func GetBestDetector(trainType aisTypes.AppType, algorithms []*types.Algorithm) *types.BestDetector {
	bestDetector := &types.BestDetector{Name: config.BestDetector}
	if trainType == aisTypes.AppTypeClassification {
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
