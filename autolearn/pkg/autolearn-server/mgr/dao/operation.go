package dao

import (
	"context"
	"sort"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	authClientv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
)

// UpdateAlgorithm 简单模式下更新版本状态和算法
func UpdateAlgorithm(client *mgolib.MgoClient, tenantID string, basicInfo *BasicInfo, algorithm *types.AutoLearnAlgorithm) error {
	d := NewDAO(client, tenantID)
	if err := insertAlgorithm(d, algorithm); err != nil {
		log.Error("caught exception during transaction in insertAlgorithm, aborting...")
		return err
	}
	return nil
}

// InsertAAE 添加训练任务AutoLearn，及其算法Algorithm和评测Evaluation
func InsertAAE(client *mgolib.MgoClient, autoLearn *types.AutoLearn) error {
	c, err := client.GetMongoClient(getMockGinContext(autoLearn.TenantID))
	if err != nil {
		log.Error("failed to get mongo client")
		return err
	}
	d := NewDAO(client, autoLearn.TenantID)

	return c.UseSession(context.TODO(), func(sctx mongo.SessionContext) error {
		if err := sctx.StartTransaction(options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.New(writeconcern.WMajority()))); err != nil {
			return err
		}

		d.Context = sctx
		if err := insertAutoLearn(d, autoLearn); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in UpdateALRevisionState")
			}
			log.Error("caught exception during transaction in UpdateALRevisionState, aborting...")
			return err
		}

		if err := insertAlgorithmAndEvaluation(sctx, d, autoLearn.AutoLearnRevisions[0], autoLearn.ProjectID); err != nil {
			log.Error("failed to insertAlgorithmAndEvaluation")
			return err
		}

		err = sctx.CommitTransaction(sctx)
		switch e := err.(type) {
		case nil:
			return nil
		case mongo.CommandError:
			if e.HasErrorLabel("UnknownTransactionCommitResult") {
				log.Error("UnknownTransactionCommitResult")
			}
			return e
		default:
			return e
		}
	})
}

// InsertRAE 添加训练任务版本AutoLearnRevision，及其算法Algorithm和评测Evaluation
func InsertRAE(client *mgolib.MgoClient, revision *types.AutoLearnRevision, tenantID, projectID string) error {
	c, err := client.GetMongoClient(getMockGinContext(tenantID))
	if err != nil {
		log.Error("failed to get mongo client")
		return err
	}
	d := NewDAO(client, tenantID)

	return c.UseSession(context.TODO(), func(sctx mongo.SessionContext) error {
		if err := sctx.StartTransaction(options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.New(writeconcern.WMajority()))); err != nil {
			return err
		}

		d.Context = sctx
		if err := insertAutoLearnRevision(d, revision, projectID); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in UpdateALRevisionState")
			}
			log.Error("caught exception during transaction in UpdateALRevisionState, aborting...")
			return err
		}

		if err := insertAlgorithmAndEvaluation(sctx, d, revision, projectID); err != nil {
			log.Error("failed to insertAlgorithmAndEvaluation")
			return err
		}

		err = sctx.CommitTransaction(sctx)
		switch e := err.(type) {
		case nil:
			return nil
		case mongo.CommandError:
			if e.HasErrorLabel("UnknownTransactionCommitResult") {
				log.Error("UnknownTransactionCommitResult")
			}
			return e
		default:
			return e
		}
	})
}

func insertAlgorithmAndEvaluation(sctx mongo.SessionContext, d *DAO, rv *types.AutoLearnRevision, project string) error {
	if rv.IsOptimized || rv.CreateMode == types.CreateModeAdvance || rv.SolutionType == types.SolutionTypeCustom {
		if err := insertAlgorithm(d, &types.AutoLearnAlgorithm{
			AutoLearnID:        rv.AutoLearnID,
			RevisionID:         rv.RevisionID,
			ActualAlgorithms:   rv.Algorithms,
			OriginalAlgorithms: rv.OriginalAlgorithms,
		}); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in insertAlgorithm")
			}
			log.Error("caught exception during transaction in insertAlgorithm, aborting...")
			return err
		}
	}

	if err := insertEvaluation(d, &types.AutoLearnEvaluation{
		ProjectID:         project,
		AutoLearnID:       rv.AutoLearnID,
		RevisionID:        rv.RevisionID,
		EvalMetric:        rv.EvalMetric,
		ClfEvalMetric:     rv.ClfEvalMetric,
		F1EvalMetric:      rv.F1EvalMetric,
		EvaluationDetails: rv.EvaluationDetails,
	}); err != nil {
		if e := sctx.AbortTransaction(sctx); e != nil {
			log.WithError(e).Error("AbortTransaction failed in insertEvaluation")
		}
		log.Error("caught exception during transaction in insertEvaluation, aborting...")
		return err
	}

	return nil
}

