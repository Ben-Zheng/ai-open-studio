package mgr

import (
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

// ListInferenceServiceVersions lists all the versions of given inference service
func (m *Mgr) ListInferenceServiceVersions(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	service, err := FindInferenceRevisions(m.getInferenceCollection(gc), gc.GetAuthProjectID(), serviceID, gc.GetPagination())
	if err != nil {
		log.WithError(err).Error("failed to delete FindInferenceRevisions")
		ctx.Error(c, err)
		return
	}

	m.fillServiceCommonInfo(gc, service)
	// add pods info of service
	m.fillServicePodsForLastRevision(service)
	// validate the service and instance status
	m.validateServiceStatus(service, true)

	ctx.Success(c, service)
}

// CreateInferenceServiceRevision updates configs of current inference service.
// A new version will be automatically created based on current version number.
// semantic version(v1,v2,...) is used.
func (m *Mgr) CreateInferenceServiceRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	defer auditlib.GinContextSetAuditData(c,
		map[string]any{
			auditlib.AuditRecordResource:     InferenceResource,
			auditlib.AuditRecordResourceName: serviceID,
		},
	)

	createRvReq := &types.CreateInferenceServiceRvReq{}
	if err := gc.GetRequestJSONBody(createRvReq); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	col := m.getInferenceCollection(gc)
	inferService, err := FindInferenceService(col, gc.GetAuthProjectID(), serviceID)
	if err != nil {
		log.WithError(err).Error("failed to FindInferenceService")
		ctx.Error(c, err)
		return
	}

	if err := m.checkServiceRvRequest(createRvReq, inferService.AppType); err != nil {
		log.WithError(err).Error("failed to checkServiceRvRequest")
		if _, ok := err.(inferenceErr.InferenceError); ok {
			ctx.Error(c, err)
			return
		}
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if err := m.checkInferenceModel(gc, createRvReq); err != nil {
		log.WithError(err).Error("failed to checkInferenceModel")
		if _, ok := err.(inferenceErr.InferenceError); ok {
			ctx.Error(c, err)
			return
		}
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	fillServiceRvRequest(inferService, createRvReq)
	inferService.Revisions = []*types.InferenceServiceRevision{getDefaultServiceRevision(gc, createRvReq, m.config)}
	mongoClient, _ := m.app.MgoClient.GetMongoClient(gc)
	if err := ResetAndAddInferenceRevision(gc, mongoClient, col, serviceID, inferService.Revisions[0]); err != nil {
		log.WithError(err).Error("failed to create new service revision")
		ctx.Error(c, err)
		return
	}

	// todo 锁需要提前
	go m.DeployKFService(ginlib.Copy(gc), col, inferService)

	ctx.Success(c, gin.H{"inferenceServiceID": inferService.ID})
}

// GetInferenceServiceVersion returns the specific version of given inference service
func (m *Mgr) GetInferenceServiceVersion(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	revision, err := getRequestParam(gc, InferenceServiceParamRevisionID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference revision from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	service, err := FindInferenceSpecificRevision(m.getInferenceCollection(gc), gc.GetAuthProjectID(), serviceID, revision)
	if err != nil {
		log.WithError(err).Error("failed to FindInferenceSpecificRevision")
		ctx.Error(c, err)
		return
	}

	m.fillServiceCommonInfo(gc, service)
	// add pods info of service
	m.fillServicePodsForLastRevision(service)
	// validate the service and instance status
	m.validateServiceStatus(service, false)

	ctx.Success(c, service)
}

func (m *Mgr) PatchInferenceServiceRevision(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	revisionID, err := getRequestParam(gc, InferenceServiceParamRevisionID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference revision name")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	updateInfo := &types.PatchIsvcRevisionReq{}
	if err := gc.GetRequestJSONBody(updateInfo); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	service, err := FindInferenceSpecificRevision(m.getInferenceCollection(gc), gc.GetAuthProjectID(), serviceID, revisionID)
	if err != nil {
		log.WithError(err).Error("failed to FindInferenceSpecificRevision")
		ctx.Error(c, err)
		return
	}
	m.validateServiceStatus(service, false)
	if service.Revisions[0].State != types.InferenceServiceStateNormal {
		log.Errorf("inference revision is not normal: %+v", service.Revisions[0])
		ctx.Error(c, inferenceErr.ErrInferenceNotNormal)
		return
	}

	if !CheckHyperparameterValid(updateInfo.Hyperparameter, service.Revisions[0]) {
		log.Errorf("invalid revision update info: %+v", updateInfo)
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if err := UpdateSpecificRevisionInfo(gc, m.getInferenceCollection(gc), serviceID, revisionID, map[string]interface{}{"hyperparameter": service.Revisions[0].Hyperparameter}); err != nil {
		log.WithError(err).Error("failed to UpdateSpecificRevisionInfo")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, service)
}

func (m *Mgr) InferenceRevisionAction(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	revisionName, err := getRequestParam(gc, InferenceServiceParamRevisionID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference revision name")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	actionInfo := &types.RevisionActionRequest{}
	if err := gc.GetRequestJSONBody(actionInfo); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if actionInfo.Action == types.ActionTypeRollout && actionInfo.RolloutRevision == "" {
		log.Error("rollout revision ID is empty")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if err := m.doInferenceRevisionAction(gc, serviceID, revisionName, actionInfo); err != nil {
		log.WithError(err).Error("failed to DoInferenceRevisionAction")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, gin.H{"inferenceServiceID": serviceID})
}

func (m *Mgr) doInferenceRevisionAction(gc *ginlib.GinContext, inferServiceID, revisionName string, actionReq *types.RevisionActionRequest) error {
	col := m.getInferenceCollection(gc)
	switch actionReq.Action {
	case types.ActionTypeShutdown:
		isvcWithSpecRv, err := FindInferenceSpecificRevision(col, gc.GetAuthProjectID(), inferServiceID, revisionName)
		if err != nil {
			log.Error("failed to FindInferenceSpecificRevision")
			return err
		}
		if ok := inferenceRevisionStageMachine(isvcWithSpecRv.Revisions[0].Stage, types.InferenceServiceRevisionStageShutdown); !ok {
			log.Errorf("invalid stage change: %s -> %s", isvcWithSpecRv.Revisions[0].Stage, types.InferenceServiceRevisionStageShutdown)
			return inferenceErr.ErrInvalidParam
		}
		// 更新状态
		if err := UpdateSpecificRevisionInfo(gc, col, inferServiceID, revisionName,
			map[string]interface{}{"stage": types.InferenceServiceRevisionStageShutdown, "ready": false}); err != nil {
			log.Error("failed to UpdateSpecificRevisionInfo")
			return err
		}
		// 删除isvc
		if err := DeleteKFService(newKserveClient(m.DynamicClient, gc.GetAuthProjectID()), isvcWithSpecRv); err != nil {
			log.WithError(err).Error("failed to remove deployed service")
		}
	case types.ActionTypeRestart:
		isvcWithLastRv, err := FindInferenceService(col, gc.GetAuthProjectID(), inferServiceID)
		if err != nil {
			log.Error("failed to FindInferenceService")
			return err
		}
		if isvcWithLastRv.Revisions[0].Revision != revisionName {
			log.Error("only the last revision support restart")
			return inferenceErr.ErrInvalidParam
		}

		if err := m.restartInferenceRevision(gc, col, isvcWithLastRv); err != nil {
			log.Error("failed to restartAndRolloutInferenceRevision")
			return err
		}
	case types.ActionTypeRollout:
		isvcWithRolloutRv, err := FindInferenceSpecificRevision(col, gc.GetAuthProjectID(), inferServiceID, actionReq.RolloutRevision)
		if err != nil {
			log.WithError(err).Error("failed to FindInferenceSpecificRevision")
			return err
		}

		if err := m.rolloutInferenceRevision(gc, col, isvcWithRolloutRv); err != nil {
			log.Error("failed to restartAndRolloutInferenceRevision")
			return err
		}
	}

	return nil
}

func inferenceRevisionStageMachine(current, target types.InferenceServiceRevisionStage) bool {
	switch current {
	case types.InferenceServiceRevisionStageDeploying:
		return target == types.InferenceServiceRevisionStageServing || target == types.InferenceServiceRevisionStageDeployFailed
	case types.InferenceServiceRevisionStageServing:
		return target == types.InferenceServiceRevisionStageShutdown
	case types.InferenceServiceRevisionStageShutdown:
		return target == types.InferenceServiceRevisionStageDeploying
	case types.InferenceServiceRevisionStageDeployFailed:
		return false
	}

	return false
}

func (m *Mgr) restartInferenceRevision(gc *ginlib.GinContext, col *mongo.Collection, inferService *types.InferenceService) error {
	if err := UpdateLastRevisionToRestart(gc, col, inferService.ID, inferService.Revisions[0].Revision); err != nil {
		log.Error("failed to UpdateLastRevisionToRestart")
		return err
	}

	go m.DeployKFService(ginlib.Copy(gc), col, inferService)

	return nil
}

func (m *Mgr) rolloutInferenceRevision(gc *ginlib.GinContext, col *mongo.Collection, inferService *types.InferenceService) error {
	inferInstance := inferService.Revisions[0].ServiceInstances[0]
	createRvReq := types.CreateInferenceServiceRvReq{ServiceInstances: []*types.CreateServiceInstanceReq{{
		From:              inferInstance.Meta.From,
		ImageURI:          inferInstance.Meta.ImageURI,
		ModelID:           inferInstance.Meta.ModelID,
		ModelRevision:     inferInstance.Meta.ModelRevision,
		ModelAppType:      aisTypes.AppType(inferInstance.Meta.ModelAppType),
		ModelFormatType:   inferInstance.Meta.ModelFormatType,
		ModelURI:          inferInstance.Meta.ModelURI,
		EntryPoint:        inferInstance.Meta.EntryPoint,
		Env:               inferInstance.Meta.Env,
		MinReplicas:       inferInstance.Spec.MinReplicas,
		MaxReplicas:       inferInstance.Spec.MaxReplicas,
		Resource:          &inferInstance.Spec.Resource,
		AutoShutdown:      inferInstance.Spec.AutoShutdown,
		KeepAliveDuration: inferInstance.Spec.KeepAliveDuration,
	}}}

	newRevision := getDefaultServiceRevision(gc, &createRvReq, m.config)
	newRevision.ParentRevision = inferService.Revisions[0].Revision
	inferService.Revisions = []*types.InferenceServiceRevision{newRevision}
	mongoClient, _ := m.app.MgoClient.GetMongoClient(gc)
	if err := ResetAndAddInferenceRevision(gc, mongoClient, col, inferService.ID, inferService.Revisions[0]); err != nil {
		log.WithError(err).Error("failed to create new service revision")
		return err
	}

	go m.DeployKFService(ginlib.Copy(gc), col, inferService)

	return nil
}

func CheckHyperparameterValid(h *types.Hyperparameter, isvcRevision *types.InferenceServiceRevision) bool {
	if h == nil {
		log.Error("hyperparameter is nil")
		return false
	}

	if h.PenaltyScore > 2.0 || h.PenaltyScore < -2.0 {
		log.Errorf("invalid penalty score: %f", h.PenaltyScore)
		return false
	}
	if h.TopP < 0 || h.TopP > 1.0 {
		log.Errorf("invalid top_p: %f", h.TopP)
		return false
	}
	if h.Temperature < 0 || h.Temperature > 1.0 {
		log.Errorf("invalid temperature: %f", h.Temperature)
		return false
	}
	h.SystemPersonality = strings.TrimSpace(h.SystemPersonality)

	if isvcRevision.Hyperparameter == nil {
		isvcRevision.Hyperparameter = &types.Hyperparameter{}
	}
	if h.SystemPersonality != "" {
		isvcRevision.Hyperparameter.SystemPersonality = h.SystemPersonality
	}
	isvcRevision.Hyperparameter.PenaltyScore = h.PenaltyScore
	isvcRevision.Hyperparameter.TopP = h.TopP
	isvcRevision.Hyperparameter.Temperature = h.Temperature

	return true
}
