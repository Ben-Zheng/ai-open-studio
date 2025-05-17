package hdlr

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	evaluationMgr "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/evaluation"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

// @Summary 评测子任务失败重试
// @tags evaluation
// @Produce application/json
// @Description 失败的评测子任务支持重试操作
// @Param x-kb-project header string true "项目名"
// @Param autolearnID path string true "autolearn ID(任务 ID)"
// @Param revisionID path string true "revision ID(版本 ID)"
// @Param iterationNumber query integer true "iteration number(迭代号)"
// @Success 200 {string} {string} "重试成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /autolearns/{autolearnID}/revisions/{revisionID}/evaluations [post]
func (m *Mgr) EvalJobRetry(c *gin.Context) {
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

	autoLearn, err := dao.GetAutoLearnByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), autoLearnID)
	if err != nil {
		log.WithError(err).Error("failed to get autoLearn")
		ctx.Error(c, errors.ErrorAutoLearnNotFound)
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

	evalDetail, err := evaluationMgr.PreCheckEvalRetry(revision, iterationNumber)
	if err != nil {
		log.WithError(err).Error("failed to preCheckEvalRetry")
		ctx.Error(c, err)
		return
	}

	// 先更新数据库返回
	if err := dao.RetryEvaluationJob(m.App.MgoClient, gc.GetAuthTenantID(), revisionID, &types.EvaluationDetail{
		IterationNumber:    int32(iterationNumber),
		EvaluationJobState: evalTypes.EvalJobResultStateNone,
		EvaluationID:       evalDetail.EvaluationID,
		EvaluationName:     evalDetail.EvaluationName,
		SdsFilePath:        evalDetail.SdsFilePath,
		ModelFilePath:      evalDetail.ModelFilePath,
		IsDeleted:          false,
	}); err != nil {
		log.WithError(err).Error("failed to RetryEvaluationJob")
		ctx.Error(c, err)
		return
	}

	// 后台创建评测任务
	go func() {
		logs := log.WithField(config.AutoLearnPrefix, fmt.Sprintf("%s/%s", autoLearnID, revisionID))
		var res *types.EvaluationDetail
		if evalDetail.EvaluationID == "" {
			res, err = client.CreateEvaluation(autoLearn, revision, evalDetail.SdsFilePath)
		} else {
			res, err = client.CreateEvaluationJob(autoLearn, revision, evalDetail.EvaluationID, evalDetail.EvaluationName, evalDetail.SdsFilePath)
		}
		if err != nil {
			logs.WithError(err).Error("failed to create evaluation job")
			if e := dao.UpdateEvaluationDetailStateAndReason(dao.NewDAO(m.App.MgoClient, autoLearn.TenantID), revisionID,
				"failed to create eval job", evalTypes.EvalJobResultStateFailed, int32(iterationNumber)); e != nil {
				logs.WithError(err).Error("failed to update evalJob state to Failed")
			}
			return
		}

		if err := dao.AddEvaluationDetailJob(dao.NewDAO(m.App.MgoClient, autoLearn.TenantID), revisionID, int32(iterationNumber), res); err != nil {
			logs.WithError(err).Error("failed to UpdateEvaluationDetailJobInfo")
			return
		}
	}()

	ctx.SuccessNoContent(c)
}

