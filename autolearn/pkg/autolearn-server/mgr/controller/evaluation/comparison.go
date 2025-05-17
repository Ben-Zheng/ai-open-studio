package evaluation

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

// PreCheckCreateComparison 创建实验对比(本质是评测任务对比)前置检查
func PreCheckCreateComparison(req *types.EvalComparisonReq) error {
	if req.Type != aisConst.AppTypeDetection {
		return fmt.Errorf("only support classification type, invalid type: %s", req.Type)
	}

	// 数量限制
	if len(req.ComparisonRevisions) < config.MinEvalComparisonJobNum || len(req.ComparisonRevisions) > config.MaxEvalComparisonJobNum {
		return fmt.Errorf("the number of version should greater than 1 and less than 21")
	}

	// 版本ID简单校验
	revisionIDMap := make(map[string]bool)
	for i := range req.ComparisonRevisions {
		if req.ComparisonRevisions == nil {
			return fmt.Errorf("revisons is nil")
		}
		req.ComparisonRevisions[i].RevisionID = strings.TrimSpace(req.ComparisonRevisions[i].RevisionID)
		req.ComparisonRevisions[i].AutoLearnID = strings.TrimSpace(req.ComparisonRevisions[i].AutoLearnID)
		if req.ComparisonRevisions[i].RevisionID == "" || req.ComparisonRevisions[i].AutoLearnID == "" {
			return fmt.Errorf(" invalid autoLearnID or revisionID")
		}

		revisionIDMap[req.ComparisonRevisions[i].RevisionID] = true
	}
	if len(revisionIDMap) != len(req.ComparisonRevisions) {
		return fmt.Errorf("duplicated revisionID")
	}

	return nil
}

// SecondCheckCreateComparison 第二阶段检查，检查对比的版本是否存在
func SecondCheckCreateComparison(revisionIDs []string, rvIDToName map[string]string) error {
	for i := range revisionIDs {
		if _, ok := rvIDToName[revisionIDs[i]]; !ok {
			return errors.ErrorAutoLearnRvNotFound
		}
	}

	return nil
}

// GetAutoLearnIDsFromEvalComparisonReq 根据请求获取任务和实验ID
func GetAutoLearnIDsFromEvalComparisonReq(req *types.EvalComparisonReq) ([]string, []string) {
	autoLearnIDs := make([]string, 0, len(req.ComparisonRevisions))
	revisionIDs := make([]string, 0, len(req.ComparisonRevisions))
	for i := range req.ComparisonRevisions {
		autoLearnIDs = append(autoLearnIDs, req.ComparisonRevisions[i].AutoLearnID)
		revisionIDs = append(revisionIDs, req.ComparisonRevisions[i].RevisionID)
	}

	return autoLearnIDs, revisionIDs
}

// CreateEvalComparison 创建EvalComparison
func CreateEvalComparison(gc *ginlib.GinContext, req *types.EvalComparisonReq, comparisonID string, comparisonEvalJobs []*types.ComparisonEvalJob) *types.EvalComparison {
	cid, _ := primitive.ObjectIDFromHex(comparisonID)
	return &types.EvalComparison{
		ID:           primitive.NewObjectID(),
		ComparisonID: cid,
		ProjectID:    gc.GetAuthProjectID(),
		AppType:      req.Type,
		EvalJobs:     comparisonEvalJobs,
		IsDeleted:    false,
	}
}

func GetEvalJobIDsFromComparisonEvalJobs(comparisonEvalJobs []*types.ComparisonEvalJob) []string {
	res := make([]string, 0, len(comparisonEvalJobs))
	for i := range comparisonEvalJobs {
		res = append(res, comparisonEvalJobs[i].EvaluationJobID)
	}

	return res
}

func AssembleEvalComparisonSnapshotResp(comparisonID string, rvIDToAlgorithm map[string]*types.AutoLearnAlgorithm, evalComparison *types.EvalComparison) *types.EvalComparisonSnapshotResp {
	resp := types.EvalComparisonSnapshotResp{
		EvalComparisonID:   evalComparison.ID.Hex(),
		Type:               evalComparison.AppType,
		ComparisonID:       comparisonID,
		EvalJobs:           evalComparison.EvalJobs,
		JobIDToJobName:     make(map[string]string),
		JobIDToRvName:      make(map[string]string),
		RvNameToAlgorithms: make(map[string]*types.Algorithm),
	}
	for _, job := range evalComparison.EvalJobs {
		resp.JobIDToJobName[job.EvaluationJobID] = job.EvaluationRJobName
		comparisonRvName := job.AutoLearnName + "/" + job.RevisionName + "/" + strconv.Itoa(int(job.IterationNumber))
		resp.JobIDToRvName[job.EvaluationJobID] = comparisonRvName
		resp.RvNameToAlgorithms[comparisonRvName] = getAlgorithmByIterationNum(rvIDToAlgorithm[job.RevisionID].ActualAlgorithms, job.IterationNumber)
	}

	return &resp
}