func GetAutoLearnByID(d *DAO, projectID, autoLearnID string) (*types.AutoLearn, error) {
	autoLearn, err := findAutoLearnByID(d, autoLearnID, projectID)
	if err != nil {
		log.Error("failed to findAutoLearnByID")
		return nil, err
	}

	rvIDToAlgorithm, err := findAlgorithmByAutoLearnID(d, autoLearnID)
	if err != nil {
		log.Error("failed to findAlgorithmByAutoLearnID")
		return nil, err
	}
	rvIDToEvaluation, err := findEvaluationByAutoLearnID(d, autoLearnID)
	if err != nil {
		log.Error("failed to findEvaluationByAutoLearnID")
		return nil, err
	}

	autoLearn.AutoLearnRevisions = filterAndFillAutoLearnRevision(autoLearn.AutoLearnRevisions, rvIDToAlgorithm, rvIDToEvaluation)

	return autoLearn, nil
}

func GetAutoLearnByIDWithOriginalInfo(d *DAO, projectID, autoLearnID string) (*types.AutoLearn, error) {
	autoLearn, err := findAutoLearnByID(d, autoLearnID, projectID)
	if err != nil {
		log.Error("failed to findAutoLearnByID")
		return nil, err
	}

	rvIDToAlgorithm, err := findAlgorithmByAutoLearnIDWithOrigianlInfo(d, autoLearnID)
	if err != nil {
		log.Error("failed to findAlgorithmByAutoLearnID")
		return nil, err
	}
	rvIDToEvaluation, err := findEvaluationByAutoLearnID(d, autoLearnID)
	if err != nil {
		log.Error("failed to findEvaluationByAutoLearnID")
		return nil, err
	}

	autoLearn.AutoLearnRevisions = filterAndFillAutoLearnRevision(autoLearn.AutoLearnRevisions, rvIDToAlgorithm, rvIDToEvaluation)

	return autoLearn, nil
}

func GetAutoLearnBasicInfoByID(d *DAO, projectID, autoLearnID string) (*types.AutoLearn, error) {
	return findAutoLearnByID(d, autoLearnID, projectID)
}

func GetAutoLearnRevisionByID(d *DAO, info *BasicInfo) (*types.AutoLearnRevision, error) {
	rv, err := findAutoLearnRevisionByID(d, info)
	if err != nil {
		log.Error("failed to findAutoLearnRevisionByID")
		return nil, err
	}

	rvIDToAlgorithm, err := findAlgorithmByRevisionID(d, info.RevisionID)
	if err != nil {
		log.Error("failed to findAlgorithmByAutoLearnID")
		return nil, err
	}
	rvIDToEvaluation, err := findEvaluationByRevisionID(d, info.RevisionID)
	if err != nil {
		log.Error("failed to findEvaluationByAutoLearnID")
		return nil, err
	}

	return filterAndFillAutoLearnRevision([]*types.AutoLearnRevision{rv}, rvIDToAlgorithm, rvIDToEvaluation)[0], nil
}

// GetAutoLearnRevisionBasicInfoByID 只获取版本基本信息
func GetAutoLearnRevisionBasicInfoByID(d *DAO, info *BasicInfo) (*types.AutoLearnRevision, error) {
	return findAutoLearnRevisionByID(d, info)
}