// @Summary 创建评测对比任务
// @tags evaluation
// @Produce application/json
// @Description 创建评测对比任务
// @Param x-kb-project header string true "项目名"
// @Param request body types.EvalComparisonReq true "评测对比任务创建请求"
// @Success 200 {object} types.EvalComparisonSnapshotResp "创建成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /v1/comparisons [post]
func (m *Mgr) CreateComparison(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	project := gc.GetAuthProjectID()
	tenant := gc.GetAuthTenantID()
	var req types.EvalComparisonReq
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to GetRequestJSONBody")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	if err := evaluationMgr.PreCheckCreateComparison(&req); err != nil {
		log.WithError(err).Error("failed to PreCheckCreateComparison")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	// 1 获取autoLearnID、revisionID对应的任务名、版本名，并校验版本是否存在
	autoLearnIDs, rvIDs := evaluationMgr.GetAutoLearnIDsFromEvalComparisonReq(&req)
	autoLearnIDToName, revisionIDToName, err := dao.GetAlAndRvNamesByAutoLearnIDs(dao.NewDAO(m.App.MgoClient, tenant), project, autoLearnIDs)
	if err != nil {
		log.WithError(err).Error("failed to GetAutoLearnAndRvNamesByAutoLearnIDs")
		ctx.Error(c, err)
		return
	}
	if err := evaluationMgr.SecondCheckCreateComparison(rvIDs, revisionIDToName); err != nil {
		log.WithError(err).Error("failed to SecondCheckCreateComparison")
		ctx.Error(c, err)
		return
	}

	// 2 根据版本名获取最后一次成功的评测任务job的相关信息
	comparisonEvalJobs, err := dao.GetComparisonEvalJobsByRevisionIDs(dao.NewDAO(m.App.MgoClient, tenant), rvIDs, autoLearnIDToName, revisionIDToName)
	if err != nil {
		log.WithError(err).Error("failed to GetComparisonEvalJobsByRevisionIDs")
		ctx.Error(c, err)
		return
	}
	if len(comparisonEvalJobs) != len(req.ComparisonRevisions) {
		log.Errorf("not all versions support evaluation task comparison: supported %d, expected %d", len(comparisonEvalJobs), len(req.ComparisonRevisions))
		ctx.Error(c, errors.ErrorNoSuccessfulEvalJob)
		return
	}

	// 3 根据版本名获取实际的算法信息
	revisionIDToAlgorithm, err := dao.FindAlgorithmByRevisionIDs(dao.NewDAO(m.App.MgoClient, tenant), rvIDs, false)
	if err != nil {
		log.Error("failed to get FindAlgorithmByRevisionIDs")
		ctx.Error(c, err)
		return
	}

	// 4 从评测服务创建真正的评测对比任务，获取评测服务生成的评测对比ID
	jobIDs := evaluationMgr.GetEvalJobIDsFromComparisonEvalJobs(comparisonEvalJobs)
	comparisonIDFromEvalHub, erroData, err := client.CreateComparison(tenant, project,
		&types.User{UserName: gc.GetUserName(), UserID: gc.GetUserID()}, &client.EvalComparisonReqBody{AppType: req.Type, JobIDs: jobIDs})
	if err != nil {
		log.WithError(err).WithField("errorData", erroData).Error("failed to call evalHub service to create comparison")
		ctx.ErrorWithData(c, err, erroData)
		return
	}

	// 5 将实验对比写入数据库中
	evalComparison := evaluationMgr.CreateEvalComparison(gc, &req, comparisonIDFromEvalHub, comparisonEvalJobs)
	if err := dao.InsertEvalComparison(dao.NewDAO(m.App.MgoClient, tenant), evalComparison); err != nil {
		log.WithError(err).Error("failed to InsertEvalComparison")
		ctx.Error(c, err)
		return
	}

	// 6 组装响应信息
	resp := evaluationMgr.AssembleEvalComparisonSnapshotResp(comparisonIDFromEvalHub, revisionIDToAlgorithm, evalComparison)

	ctx.Success(c, &resp)
	auditlib.GinContextSendAudit(c, aisTypes.ResourceTypeEvaluationComparison, "", string(aisTypes.OperationTypeCreate))
}

