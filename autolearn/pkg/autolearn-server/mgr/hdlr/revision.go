package hdlr

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	autoLearnMgr "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/autolearn"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	apiServerUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

// @Summary 创建自动学习任务版本(新建版本页面)
// @tags autoLearn
// @Produce application/json
// @Description 创建自动学习任务版本
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autoLearnID, 当前autoLearn的autoLearnID"
// @Param createRevision body types.RevisionCreateRequest true "autoLearn版本创建请求"
// @Success 200 {object} types.AutoLearnRevision "创建成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions [post]
func (m *Mgr) CreateAutoLearnRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	var request types.RevisionCreateRequest
	if err := gc.GetRequestJSONBody(&request); err != nil {
		log.WithError(err).Error("failed to parse createRevisionRequest")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	// 校验autoLearn是否存在
	autoLearn, err := dao.GetAutoLearnByIDWithOriginalInfo(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn before revision creating")
		ctx.Error(c, err)
		return
	}

	if request.RevisionName != "" {
		if err := autoLearnMgr.CheckRevisionNameValid(autoLearn, request.RevisionName); err != nil {
			log.WithError(err).Error("invalid or duplicate revision name")
			ctx.Error(c, err)
			return
		}
	}

	// 1 前置综合校验，以及填充数据集信息
	var datasets []*types.Dataset
	if err := m.Controller.CreatePreCheckAndGetDatasetInfo(
		autoLearn,
		gc,
		request.SameCreateRequest,
		&datasets,
		m.App.Conf.AuthConf.Debug,
	); err != nil {
		log.WithError(err).Error("failed to createPreCheckAndGetDatasetInfo")
		ctx.Error(c, err)
		return
	}

	// 4 存入DB
	newRevision, err := m.Controller.LoadAndStoreRevision(gc, m.App.Gin, &request, datasets, autoLearn)
	if err != nil {
		log.WithError(err).Error("failed to create autoLearn revision")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	newRevision.QuotaGroupName = request.QuotaGroupName

	go func() {
		// 数据集格式转换和加速
		log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, newRevision.RevisionID)).
			Info("step1: start to do preparatory work")
		if err := m.Controller.CheckAndSpeedDataset(autoLearn, newRevision); err != nil {
			log.WithError(err).Error("failed to CheckAndSpeedDataset")
			return
		}

		// 5 获取推荐算法
		if err := m.Controller.GetAndUpdateRecommendAlgorithm(autoLearn, newRevision); err != nil {
			log.WithError(err).Error("failed to GetAndUpdateRecommendAlgorithm")
			return
		}

		// 7 创建autoLearn master pod
		if err := m.Controller.CreateAutoLearnMasterPod(autoLearn, newRevision); err != nil {
			log.WithError(err).Error("failed to CreateAutoLearnMasterPod")
			return
		}
	}()

	autoLearnMgr.TransformRevision(newRevision)

	utils.MetricEventDump(autoLearn.AutoLearnID, newRevision.RevisionID, newRevision.Type, config.MetricAutoLearnCreation, map[string]any{
		utils.LogFieldRvCreatedAt:   time.Unix(newRevision.CreatedAt, 0).Format(time.RFC3339),
		utils.LogFieldSnapXImageURI: newRevision.SnapXImageURI,
		utils.LogFieldManagedBy:     newRevision.ManagedBy,
	})

	ctx.Success(c, newRevision)

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
	resourceName := fmt.Sprintf("%s-%s", autoLearn.AutoLearnName, newRevision.RevisionName)
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeAutomaticLearning, resourceName, string(aisConst.OperationTypeStart))
}