func LastAutoLearn(gc *ginlib.GinContext, d *DAO) (*types.AutoLearnLastRevision, error) {
	autoLearnRv, err := findLastAutoLearn(gc, d)
	if err != nil || autoLearnRv.AutoLearnRevision == nil {
		log.Error("failed to FindAutoLearn")
		return nil, err
	}

	rvIDToAlgorithm, err := findAlgorithmByAutoLearnIDs(d, []string{autoLearnRv.AutoLearnID}, false)
	if err != nil {
		log.Error("failed to findAlgorithmByAutoLearnID")
		return nil, err
	}

	rvIDToEvaluation, err := findEvaluationByAutoLearnIDs(d, []string{autoLearnRv.AutoLearnID})
	if err != nil {
		log.Error("failed to findEvaluationByAutoLearnID")
		return nil, err
	}

	authclient := authClientv1.NewClientWithAuthorazation(config.Profile.AisEndPoint.AuthServer, config.Profile.MultiSiteConfig.CenterAK, config.Profile.MultiSiteConfig.CenterSK)
	project, err := authclient.GetProject(gc, autoLearnRv.TenantID, autoLearnRv.ProjectID)
	if err != nil {
		log.WithError(err).Error("get tenant failed")
		return nil, err
	}
	autoLearnRv.ProjectUID = project.ID
	autoLearnRv.ProjectName = project.Name

	filterAndFillAutoLearnRevision([]*types.AutoLearnRevision{autoLearnRv.AutoLearnRevision}, rvIDToAlgorithm, rvIDToEvaluation)
	return autoLearnRv, nil
}

// ListListAutoLearns 根据关键字等参数获取任务列表
func ListListAutoLearns(gc *ginlib.GinContext, d *DAO) (int64, []*types.AutoLearn, error) {
	total, autoLearns, err := findAutoLearn(gc, d)
	if err != nil {
		log.Error("failed to FindAutoLearn")
		return 0, nil, err
	}
	if total == 0 {
		return total, autoLearns, err
	}

	autoLearnIDs := make([]string, 0, len(autoLearns))
	for i := range autoLearns {
		autoLearnIDs = append(autoLearnIDs, autoLearns[i].AutoLearnID)
	}

	rvIDToEvaluation, err := findEvaluationByAutoLearnIDs(d, autoLearnIDs)
	if err != nil {
		log.Error("failed to findEvaluationByAutoLearnID")
		return 0, nil, err
	}

	for i := range autoLearns {
		autoLearns[i].AutoLearnRevisions = filterAndFillAutoLearnRevision(autoLearns[i].AutoLearnRevisions, nil, rvIDToEvaluation)
	}

	return total, autoLearns, nil
}

func ListRevisions(gc *ginlib.GinContext, d *DAO) (int64, []*types.AutoLearnRevision, error) {
	total, revisions, err := findRevisions(gc, d)
	if err != nil {
		log.Error("failed to revisions")
		return 0, nil, err
	}
	return total, revisions, err
}

// ListAutoLearnRevisions 根据关键字等参数获取任务版本列表
func ListAutoLearnRevisions(gc *ginlib.GinContext, d *DAO, autoLearnID string) (int64, bool, []*types.AutoLearnRevision, error) {
	total, existInvisible, revisions, err := findAutoLearnRevision(gc, d, autoLearnID)
	if err != nil {
		log.Error("failed to FindAutoLearnRevision")
		return 0, existInvisible, nil, err
	}
	if total == 0 {
		return total, existInvisible, revisions, err
	}

	revisionIDs := make([]string, 0, len(revisions))
	for i := range revisions {
		revisionIDs = append(revisionIDs, revisions[i].RevisionID)
	}

	rvIDToAlgorithm, err := FindAlgorithmByRevisionIDs(d, revisionIDs, false)
	if err != nil {
		log.Error("failed to findAlgorithmByRevisionIDs")
		return 0, existInvisible, nil, err
	}

	rvIDToEvaluation, err := findEvaluationByRevisionIDs(d, revisionIDs)
	if err != nil {
		log.Error("failed to findEvaluationByRevisionIDs")
		return 0, existInvisible, nil, err
	}

	return total, existInvisible, filterAndFillAutoLearnRevision(revisions, rvIDToAlgorithm, rvIDToEvaluation), nil
}

