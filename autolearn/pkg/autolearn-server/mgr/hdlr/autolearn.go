package hdlr

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	autoLearnMgr "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/autolearn"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	apiServerUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	datasetCli "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/controller/dataset"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

func (m *Mgr) CreateAutoLearn(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var autoLearnReq types.AutoLearnCreateRequest
	if err := gc.GetRequestJSONBody(&autoLearnReq); err != nil {
		log.WithError(err).Error("failed to parse request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	// 1 前置综合校验，以及填充数据集信息
	var datasets []*types.Dataset
	if err := m.Controller.CreatePreCheckAndGetDatasetInfo(
		nil,
		gc,
		autoLearnReq.SameCreateRequest,
		&datasets,
		m.App.Conf.AuthConf.Debug,
	); err != nil {
		log.WithError(err).Error("failed to createPreCheckAndGetDatasetInfo")
		ctx.Error(c, err)
		return
	}

	// 2 任务名正则、重名校验
	d := dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID())
	if err := m.Controller.CheckAutoLearnNameValid(d, gc.GetAuthProjectID(), autoLearnReq.Name); err != nil {
		log.WithError(err).Error("failed to checkAutoLearnIsValid")
		ctx.Error(c, err)
		return
	}

	for i, dataset := range autoLearnReq.Datasets {
		if dataset.SourceType == "Dataset" || dataset.SourceType == "Pair" {
			IDs := strings.Split(dataset.DataURI, ",")
			externalDataset, err := datasetCli.GetExternalDataset(gc.GetAuthTenantID(), gc.GetAuthProjectID(), IDs[0], IDs[1])
			if err != nil {
				log.Warn("failed to get external dataset")
				continue // 跳过当前循环，继续处理下一个 revision
			}

			// 确保 externalDataset 和 externalDataset.Object 不为 nil
			if externalDataset == nil || externalDataset.Object == nil {
				log.Warn("external dataset or object is nil")
				continue
			}
			datasets[i].DatasetMeta.ExternalDateSetID = externalDataset.Object.ExternalDateSetID
			datasets[i].DatasetMeta.ExternalRevisionID = externalDataset.Object.ExternalRevisionID
			datasets[i].DatasetMeta.BatchID = externalDataset.Object.BatchID
			datasets[i].DatasetMeta.InstitutionID = externalDataset.Object.InstitutionID
		}
	}

	// 3 新建autoLearn默认版本, 存入DB
	autoLearn, revision, err := m.Controller.LoadAndStoreAutoLearn(gc, m.App.Gin, &autoLearnReq, datasets)
	if err != nil {
		log.WithError(err).Error("failed to insertAutoLearn")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	revision.QuotaGroupName = autoLearnReq.QuotaGroupName
	go func() {
		// 4 数据集格式检查和加速，数据集格式校验主要是保证训练类型和数据集类型一致
		log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, revision.RevisionID)).Info("step1: start to do preparatory work")
		if err := m.Controller.CheckAndSpeedDataset(autoLearn, revision); err != nil {
			log.WithError(err).Error("CheckAndSpeedDataset failed")
			return
		}

		// 5 获取推荐算法
		if err := m.Controller.GetAndUpdateRecommendAlgorithm(autoLearn, revision); err != nil {
			log.WithError(err).Error("GetAndUpdateRecommendAlgorithm failed")
			return
		}

		// 6 创建autoLearn master pod
		if err := m.Controller.CreateAutoLearnMasterPod(autoLearn, revision); err != nil {
			log.WithError(err).Error("CreateAutoLearnMasterPod failed")
			return
		}
	}()

	m.Controller.TransformAutoLearn(autoLearn)

	utils.MetricEventDump(autoLearn.AutoLearnID, revision.RevisionID, autoLearn.Type, config.MetricAutoLearnCreation, map[string]any{
		utils.LogFieldRvCreatedAt:   time.Unix(revision.CreatedAt, 0).Format(time.RFC3339),
		utils.LogFieldSnapXImageURI: revision.SnapXImageURI,
		utils.LogFieldManagedBy:     revision.ManagedBy,
	})

	ctx.Success(c, &autoLearn)

	auditDatasets := []*resource.DatasetMeta{}
	for _, ds := range datasets {
		if ds.SourceType != types.SourceTypeDataset {
			continue
		}
		auditDatasets = append(auditDatasets, &resource.DatasetMeta{
			ID:    ds.DatasetMeta.ID,
			RvID:  ds.DatasetMeta.RevisionID,
			Level: ds.DatasetMeta.OriginLevel,
		})
	}
	auditlib.GinContextSetAuditData(c, map[string]any{
		auditlib.AuditRecordDatasets: auditDatasets,
	})
	resourceName := fmt.Sprintf("%s-%s", autoLearn.AutoLearnName, revision.RevisionName)
	auditlib.GinContextSendAudit(c, aisTypes.ResourceTypeAutomaticLearning, resourceName, string(aisTypes.OperationTypeCreate))
}

func (m *Mgr) GetAutoLearn(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)

	if err != nil {
		log.WithError(err).Error("failed to get autoLearn")
		ctx.Error(c, err)
		return
	}

	m.Controller.TransformAutoLearn(autoLearn)

	ctx.Success(c, autoLearn)
}