// @Summary 获取自动学习任务指定版本
// @tags autoLearn
// @Produce application/json
// @Description 获取自动学习任务版本
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn id"
// @Param revisionID path string true "revision id"
// @Success 200 {object} types.AutoLearnRevision "获取成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID} [get]
func (m *Mgr) GetAutoLearnRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}
	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param revisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revision, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revision")
		ctx.Error(c, err)
		return
	}

	if strings.HasSuffix(c.Request.URL.Path, "/status") {
		action := ""
		if revision.AutoLearnState == types.AutoLearnStateStopped {
			action = "Stop"
		}
		resp := struct {
			Action string `json:"action"`
		}{
			Action: action,
		}
		ctx.Success(c, resp)
		return
	}

	autoLearnMgr.TransformRevision(revision)

	ctx.Success(c, revision)
}

// @Summary 获取自动学习任务所有版本
// @tags autoLearn
// @Produce application/json
// @Description 获取自动学习任务所有版本
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn id"
// @Param pageSize query integer false "分页大小" default(10)
// @Param page query integer false "页码" default(1)
// @Param category query string false "获取全部" Enums("all")
// @Param keywords query string false "查询关键词，支持实验名进行搜索"
// @Param exactly query bool false "精确查询，true表示精确查询"
// @Success 200 {object} types.RevisionListResp "获取成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions [get]
func (m *Mgr) ListAutoLearnRevisions(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	visibility := gc.Query("visibility")
	if visibility != "" && visibility != types.VisibilityOptionVisible && visibility != types.VisibilityOptionHidden {
		log.Errorf("invalid param visibility: %s", visibility)
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	total, existInvisible, revisions, err := dao.ListAutoLearnRevisions(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revisions")
		ctx.Error(c, err)
		return
	}

	for i := range revisions {
		autoLearnMgr.TransformRevision(revisions[i])
	}

	ctx.SuccessWithList(c, &types.RevisionListResp{
		Total:          total,
		ExistInvisible: existInvisible,
		Revisions:      revisions,
	})
}

// @Summary 删除自动学习任务版本(删除自动学习任务版本页面)
// @tags autoLearn
// @Produce application/json
// @Description 删除自动学习任务版本
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn id(任务 id)"
// @Param revisionID path string true "revision id(版本 id)"
// @Success 204 "请求成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID} [delete]
func (m *Mgr) DeleteAutoLearnRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param revisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnBasicInfoByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn")
		ctx.Error(c, err)
		return
	}

	revision, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revision")
		ctx.Error(c, err)
		return
	}

	if !apiServerUtils.AutolearnStateInFinal(revision.AutoLearnState) {
		log.Errorf("revision-%s not in final state", revision.RevisionID)
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	if err := dao.DeleteALAndALRevision(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	}); err != nil {
		log.WithError(err).Error("failed to delete autoLearn revision")
		ctx.Error(c, err)
		return
	}

	// delete autolearn task
	go autoLearnMgr.DeleteAutoLearnMasterPod(m.SchedulerClient, m.KubeClient, m.AisClient, autoLearn.ProjectID,
		utils.GetAutoLearnMasterPodName(autoLearn.AutoLearnID, revisionID))
	ctx.SuccessNoContent(c)
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeAutomaticLearning, autoLearn.AutoLearnName, string(aisConst.OperationTypeDelete))
}