// RetryEvaluationJob 评测任务job重试
func RetryEvaluationJob(client *mgolib.MgoClient, tenantID, revisionID string, evaluationDetail *types.EvaluationDetail) error {
	c, err := client.GetMongoClient(getMockGinContext(tenantID))
	if err != nil {
		log.Error("failed to get mongo client")
		return err
	}
	d := NewDAO(client, tenantID)

	return c.UseSession(context.TODO(), func(sctx mongo.SessionContext) error {
		if err := sctx.StartTransaction(options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.New(writeconcern.WMajority()))); err != nil {
			return err
		}

		d.Context = sctx
		if err := DeleteEvaluationDetail(d, revisionID, evaluationDetail.IterationNumber); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in DeleteEvaluationDetail")
			}
			log.Error("caught exception during transaction in DeleteEvaluationDetail, aborting...")
			return err
		}

		if err := AddEvaluationDetail(d, revisionID, evaluationDetail); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in AddEvaluationDetail")
			}
			log.Error("caught exception during transaction in AddEvaluationDetail, aborting...")
			return err
		}

		err = sctx.CommitTransaction(sctx)
		switch e := err.(type) {
		case nil:
			return nil
		case mongo.CommandError:
			if e.HasErrorLabel("UnknownTransactionCommitResult") {
				log.Error("UnknownTransactionCommitResult")
			}
			return e
		default:
			return e
		}
	})
}

func ListAutoLearnsForOptions(gc *ginlib.GinContext, d *DAO) ([]*types.AutoLearn, error) {
	autolearnName, _ := gc.GetRequestParam("searchKey")
	appType, _ := gc.GetRequestParam("appType")
	projectID := gc.GetAuthProjectID()

	autoLearns, err := findAutoLearnForOption(d, projectID, appType, autolearnName)
	if err != nil {
		log.Error("failed to findAutoLearnAllByProjectID")
		return nil, err
	}

	var autoLearnIDs []string
	for i := range autoLearns {
		autoLearnIDs = append(autoLearnIDs, autoLearns[i].AutoLearnID)
	}

	if len(autoLearnIDs) == 0 {
		return autoLearns, nil
	}

	rvToEvaluation, err := findEvaluationByAutoLearnIDs(d, autoLearnIDs)
	if err != nil {
		log.Error("failed to findEvaluationByAutoLearnIDs")
	}

	rvIDToAlgorithm, err := findAlgorithmByAutoLearnIDs(d, autoLearnIDs, false)
	if err != nil {
		log.Error("failed to findAlgorithmByAutoLearnIDs")
		return nil, err
	}

	for i := range autoLearns {
		var autoLearnRevisions []*types.AutoLearnRevision
		for _, revision := range autoLearns[i].AutoLearnRevisions {
			if revision.AutoLearnState == types.AutoLearnStateCompleted {
				autoLearnRevisions = append(autoLearnRevisions, revision)
			}
		}
		autoLearns[i].AutoLearnRevisions = filterAndFillAutoLearnRevision(autoLearnRevisions, rvIDToAlgorithm, rvToEvaluation)
	}

	return autoLearns, nil
}

func filterAndFillAutoLearnRevision(rvs []*types.AutoLearnRevision, rvIDToAlgorithm map[string]*types.AutoLearnAlgorithm, rvIDToEvaluation map[string]*types.AutoLearnEvaluation) []*types.AutoLearnRevision {
	revisions := make([]*types.AutoLearnRevision, 0, len(rvs))

	for _, rv := range rvs {
		if rv.IsDeleted {
			continue
		}

		// 对于ListListAutoLearns发起的调用, algorithm信息没用,所以可能存在rvIDToAlgorithm为nil的情况
		if rvIDToAlgorithm != nil && rvIDToAlgorithm[rv.RevisionID] != nil {
			var algorithms []*types.Algorithm
			for _, item := range rvIDToAlgorithm[rv.RevisionID].ActualAlgorithms {
				if item != nil {
					algorithms = append(algorithms, item)
				}
			}
			rv.Algorithms = algorithms
			rv.OriginalAlgorithms = rvIDToAlgorithm[rv.RevisionID].OriginalAlgorithms
		}
		if rvIDToEvaluation[rv.RevisionID] != nil {
			rv.EvalMetric = rvIDToEvaluation[rv.RevisionID].EvalMetric
			rv.ClfEvalMetric = rvIDToEvaluation[rv.RevisionID].ClfEvalMetric
			rv.F1EvalMetric = rvIDToEvaluation[rv.RevisionID].F1EvalMetric
			rv.EvaluationDetails = rvIDToEvaluation[rv.RevisionID].EvaluationDetails
		}
		revisions = append(revisions, rv)
	}

	return revisions
}

