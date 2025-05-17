package evaluation

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
)

// PreCheckEvalRetry 重试前检查
func PreCheckEvalRetry(rv *types.AutoLearnRevision, iterationNumber uint64) (*types.EvaluationDetail, error) {
	var evaluationDetail types.EvaluationDetail
	for i := range rv.EvaluationDetails {
		if rv.EvaluationDetails[i].IterationNumber == int32(iterationNumber) && !rv.EvaluationDetails[i].IsDeleted {
			evaluationDetail = *(rv.EvaluationDetails[i])
			break
		}
	}
	if evaluationDetail.IterationNumber == 0 {
		return nil, errors.ErrorEvalJobNotFound
	}

	// 校验评测子任务状态
	if !evaluationDetail.EvaluationJobState.IsFailed() {
		log.Error("evalJob is not failed")
		return nil, errors.ErrorInvalidParams
	}

	if evaluationDetail.SdsFilePath == "" {
		return nil, errors.ErrorInvalidParams
	}

	// 获取评测ID、name
	for i := range rv.EvaluationDetails {
		if rv.EvaluationDetails[i].EvaluationID != "" {
			evaluationDetail.EvaluationID = rv.EvaluationDetails[i].EvaluationID
			evaluationDetail.EvaluationName = rv.EvaluationDetails[i].EvaluationName
			break
		}
	}

	return &evaluationDetail, nil
}

// UpdateAutoLearnEvalJobStatePeriodically 周期性查询并更新评测job状态
func UpdateAutoLearnEvalJobStatePeriodically(c *mgolib.MgoClient) {
	ticker := time.NewTicker(config.EvalJobStateCheckInterval * time.Second)
	defer ticker.Stop()

	log.Info("Update evalJob periodically")

	for range ticker.C {
		// 1 获取租户
		tenantNames, err := dao.GetAllTenantName(c)
		if err != nil {
			log.WithError(err).Error("failed to get all tenant names")
			continue
		}
		updateEvalJobStateInTenants(c, tenantNames)
	}
}

func updateEvalJobStateInTenants(c *mgolib.MgoClient, tenantName []string) {
	for i := range tenantName {
		evaluations, err := dao.GetEvaluationJobWithoutFinalState(dao.NewDAO(c, tenantName[i]))
		if err != nil {
			log.WithError(err).Error("failed to get autoLearn eval job without final state")
			return
		}
		for j := range evaluations {
			if err := queryAndUpdateEvalJobState(c, tenantName[i], evaluations[j]); err != nil {
				log.WithError(err).Error("failed to queryAndUpdateEvalJobState")
			}
		}
	}
}

func queryAndUpdateEvalJobState(mgoClient *mgolib.MgoClient, tenantID string, evaluation *types.AutoLearnEvaluation) error {
	for i := range evaluation.EvaluationDetails {
		evalDetail := evaluation.EvaluationDetails[i]
		logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", evaluation.AutoLearnID, evaluation.RevisionID))

		job, err := client.GetEvaluationJob(tenantID, evaluation.ProjectID, evalDetail.EvaluationID, evalDetail.EvaluationJobID)
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
				jobDetail, err := client.GetEvaluationJob(tenantID, evaluation.ProjectID, evalDetail.EvaluationID, evalDetail.EvaluationJobs[i].EvaluationJobID)
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

		if err := dao.UpdateEvaluationDetailStateAndReason(dao.NewDAO(mgoClient, tenantID),
			evaluation.RevisionID, job.Reason, job.State, evalDetail.IterationNumber, jobs...); err != nil {
			logs.WithError(err).Errorf("failed to update evaluation job state to %s", job.State)
			return err
		}
	}

	return nil
}