// @Summary 停止自动学习任务版本(停止页面)
// @tags autoLearn
// @Produce application/json
// @Description 停止自动学习任务版本
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn id(任务 id)"
// @Param revisionID path string true "revision id(版本 id)"
// @Param updateRevisionReq body types.UpdateRevisionRequest true "版本更新信息"
// @Success 204 "请求成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID} [patch]
func (m *Mgr) UpdateAutoLearnRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param revisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	var req types.UpdateRevisionRequest
	if err := gc.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("failed to parse request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn before stopping")
		ctx.Error(c, err)
		return
	}
	basicInfo := &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	}

	rv, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), basicInfo)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revision before stopping")
		ctx.Error(c, err)
		return
	}
	//  版本名重名、正则校验及更新
	if req.RevisionName != "" {
		if err = autoLearnMgr.CheckRevisionNameValid(autoLearn, req.RevisionName); err != nil {
			log.WithError(err).Error("invalid or duplicate revision name")
			ctx.Error(c, err)
			return
		}
		if err = dao.UpdateALRevisionName(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), basicInfo, req.RevisionName); err != nil {
			log.WithError(err).Error("failed to UpdateALRevisionName")
			ctx.Error(c, err)
			return
		}
	}

	// 状态校验及更新
	if req.State != "" {
		if req.State != types.ActionStateStopped {
			log.Error("invalid autoLearn state")
			ctx.Error(c, errors.ErrorInvalidParams)
			return
		}

		if apiServerUtils.AutolearnStateInFinal(rv.AutoLearnState) {
			log.Error("invalid autoLearn state")
			ctx.Error(c, errors.ErrorInvalidParams)
			return
		}

		if err := m.Controller.StateMachine.Action(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &types.StateEvent{
			T: types.StateEventTypeStop,
			A: types.EventActionSuccess,
		}, rv.RevisionID, types.StateEventCodeNothing); err != nil {
			log.WithError(err).Error("change autolearn state failded")
			ctx.Error(c, errors.ErrorInvalidParams)
			return
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
		log.WithFields(log.Fields{"autoLearnID": autoLearnID, "revisionID": rv.RevisionID}).Info(config.MetricAutoLearnCompleted)
		utils.MetricEventDump(autoLearn.AutoLearnID, rv.RevisionID, rv.Type, config.MetricAutoLearnCompleted, extra)
	}

	// 隐藏、取消隐藏
	if req.Visibility != "" {
		if req.Visibility != types.VisibilityOptionVisible && req.Visibility != types.VisibilityOptionHidden {
			log.Errorf("invalid params: %s", req.Visibility)
			ctx.Error(c, errors.ErrorInvalidParams)
			return
		}
		isHidden := false
		if req.Visibility == types.VisibilityOptionHidden {
			isHidden = true
		}
		if err = dao.UpdateALRevisionVisibility(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), basicInfo, isHidden); err != nil {
			log.WithError(err).Error("failed to UpdateALRevisionVisibility")
			ctx.Error(c, err)
			return
		}
	}

	ctx.SuccessNoContent(c)
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeAutomaticLearning, autoLearn.AutoLearnName, string(aisConst.OperationTypeUpdate))
}

// @Summary 重建一个学习任务版本
// @tags autoLearn
// @Produce application/json
// @Description 停止自动学习任务版本
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn id(任务 id)"
// @Param revisionID path string true "revision id(版本 id)"
// @Success 204 "请求成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID}/rebuild [POST]
func (m *Mgr) RebuildAutoLearnRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param revisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn before stopping")
		ctx.Error(c, err)
		return
	}
	basicInfo := &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	}

	rv, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), basicInfo)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revision before stopping")
		ctx.Error(c, err)
		return
	}

	if rv.SolutionType == types.SolutionTypeCustom {
		exist, err := m.Controller.CheckCodebaseExist(gc.GetAuthTenantID(), gc.GetAuthProjectID(), rv)
		if err != nil || !exist {
			log.WithError(err).Error("failed to get autoLearn revision before stopping")
			ctx.Error(c, errors.ErrorAutoLearnRebuildAlgorithmUnavailable)
			return
		}
	}

	if !apiServerUtils.AutolearnStateInFinal(rv.AutoLearnState) &&
		rv.IsOptimized {
		log.Error("invalid autoLearn state")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionCreatReq := autoLearnMgr.GetRebuildRevisionCreateRequest(rv)
	data := []*types.DataSource{}
	for i := range rv.Datasets {
		data = append(data, &types.DataSource{
			SourceType: types.SourceTypeSDS,
			DataURI:    rv.Datasets[i].SDSPath,
			Type:       rv.Datasets[i].Type,
		})
	}
	revisionCreatReq.Datasets = data
	// 4 存入DB
	newRevision, err := m.Controller.LoadAndStoreRevision(gc, m.App.Gin, revisionCreatReq, rv.Datasets, autoLearn)
	if err != nil {
		log.WithError(err).Error("failed to create autoLearn revision")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	newRevision.QuotaGroupName = revisionCreatReq.QuotaGroupName

	go func() {
		// 数据集格式转换和加速
		log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, newRevision.RevisionID)).
			Info("step1: start to do preparatory work")
		if err := m.Controller.CheckAndSpeedDataset(autoLearn, newRevision); err != nil {
			log.WithError(err).Error("failed to CheckAndSpeedDataset")
			return
		}

		// 5 获取推荐算法
		if err := m.Controller.GetAndUpdateRecommendAlgorithm(autoLearn, newRevision); err != nil {
			log.WithError(err).Error("failed to GetAndUpdateRecommendAlgorithm")
			return
		}

		// 7 创建autoLearn master pod
		if err := m.Controller.CreateAutoLearnMasterPod(autoLearn, newRevision); err != nil {
			log.WithError(err).Error("failed to CreateAutoLearnMasterPod")
			return
		}
	}()

	autoLearnMgr.TransformRevision(newRevision)

	utils.MetricEventDump(autoLearn.AutoLearnID, newRevision.RevisionID, newRevision.Type, config.MetricAutoLearnCreation, map[string]any{
		utils.LogFieldRvCreatedAt:   time.Unix(newRevision.CreatedAt, 0).Format(time.RFC3339),
		utils.LogFieldSnapXImageURI: newRevision.SnapXImageURI,
		utils.LogFieldManagedBy:     newRevision.ManagedBy,
	})

	ctx.Success(c, newRevision)

	auditDatasets := []*resource.DatasetMeta{}
	for _, ds := range rv.Datasets {
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
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeAutomaticLearning, autoLearn.AutoLearnName, string(aisConst.OperationTypeStart))
}