// UpdateALRevisionStateAndTrainData 更新优化实验的状态和train SDS 信息
func UpdateALRevisionStateAndTrainData(client *mgolib.MgoClient, tenantID string, basicInfo *BasicInfo, state types.AutoLearnState, dataset *types.Dataset) error {
	c, err := client.GetMongoClient(getMockGinContext(tenantID))
	if err != nil {
		log.Error("failed to get mongo client")
		return err
	}
	d := NewDAO(client, tenantID)

	return c.UseSession(context.TODO(), func(sctx mongo.SessionContext) error {
		if err := sctx.StartTransaction(options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.New(writeconcern.WMajority()))); err != nil {
			return err
		}

		d.Context = sctx
		if err := UpdateALRevisionState(d, basicInfo, state, true); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in UpdateALRevisionState")
			}
			log.Error("caught exception during transaction in UpdateALRevisionState, aborting...")
			return err
		}
		if err := AddALRevisionDataset(d, basicInfo, dataset, true); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in AddALRevisionDataset")
			}
			log.Error("caught exception during transaction in AddALRevisionDataset, aborting...")
			return err
		}

		err = sctx.CommitTransaction(sctx)
		switch e := err.(type) {
		case nil:
			return nil
		case mongo.CommandError:
			if e.HasErrorLabel("UnknownTransactionCommitResult") {
				log.Error("UnknownTransactionCommitResult")
			}
			return e
		default:
			return e
		}
	})
}

// GetComparisonEvalJobsByRevisionIDs 根据 revisionID 获取实验对比所需的评测任务Job信息
func GetComparisonEvalJobsByRevisionIDs(d *DAO, revisionIDs []string, autoLearnIDToName, revisionIDToName map[string]string) ([]*types.ComparisonEvalJob, error) {
	rvIDToEvaluation, err := findEvaluationByRevisionIDs(d, revisionIDs)
	if err != nil {
		log.WithError(err).Error("failed to findEvaluationByRevisionIDs")
		return nil, err
	}

	var res []*types.ComparisonEvalJob
	for rvID, v := range rvIDToEvaluation {
		if v == nil {
			log.Warnf("the revision of evaluation is nil %s", rvID)
			continue
		}

		// 1 保留非删除且成功状态的评测job
		validDetails := make([]*types.EvaluationDetail, 0, len(v.EvaluationDetails))
		for j := range v.EvaluationDetails {
			if v.EvaluationDetails[j] == nil {
				log.Warnf("the revision of evaluationDetails is nil: %s", rvID)
				continue
			}
			if !v.EvaluationDetails[j].IsDeleted && v.EvaluationDetails[j].EvaluationJobState == evalTypes.EvalJobResultStateSuccess {
				validDetails = append(validDetails, v.EvaluationDetails[j])
			}
		}

		// 2 根据迭代号排序
		if len(validDetails) == 0 {
			log.Warnf("the revision of valid evaluationDetails is zero: %s", rvID)
			continue
		}
		sort.Slice(validDetails, func(i, j int) bool { return validDetails[i].IterationNumber < validDetails[j].IterationNumber })

		// 组装结果
		l := len(validDetails)
		res = append(res, &types.ComparisonEvalJob{
			AutoLearnID:        v.AutoLearnID,
			AutoLearnName:      autoLearnIDToName[v.AutoLearnID],
			RevisionID:         v.RevisionID,
			RevisionName:       revisionIDToName[v.RevisionID],
			IterationNumber:    validDetails[l-1].IterationNumber,
			EvaluationID:       validDetails[l-1].EvaluationID,
			EvaluationJobID:    validDetails[l-1].EvaluationJobID,
			EvaluationRJobName: validDetails[l-1].EvaluationRJobName,
		})
	}

	for i := range res {
		log.Debugf("[CXY] comparisonEvalJob is %+v", res[i])
	}

	return res, nil
}