func AssembleEvalComparisonSnapshotErrorResp(evalComparison *types.EvalComparison, errorData *evalTypes.ComparisonErrorData) *types.EvalComparisonSnapshotErrorResp {
	resp := &types.EvalComparisonSnapshotErrorResp{
		EvalComparisonID: evalComparison.ID.Hex(),
		Type:             evalComparison.AppType,
		Jobs:             make([]*types.EvalComparisonErrorJob, 0),
	}

	jobIDToEvalJob := make(map[string]*types.ComparisonEvalJob)
	for _, j := range evalComparison.EvalJobs {
		jobIDToEvalJob[j.EvaluationJobID] = j
	}

	for i := range errorData.Jobs {
		if ej, ok := jobIDToEvalJob[errorData.Jobs[i].JobID]; ok {
			resp.Jobs = append(resp.Jobs, &types.EvalComparisonErrorJob{
				AutoLearnID:        ej.AutoLearnID,
				AutoLearnName:      ej.AutoLearnName,
				RevisionID:         ej.RevisionID,
				RevisionName:       ej.RevisionName,
				IterationNumber:    ej.IterationNumber,
				EvaluationID:       ej.EvaluationID,
				EvaluationJobID:    ej.EvaluationJobID,
				EvaluationRJobName: ej.EvaluationRJobName,
			})
		}
	}
	return resp
}

func getAlgorithmByIterationNum(algorithms []*types.Algorithm, iterationNumber int32) *types.Algorithm {
	for i := range algorithms {
		if algorithms[i] == nil {
			log.Warn("algorithm is nil")
			continue
		}
		if algorithms[i].Score == nil {
			log.Warn("algorithm score is nil")
			continue
		}
		for j := range algorithms[i].Score {
			if algorithms[i].Score[j].IterationNumber == iterationNumber {
				algorithms[i].OriginalInfo = ""
				return algorithms[i]
			}
		}
	}

	return nil
}

// CreateEvalComparisonSnapshot 创建CreateEvalComparisonSnapshot实例
func CreateEvalComparisonSnapshot(gc *ginlib.GinContext, appType aisConst.AppType, req *types.EvalComparisonSnapshotReq) (*types.EvalComparisonSnapshot, error) {
	evalComparisonID, _ := primitive.ObjectIDFromHex(req.EvalComparisonID)
	comparisonID, comparisonSnapshotID, err := client.CreateComparisonSnapshot(gc.GetAuthTenantID(), gc.GetAuthProjectID(),
		&types.User{UserName: gc.GetUserName(), UserID: gc.GetUserID()}, req.SnapshotInfo)
	if err != nil {
		log.WithError(err).Error("failed to CreateComparisonSnapshot")
		return nil, err
	}

	return &types.EvalComparisonSnapshot{
		ID:                   primitive.NewObjectID(),
		AppType:              appType,
		ProjectID:            gc.GetAuthProjectID(),
		EvalComparisonID:     evalComparisonID,
		ComparisonID:         comparisonID,
		ComparisonSnapshotID: comparisonSnapshotID,
		CreatedBy:            &types.User{UserName: gc.GetUserName(), UserID: gc.GetUserID()},
		CreatedAt:            time.Now().Unix(),
		UpdatedAt:            time.Now().Unix(),
	}, nil
}

// ExportEvalComparison 导出实验对比数据
func ExportEvalComparison(gc *ginlib.GinContext, comparisonID string, req *types.ExportComparisonSnapshotReq) (*evalTypes.ComparisonWithSnapshot, error) {
	comparisonWithSnapshot, err := client.ExportComparison(gc.GetAuthTenantID(), gc.GetAuthProjectID(),
		&types.User{UserName: gc.GetUserName(), UserID: gc.GetUserID()}, comparisonID, *req.EvalRequest)
	if err != nil {
		log.WithError(err).Error("failed to ExportEvalComparison")
		return nil, err
	}
	return comparisonWithSnapshot, nil
}

// GetEvalComparison 获取评测对比数据
func GetEvalComparison(gc *ginlib.GinContext, comparisonID string) (*evalTypes.ComparisonWithSnapshot, *evalTypes.ComparisonErrorData, error) {
	comparisonWithSnapshot, errorData, err := client.GetComparison(gc.GetAuthTenantID(), gc.GetAuthProjectID(),
		&types.User{UserName: gc.GetUserName(), UserID: gc.GetUserID()}, comparisonID)
	if err != nil {
		log.WithError(err).Error("failed to ExportEvalComparison")
		return nil, errorData, err
	}
	return comparisonWithSnapshot, nil, nil
}