// @Summary 接收InternalMethodPostreRebuild 上报信息
// @tags autoLearn
// @Produce application/json
// @Description 供内部Internal agent 上报学习状态、sds文件和model文件
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn ID(任务 ID"
// @Param revisionID path string true "revision ID(版本 ID)"
// @Param internalAgentInfoReq body types.InternalAgentInfoReq true "更新信息"
// @Success 204 "更新成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID}/internal [patch]
func (m *Mgr) InternalAgentInfo(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param revisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	var req types.InternalAgentInfoReq
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to parse request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearnID, revisionID))
	logs.Infof("internal request: %+v:", req)

	autoLearn, err := dao.GetAutoLearnBasicInfoByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn")
		ctx.Error(c, errors.ErrorAutoLearnNotFound)
		return
	}

	// 1 任务、版本存在性校验
	revision, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		logs.WithError(err).Error("autoLearn not in database and it will be deleted")
		ctx.Error(c, errors.ErrorAutoLearnRvNotFound)
		return
	}

	// 2 前置检查
	if err := autoLearnMgr.InternalPreCheck(m.OssClient, &req, revision); err != nil {
		logs.WithError(err).Error("failed to internalPreCheck")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}
	basicInfo := &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	}

	// 3 状态转换
	state, event, eventCode, ok := m.Controller.ActionToState(req.State, req.ErrorCode)
	if !ok {
		logs.Error("failed to convert autoLearn state")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	d := dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID())

	if revision.AutoLearnState != state {
		// 有未创建 ajob，前期就直接失败的可能
		if revision.AutoLearnState == types.AutoLearnStateMasterPodRunning {
			if state == types.AutoLearnStateLearnFailed {
				code := m.Controller.TransformErrorCode(req.ErrorCode)
				if err := m.Controller.StateMachine.Action(d, &types.StateEvent{
					T: types.StateEventTypeWorkerAJobCreate,
					A: types.EventActionStepinTimeout,
				}, revisionID, code); err != nil {
					logs.Error("failed to update autoLearn state")
					ctx.Error(c, errors.ErrorInternal)
					return
				}
				ctx.SuccessNoContent(c)
				return
			}
		}

		if err := m.Controller.StateMachine.Action(d, event, revisionID, eventCode); err != nil {
			logs.Error("failed to update autoLearn state")
			ctx.Error(c, errors.ErrorInternal)
			return
		}
	}

	// 5 迭代版本重复性校验，如果为已存在迭代版本号则需检查是否已存在评测任务(禁止重复评测),对于新迭代版本号往evaluationDetail新增元素，
	var evaluationID, evaluationName string
	var flag bool
	if req.IterationNumber != 0 {
		for i := range revision.EvaluationDetails {
			if revision.EvaluationDetails[i].EvaluationID != "" {
				evaluationID = revision.EvaluationDetails[i].EvaluationID
				evaluationName = revision.EvaluationDetails[i].EvaluationName
			}

			if revision.EvaluationDetails[i].IterationNumber == req.IterationNumber {
				flag = true
				if revision.EvaluationDetails[i].EvaluationJobID != "" {
					logs.Warnf("iterationNumber %d already has evaluation job", req.IterationNumber)
					req.PredictSDS = ""
				}
				if revision.EvaluationDetails[i].ModelFilePath != "" {
					logs.Warnf("iterationNumber %d already has evaluation model file", req.IterationNumber)
					req.ModelFilePath = ""
				}
				break
			}
		}

		if !flag {
			if err := dao.AddEvaluationDetail(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revision.RevisionID,
				&types.EvaluationDetail{IterationNumber: req.IterationNumber, IsDeleted: false, Epoch: req.Epoch}); err != nil {
				logs.WithError(err).Error("failed to update autoLearn revision sds file path")
				ctx.Error(c, err)
				return
			}
		}
	}

	// 6 更新2：建立评测任务, 更新sdsPath和evaluation job info
	if req.PredictSDS != "" {
		var autoLearnDetailEval *types.EvaluationDetail
		if evaluationID == "" {
			autoLearnDetailEval, err = client.CreateEvaluation(autoLearn, revision, req.PredictSDS)
		} else {
			autoLearnDetailEval, err = client.CreateEvaluationJob(autoLearn, revision, evaluationID, evaluationName, req.PredictSDS)
		}

		// 评测错误不能返回
		if err != nil {
			logs.WithError(err).Error("failed to create evaluation")
			if e := dao.AddEvaluationDetailJob(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revisionID, req.IterationNumber,
				&types.EvaluationDetail{
					EvaluationJobState: client.EvalJobResultStateFailed,
					SdsFilePath:        req.PredictSDS,
					Reason:             "failed to create eval job",
				},
			); e != nil {
				logs.WithError(e).Errorf("failed to update revision evaluation job to %s", client.EvalJobResultStateFailed)
			}
		} else if err := dao.AddEvaluationDetailJob(
			dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revisionID, req.IterationNumber, autoLearnDetailEval); err != nil {
			logs.WithError(err).Error("failed to update autoLearn revision evaluation info")
		}
	}

	// 7 更新3： 更新modelFile路径
	if req.ModelFilePath != "" {
		if err := dao.UpdateEvaluationDetailModelPath(
			dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revisionID, req.IterationNumber, req.ModelFilePath); err != nil {
			logs.WithError(err).Error("failed to update DAOUpdateRevisionEvalModelPath")
			ctx.Error(c, err)
			return
		}
	}

	// 8 更新4： 更新Score值以及MaxScore值
	if req.AverageRecall >= 0 {
		req.AverageRecall, _ = decimal.NewFromFloat(req.AverageRecall).Round(config.EvalMetricPrecision).Float64()
		if err := dao.UpdateAlgorithmScore(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revisionID, req.SolutionName, req.SolutionID, &types.Score{
			Time: req.UpdateTime, Value: req.AverageRecall, IterationNumber: req.IterationNumber, Epoch: req.Epoch,
		}, revision.Type, revision.SolutionType); err != nil {
			logs.WithError(err).Error("failed to UpdateAlgorithmScore")
			ctx.Error(c, err)
			return
		}
	}

	switch revision.Type {
	case aisConst.AppTypeDetection, aisConst.AppTypeKPS, aisConst.AppTypeSegmentation, aisConst.AppTypeOCR, aisConst.AppTypeText2Text, aisConst.AppTypeImage2Text:
		if req.AverageRecall > revision.EvalMetric.MaxScore {
			if err := dao.UpdateDetEvalMetricMaxScore(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revisionID, req.AverageRecall); err != nil {
				logs.WithError(err).Error("failed to UpdateEvalMetricMaxScore")
				ctx.Error(c, err)
				return
			}
		}
	case aisConst.AppTypeClassification, aisConst.AppTypeClassificationAndRegression, aisConst.AppTypeRegression:
		if req.AverageRecall > revision.ClfEvalMetric.MaxScore {
			if err := dao.UpdateClfEvalMetricMaxScore(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), revisionID, req.AverageRecall); err != nil {
				logs.WithError(err).Error("failed to UpdateClfEvalMetricMaxScore")
				ctx.Error(c, err)
				return
			}
		}
	}

	// 9 更新5： 更新score曲线是否平滑
	if req.EarlyStop {
		if err := dao.UpdateALRevisionEarlyStop(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), basicInfo, req.EarlyStop); err != nil {
			logs.WithError(err).Error("failed to DAOUpdateRevisionEarlyStop")
			ctx.Error(c, err)
			return
		}
	}

	// 记录 metric event
	metricMap := map[string]any{
		utils.LogFieldRvCreatedAt:   time.Unix(revision.CreatedAt, 0).Format(time.RFC3339),
		utils.LogFieldSnapXImageURI: revision.SnapXImageURI,
		utils.LogFieldManagedBy:     revision.ManagedBy,
	}
	var metricString string
	if state == types.AutoLearnStateLearning {
		metricString = config.MetricAutoLearnLearning
		metricMap[utils.LogFieldRvCreatedAt] = time.Unix(revision.CreatedAt, 0).Format(time.RFC3339)
	}

	if state == types.AutoLearnStateLearnFailed {
		metricMap[utils.LogFieldFailedReason] = types.EventCodeMessage[eventCode]
		metricString = config.MetricAutoLearnFailedReason
	}

	if state == types.AutoLearnStateCompleted {
		metricMap[utils.LogFieldCompletedWay] = utils.CompletedWayNormal
		metricString = config.MetricAutoLearnCompleted
	}

	utils.MetricEventDump(autoLearn.AutoLearnID, revision.RevisionID, revision.Type, metricString, metricMap)

	ctx.SuccessNoContent(c)
}