func (m *Mgr) DeleteAutoLearn(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn before deleting")
		ctx.Error(c, err)
		return
	}

	for i := range autoLearn.AutoLearnRevisions {
		if !apiServerUtils.AutolearnStateInFinal(autoLearn.AutoLearnRevisions[i].AutoLearnState) {
			log.Errorf("revision-%s not in final state", autoLearn.AutoLearnRevisions[i].RevisionID)
			ctx.Error(c, errors.ErrorInvalidParams)
			return
		}
	}

	if err := dao.DeleteAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID); err != nil {
		log.WithError(err).Error("failed to delete autoLearn")
		ctx.Error(c, err)
		return
	}

	// delete autoLearn task
	for _, rv := range autoLearn.AutoLearnRevisions {
		go autoLearnMgr.DeleteAutoLearnMasterPod(m.SchedulerClient, m.KubeClient, m.AisClient, autoLearn.ProjectID,
			utils.GetAutoLearnMasterPodName(autoLearn.AutoLearnID, rv.RevisionID))
	}

	ctx.SuccessNoContent(c)
	auditlib.GinContextSendAudit(c, aisTypes.ResourceTypeAutomaticLearning, autoLearn.AutoLearnName, string(aisTypes.OperationTypeDelete))
}

func (m *Mgr) LastAutoLearn(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearn, err := dao.LastAutoLearn(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()))
	if err != nil {
		log.WithError(err).Error("failed to list autoLearns")
		ctx.Error(c, err)
		return
	}
	autoLearnMgr.TransformRevision(autoLearn.AutoLearnRevision)

	ctx.SuccessWithList(c, autoLearn)
}

func (m *Mgr) ListAutoLearns(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	total, autoLearns, err := dao.ListListAutoLearns(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()))
	if err != nil {
		log.WithError(err).Error("failed to list autoLearns")
		ctx.Error(c, err)
		return
	}

	for i := range autoLearns {
		m.Controller.TransformAutoLearn(autoLearns[i])
	}

	ctx.SuccessWithList(c, &types.AutoLearnListResp{
		Total:      total,
		AutoLearns: autoLearns,
	})
}

func (m *Mgr) ListRevisions(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	total, revisions, err := dao.ListRevisions(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()))
	if err != nil {
		log.WithError(err).Error("failed to list autoLearns")
		ctx.Error(c, err)
		return
	}

	for _, rv := range revisions {
		autoLearnMgr.TransformRevision(rv)
	}

	ctx.SuccessWithList(c, &types.RevisionListResp{
		Total:     total,
		Revisions: revisions,
	})
}

func (m *Mgr) Options(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnsForOptions, err := dao.ListAutoLearnsForOptions(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()))
	if err != nil {
		log.WithError(err).Error("failed to DAOListAutoLearnsForOptions")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	ctx.Success(c, autoLearnMgr.TransformAutoLearnForOptions(autoLearnsForOptions))
}

func (m *Mgr) ReleaseByUser(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	req := gc.GetReleaseByUserRequest()
	if req == nil {
		log.Error("failed to GetReleaseByUserRequest")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearns, err := dao.ListAutoLearnByUserID(dao.NewDAO(m.App.MgoClient, req.TenantID), req.UserID)
	if err != nil {
		log.WithError(err).Error("failed to ListAutoLearnByUserID")
		ctx.Error(c, err)
		return
	}

	for _, autoLearn := range autoLearns {
		// delete autoLearn task
		for _, rv := range autoLearn.AutoLearnRevisions {
			if rv.CreatedBy.UserID != req.UserID {
				continue
			}

			if apiServerUtils.AutolearnStateInFinal(rv.AutoLearnState) {
				continue
			}

			log.Infof("release autolearn revision: %+v, %+v", autoLearn.AutoLearnID, rv.RevisionID)

			if err := m.Controller.StateMachine.Action(dao.NewDAO(m.App.MgoClient, req.TenantID), &types.StateEvent{
				T: types.StateEventTypeStop,
				A: types.EventActionSuccess,
			}, rv.RevisionID, types.StateEventCodeNothing); err != nil {
				log.WithError(err).Error("change autolearn state failded")
				continue
			}

			// 记录成功数，不论当前处于何种状态，主动停止记录为成功
			extra := map[string]any{
				utils.LogFieldRvCreatedAt:   time.Unix(rv.CreatedAt, 0).Format(time.RFC3339),
				utils.LogFieldCompletedWay:  utils.CompletedWayManualStopBfLearning,
				utils.LogFieldSnapXImageURI: rv.SnapXImageURI,
				utils.LogFieldManagedBy:     rv.ManagedBy,
			}
			autoLearnMgr.TransformRevisionState(rv)
			if rv.AutoLearnState == types.AutoLearnStateLearning {
				extra[utils.LogFieldCompletedWay] = utils.CompletedWayManualStopAfLearning
			}
			log.WithFields(log.Fields{"autoLearnID": autoLearn.AutoLearnID, "revisionID": rv.RevisionID}).Info(config.MetricAutoLearnCompleted)
			utils.MetricEventDump(autoLearn.AutoLearnID, rv.RevisionID, rv.Type, config.MetricAutoLearnCompleted, extra)
		}
	}
	ctx.SuccessNoContent(c)
}