// @Summary 创建评测对比任务
// @tags evaluation
// @Produce application/json
// @Description 创建评测对比任务
// @Param x-kb-project header string true "项目名"
// @Param request body types.EvalComparisonReq true "评测对比任务创建请求"
// @Success 200 {object} types.EvalComparisonSnapshotResp "创建成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /v1/comparisons [post]
func (m *Mgr) ExportComparison(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	project := gc.GetAuthProjectID()
	tenant := gc.GetAuthTenantID()

	evalComparisonID, err := primitive.ObjectIDFromHex(gc.Param("comparisonID"))
	if err != nil {
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	var req types.ExportComparisonSnapshotReq
	if err := gc.ShouldBindQuery(&req); err != nil {
		log.WithError(err).Error("failed to GetRequestJSONBody")
		ctx.Error(c, err)
		return
	}

	evalComparison, err := dao.FindEvalComparisonByID(dao.NewDAO(m.App.MgoClient, tenant), project, evalComparisonID)
	if err != nil {
		log.WithError(err).Error("failed to FindEvalComparisonByID")
		ctx.Error(c, err)
		return
	}

	comparisonWithSnapshot, err := evaluationMgr.ExportEvalComparison(gc, evalComparison.ComparisonID.Hex(), &req)
	if err != nil {
		log.WithError(err).Error("failed to CreateEvalComparisonSnapshot")
		ctx.Error(c, err)
		return
	}

	hour, min, sec := time.Now().Clock()
	var content []byte
	var filename string
	data := &evaluationMgr.EvalComparisonExportData{
		EvalComparison:         evalComparison,
		ComparisonWithSnapshot: comparisonWithSnapshot,
	}
	if req.EvalRequest.Type == "markdown" { // 导出 markdown
		content, err = evaluationMgr.GenerateMakrdown(m.App.MgoClient, data)
		filename = fmt.Sprintf("%d%d%d.md", hour, min, sec)
	} else if req.EvalRequest.Type == "json" {
		content, err = json.Marshal(data)
		filename = fmt.Sprintf("%d%d%d.json", hour, min, sec)
	} else {
		ctx.Success(c, data)
		return
	}
	if err != nil {
		log.WithError(err).Error("failed to generate json")
		ctx.Error(c, err)
		return
	}
	ctx.SuccessWithByte(c, filename, content)
}

// @Summary 保存评测对比任务快照
// @tags evaluation
// @Produce application/json
// @Description 保存评测对比任务快照
// @Param x-kb-project header string true "项目名"
// @Param request body types.EvalComparisonSnapshotReq true "评测对比任务快照创建请求"
// @Success 200  {string} {string} "创建成功(返回的为自动学习服务中的评测快照ID)"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /v1/comparison-snapshots [post]
func (m *Mgr) SaveComparisonSnapshot(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	project := gc.GetAuthProjectID()
	tenant := gc.GetAuthTenantID()

	var req types.EvalComparisonSnapshotReq
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to GetRequestJSONBody")
		ctx.Error(c, err)
		return
	}

	evalComparisonID, err := primitive.ObjectIDFromHex(req.EvalComparisonID)
	if err != nil {
		log.WithError(err).Error("failed to get evalComparisonID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}
	evalComparison, err := dao.FindEvalComparisonByID(dao.NewDAO(m.App.MgoClient, tenant), project, evalComparisonID)
	if err != nil {
		log.WithError(err).Error("failed to FindEvalComparisonByID")
		ctx.Error(c, err)
		return
	}

	evalComparisonSnapshot, err := evaluationMgr.CreateEvalComparisonSnapshot(gc, evalComparison.AppType, &req)
	if err != nil {
		log.WithError(err).Error("failed to CreateEvalComparisonSnapshot")
		ctx.Error(c, err)
		return
	}

	if err := dao.InsertEvalComparisonSnapshot(dao.NewDAO(m.App.MgoClient, tenant), evalComparisonSnapshot); err != nil {
		log.WithError(err).Error("failed to InsertEvalComparisonSnapshot")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	ctx.Success(c, evalComparisonSnapshot.ID.Hex())
}

// @Summary 删除对比任务快照
// @tags evaluation
// @Produce application/json
// @Description 删除评测对比任务快照
// @Param x-kb-project header string true "项目名"
// @Param comparisonSnapshotID path string true "comparisonSnapshotID ID(快照 ID)"
// @Success 204 "请求成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /v1/comparison-snapshots/{comparisonSnapshotID} [delete]
func (m *Mgr) DeleteComparisonSnapshot(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	comparisonSnapshotID, err := primitive.ObjectIDFromHex(gc.Param("comparisonSnapshotID"))
	if err != nil {
		log.Error("invalid param comparisonSnapshotID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	_, err = dao.GetEvalComparisonSnapshotByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), comparisonSnapshotID)
	if err != nil {
		log.WithError(err).Error("failed to get comparisonSnapshot")
		ctx.Error(c, errors.ErrorEvalComparisonSnapshotNotFound)
		return
	}

	if err := dao.DeleteComparisonSnapshotByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), comparisonSnapshotID); err != nil {
		log.WithError(err).Error("failed to delete comparisonSnapshot")
		ctx.Error(c, errors.ErrorEvalComparisonSnapshotNotFound)
		return
	}

	ctx.SuccessNoContent(c)
}

// @Summary 获取对比任务快照列表
// @tags evaluation
// @Produce application/json
// @Description 获取对比任务快照列表
// @Param x-kb-project header string true "项目名"
// @Param pageSize query integer false "分页大小" default(10)
// @Param page query integer false "页码" default(1)
// @Param sortBy query string false "指定排序关键字" Enums("updatedAt")
// @Param order query string false "指定排序方式(升序/降序)" Enums("asc", "desc")
// @Param onlyMe query bool false "只显示某个任务下存在的我创建版本的任务"
// @Success 200 {object} types.EvalComparisonSnapshotListResp "获取成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /v1/comparison-snapshots [get]
func (m *Mgr) ListComparisonSnapshot(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	// 1 获取快照基本信息
	count, evalComparisonSnapshots, err := dao.ListEvalComparisonSnapshot(gc, dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID())
	if err != nil {
		log.WithError(err).Error("failed to list comparisonSnapshot")
		ctx.Error(c, err)
		return
	}

	// 2 获取快照对应的评测对比基本信息
	ids := make([]primitive.ObjectID, 0, len(evalComparisonSnapshots))
	for i := range evalComparisonSnapshots {
		ids = append(ids, evalComparisonSnapshots[i].EvalComparisonID)
	}
	idToEvalComparison, err := dao.FindEvalComparisonByIDs(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), ids)
	if err != nil {
		log.WithError(err).Error("failed to FindEvalComparisonByIDs")
	}

	// 3 组装信息
	for _, snapshot := range evalComparisonSnapshots {
		if idToEvalComparison[snapshot.EvalComparisonID] == nil {
			log.Warnf("evalComparisonSnapshot without evalComparison: %+v", snapshot)
			continue
		}
		snapshot.AppType = idToEvalComparison[snapshot.EvalComparisonID].AppType
		snapshot.EvalJobs = idToEvalComparison[snapshot.EvalComparisonID].EvalJobs
	}

	ctx.Success(c, types.EvalComparisonSnapshotListResp{
		ComparisonSnapshots: evalComparisonSnapshots,
		Total:               count,
	})
}

// @Summary 获取单个对比任务快照
// @tags evaluation
// @Produce application/json
// @Description 删除评测对比任务快照
// @Param x-kb-project header string true "项目名"
// @Param comparisonSnapshotID path string true "comparisonSnapshotID ID(自动学习服务中的快照 ID)"
// @Success 200  {object} types.EvalComparisonSnapshotResp "获取成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /v1/comparison-snapshots/{comparisonSnapshotID} [get]
func (m *Mgr) GetComparisonSnapshot(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	comparisonSnapshotID, err := primitive.ObjectIDFromHex(gc.Param("comparisonSnapshotID"))
	if err != nil {
		log.WithError(err).Error("invalid param comparisonSnapshotID")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	// 1 获取评测对比相关信息
	evalComparisonSnapshot, err := dao.GetEvalComparisonSnapshotByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), comparisonSnapshotID)
	if err != nil {
		log.WithError(err).Error("failed to get comparisonSnapshot")
		ctx.Error(c, errors.ErrorEvalComparisonSnapshotNotFound)
		return
	}
	evalComparison, err := dao.FindEvalComparisonByID(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), gc.GetAuthProjectID(), evalComparisonSnapshot.EvalComparisonID)
	if err != nil {
		log.WithError(err).Error("failed to FindEvalComparisonByID")
		ctx.Error(c, errors.ErrorInternal)
		return
	}

	// 1.1 调用评测接口 校验评测任务是否被归档
	_, errorData, err := evaluationMgr.GetEvalComparison(gc, evalComparison.ComparisonID.Hex())
	if err != nil {
		if errorData != nil {
			errorResp := evaluationMgr.AssembleEvalComparisonSnapshotErrorResp(evalComparison, errorData)
			log.WithError(err).Errorf("failed to GetEvalComparison With erroData: %+v", errorResp)
			ctx.ErrorWithData(c, errors.ErrorEvalComparisonJobAlreadyArchived, errorResp)
			return
		}
		log.WithError(err).Error("failed to GetEvalComparison")
		ctx.Error(c, err)
		return
	}

	// 2 获取算法信息
	rvIDs := make([]string, 0, len(evalComparison.EvalJobs))
	for i := range evalComparison.EvalJobs {
		rvIDs = append(rvIDs, evalComparison.EvalJobs[i].RevisionID)
	}
	revisionIDToAlgorithm, err := dao.FindAlgorithmByRevisionIDs(dao.NewDAO(m.App.MgoClient, gc.GetAuthTenantID()), rvIDs, false)
	if err != nil {
		log.Error("failed to get FindAlgorithmByRevisionIDs")
		ctx.Error(c, err)
		return
	}

	// 3 组装信息
	res := evaluationMgr.AssembleEvalComparisonSnapshotResp(evalComparisonSnapshot.ComparisonID.Hex(), revisionIDToAlgorithm, evalComparison)

	ctx.Success(c, &res)
}