// @Summary 下载曲线图
// @tags autoLearn
// @Produce application/json
// @Description 可以下载markdown或者json格式
// @Param x-kb-project header string true "项目名"
// @Param type query string true "格式类型" Enums("Markdown, Json")
// @Param autolearnID path string true "autolearn ID(任务 ID)"
// @Param revisionID path string true "revision ID(版本 ID)"
// @Success 204 {array} byte "下载成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID}/exportScore [get]
func (m *Mgr) ExportAlgorithmScore(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	log.Debugf("project: %s", gc.GetAuthProjectID())

	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param revisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	ty, exist := gc.GetRequestParam("type")
	if !exist || !(ty == config.AlgorithmScoreTypeJSON || ty == config.AlgorithmScoreTypeMarkdown) {
		log.Error("invalid param type")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnBasicInfoByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn")
		ctx.Error(c, err)
		return
	}
	rv, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revision")
		ctx.Error(c, err)
		return
	}

	resp := types.ExportAlgorithmScoreResp{
		AutoLearnName: autoLearn.AutoLearnName,
		RevisionName:  rv.RevisionName,
		Train:         autoLearnMgr.GetTrainDatasetName(rv.Datasets),
		Validation:    autoLearnMgr.GetValDatasetName(rv.Datasets),
		EvalMetric:    rv.EvalMetric,
		Algorithms:    rv.Algorithms,
		BestDetector:  autoLearnMgr.GetBestDetector(rv.Type, rv.Algorithms),
	}

	if ty == config.AlgorithmScoreTypeMarkdown {
		markdown, err := autoLearnMgr.AlgorithmScoreMarkdown(&resp)
		if err != nil {
			log.WithError(err).Error("failed to get AlgorithmScoreMarkdown")
			ctx.Error(c, errors.ErrorInternal)
			return
		}
		ctx.SuccessWithByte(c, fmt.Sprintf("score-%s.md", revisionID), markdown)
		return
	}

	for i := range resp.Algorithms {
		resp.Algorithms[i].OriginalInfo = ""
	}
	content, err := json.Marshal(resp)
	if err != nil {
		log.WithError(err).Error("failed to get AlgorithmScoreJson")
		ctx.Error(c, err)
		return
	}
	ctx.SuccessWithByte(c, fmt.Sprintf("score-%s.json", revisionID), content)
}

// @Summary 下载模型文件
// @tags autoLearn
// @Produce application/json
// @Description 下载模型文件
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn ID(任务 ID)"
// @Param revisionID path string true "revision ID(版本 ID)"
// @Param iterationNumber query integer true "iteration number(迭代号)"
// @Success 200 {string} {string} "下载成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID}/model.zip [get]
func (m *Mgr) ModelFileDownload(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	autoLearnID, exist := gc.GetRequestParam("autolearnID")
	if !exist || autoLearnID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revisionID, exist := gc.GetRequestParam("revisionID")
	if !exist || revisionID == "" {
		log.Error("invalid param autoLearnID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	iterationNumber, err := strconv.ParseUint(gc.Query("iterationNumber"), 10, 64)
	if err != nil || iterationNumber == 0 {
		log.WithError(err).Error("invalid param iterationNumber")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	revision, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn revision")
		ctx.Error(c, err)
		return
	}

	modelFilePath := apiServerUtils.GetModelFilePathByIterationNum(int32(iterationNumber), revision.EvaluationDetails)
	if modelFilePath == "" {
		log.Warn("model file is not ready")
		ctx.Error(c, errors.ErrorModelNotFound)
		return
	}

	bucket, key, err := oss.ParseOSSPath(modelFilePath)
	if err != nil {
		log.WithError(err).Error("failed to parse oss URI")
		ctx.Error(c, err)
		return
	}

	downloadURL, err := m.OssProxyClient.GetPresignURLWithGetObjectInput(
		&s3.GetObjectInput{
			Bucket:              aws.String(bucket),
			Key:                 aws.String(key),
			ResponseContentType: aws.String("application/octet-stream"),
		},
		config.ExpireTimeModelFileDownload*time.Second,
	)
	if err != nil {
		log.WithError(err).Error("failed to GetPresignURLWithGetObjectInput")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	c.Header("Content-Security-Policy", "upgrade-insecure-requests")
	ctx.Success(c, downloadURL)
}

// @Summary 创建自动学习优化任务版本
// @tags autoLearn
// @Produce application/json
// @Description autoLearn版本优化请求
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autoLearnID, 当前autoLearn的autoLearnID"
// @Param revisionID path string true "revision ID(版本 ID)"
// @Success 200 {object} types.OptimizationResp "创建成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID}/optimization [post]
func (m *Mgr) AutoLearnRevisionOptimization(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, revisionID, err := getAutoLearnAndRevisionID(gc)
	if err != nil {
		log.WithError(err).Error("failed to get getAutoLearnAndRevisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	autoLearn, err := dao.GetAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn before revision creating")
		ctx.Error(c, err)
		return
	}
	rv, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		log.WithError(err).Error("failed to GetAutoLearnRevisionByID")
		ctx.Error(c, err)
		return
	}

	// 1 前置检查
	autoLearnMgr.TransformRevision(rv)
	evalDetail, err := autoLearnMgr.PreCheckAndGetOptimization(m.OssClient, rv)
	if err != nil {
		log.Error("failed PreCheckOptimization")
		ctx.Error(c, err)
		return
	}

	// 2 构建优化版本创建请求
	var datasets []*types.Dataset
	revisionCreatReq := autoLearnMgr.GetOpRevisionCreateRequest(rv)
	if err := m.Controller.CreatePreCheckAndGetDatasetInfo(
		autoLearn,
		gc,
		revisionCreatReq.SameCreateRequest,
		&datasets,
		m.App.Conf.AuthConf.Debug,
	); err != nil {
		log.WithError(err).Error("failed to createPreCheckAndGetDatasetInfo")
		ctx.Error(c, err)
		return
	}

	// 3 版本名重名、正则校验
	if err := autoLearnMgr.CheckRevisionNameValid(autoLearn, revisionCreatReq.RevisionName); err != nil {
		log.WithError(err).Error("invalid or duplicate revision name")
		ctx.Error(c, err)
		return
	}

	// 4 存入DB
	newRevision, trainSDS := m.Controller.NewOpAutoLearnRevision(gc, m.App.Gin, revisionCreatReq, datasets, autoLearn)
	if err := dao.InsertRAE(m.App.MgoClient, newRevision, gc.GetAuthTenantID(), gc.GetAuthProjectID()); err != nil {
		log.WithError(err).Error("failed to create autoLearn revision")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	m.Controller.StateMachine.SetLock(newRevision.RevisionID)

	newRevision.QuotaGroupName = revisionCreatReq.QuotaGroupName

	go func() {
		log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, newRevision.RevisionID)).Info("step1: start to do preparatory work")
		// 5 清洗
		if err := autoLearnMgr.CreateAndCheckReLabelTask(gc, m.App.MgoClient, m.OssClient, newRevision, autoLearn.AutoLearnName, trainSDS, evalDetail); err != nil {
			return
		}

		// 6 数据集格式转换和加速
		if err := m.Controller.CheckAndSpeedDataset(autoLearn, newRevision); err != nil {
			return
		}

		// 5 获取推荐算法
		if err := m.Controller.GetAndUpdateRecommendAlgorithm(autoLearn, newRevision); err != nil {
			return
		}

		// 7 创建autoLearn master pod
		if err := m.Controller.CreateAutoLearnMasterPod(autoLearn, newRevision); err != nil {
			return
		}
	}()

	utils.MetricEventDump(autoLearn.AutoLearnID, newRevision.RevisionID, newRevision.Type, config.MetricAutoLearnCreation, map[string]any{
		utils.LogFieldRvCreatedAt:   time.Unix(newRevision.CreatedAt, 0).Format(time.RFC3339),
		utils.LogFieldSnapXImageURI: newRevision.SnapXImageURI,
		utils.LogFieldManagedBy:     newRevision.ManagedBy,
	})

	ctx.Success(c, &types.OptimizationResp{AutoLearnID: rv.AutoLearnID, RevisionID: newRevision.RevisionID, RevisionName: newRevision.RevisionName})
}

func (m *Mgr) ExportBasicInfo(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	autoLearnID, revisionID, err := getAutoLearnAndRevisionID(gc)
	if err != nil {
		log.WithError(err).Error("failed to get getAutoLearnAndRevisionID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	rv, err := dao.GetAutoLearnRevisionByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), &dao.BasicInfo{
		Project:     gc.GetAuthProjectID(),
		AutoLearnID: autoLearnID,
		RevisionID:  revisionID,
	})
	if err != nil {
		log.WithError(err).Error("failed to GetAutoLearnRevisionByID")
		ctx.Error(c, err)
		return
	}

	// 参考：https://git-core.megvii-inc.com/FaceTeam/snapx/snapx-resource/-/blob/master/snapx-resource/examples/create_det_task/template.yml
	yamlInfo, err := autoLearnMgr.BasicInfoForYaml(rv)
	if err != nil {
		log.WithError(err).Error("failed to export basic yaml info")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	ctx.SuccessWithByte(c, fmt.Sprintf("autolearn-template-%s-%s.yaml", rv.AutoLearnName, rv.RevisionName), yamlInfo)
}
