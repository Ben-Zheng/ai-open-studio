package mgr

import (
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
)

const (
	InstanceIDService                     string = "0"
	InstanceIDName                        string = "AIS_INSTANCE_ID"
	InferenceResponseHeaderInstanceIDName string = "x-env-AIS-INSTANCE-ID"

	MaxInferenceNameLength = 64
	InferenceResource      = "inference"
	DefaultResourceCPU     = 1
	DefaultResourceMemory  = 4
	DefaultResourceGPU     = 1

	ConvertResourceCPU    = 4
	ConvertResourceMemory = 30
	ConvertResourceGPU    = 1
)

// ListInferenceServices lists inference service with query options
func (m *Mgr) ListInferenceServices(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	query, err := buildListQuery(gc)
	if err != nil {
		log.WithError(err).Error("failed to buildListQuery")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	// todo 检查query
	col := m.getInferenceCollection(gc)
	res, err := FindInferenceServices(gc, col, query)
	if err != nil {
		log.WithError(err).Error("failed to FindInferenceServices")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}

	for _, service := range res.Data {
		m.fillServiceCommonInfo(gc, service)
		// add pods info of service
		m.fillServicePodsForLastRevision(service)
		// validate the service and instance status
		m.validateServiceStatus(service, false)
	}

	ctx.Success(c, res)
}

// GetInferenceService returns the detailed inference service info(latest version)
func (m *Mgr) GetInferenceService(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	var serviceID string
	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	col := m.getInferenceCollection(gc)
	res, err := FindInferenceService(col, gc.GetAuthProjectID(), serviceID)
	if err != nil {
		log.WithError(err).WithField("ServiceID", serviceID).Error("failed to query inference service")
		ctx.Error(c, err)
		return
	}

	m.fillServiceCommonInfo(gc, res)
	// add pods info of service
	m.fillServicePodsForLastRevision(res)
	// validate the service and instance status
	m.validateServiceStatus(res, false)

	ctx.Success(c, res)
}

// CreateInferenceService creates new inference service
func (m *Mgr) CreateInferenceService(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	createReq := &types.CreateInferenceServiceReq{}
	if err := gc.GetRequestJSONBody(createReq); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	if err := m.checkServiceRequest(createReq); err != nil {
		log.Error("create inference service request is invalid")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if err := m.checkInferenceModel(gc, createReq.Revision); err != nil {
		log.WithError(err).Error("failed to checkInferenceModel")
		ctx.Error(c, err)
		return
	}

	// fill default inference service from request
	inferService := getDefaultInferenceService(gc, createReq, m.config)
	defer auditlib.GinContextSetAuditData(c,
		map[string]any{
			auditlib.AuditRecordResource:     InferenceResource,
			auditlib.AuditRecordResourceName: inferService.ID,
		},
	)

	// write into DB
	col := m.getInferenceCollection(gc)
	if err := CreateInferenceService(col, inferService); err != nil {
		e := inferenceErr.ErrInternal
		if IsDupErr(err) {
			e = inferenceErr.ErrDatabaseDuplicatedKey
		}
		log.WithError(err).Error("failed to create new service")
		ctx.Error(c, e)
		return
	}

	// create kserve inference service
	go m.DeployKFService(ginlib.Copy(gc), col, inferService)

	ctx.Success(c, gin.H{"inferenceServiceID": inferService.ID})
}

// UpdateInferenceService updates basic information of current inference service
func (m *Mgr) UpdateInferenceService(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	updateInfo := &types.PatchInferenceServiceReq{}
	if err := gc.GetRequestJSONBody(updateInfo); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if err := UpdateInferenceServiceInfo(gc, m.getInferenceCollection(gc), serviceID, updateInfo); err != nil {
		log.WithError(err).Error("failed to update inference service info")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, gin.H{"inferenceServiceID": serviceID})
}

// DeleteInferenceService removes inference service by id(not recoverable)
func (m *Mgr) DeleteInferenceService(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	defer auditlib.GinContextSetAuditData(c,
		map[string]any{
			auditlib.AuditRecordResource:     InferenceResource,
			auditlib.AuditRecordResourceName: serviceID,
		},
	)

	col := m.getInferenceCollection(gc)
	service, err := FindInferenceService(col, gc.GetAuthProjectID(), serviceID)
	if err != nil {
		log.WithError(err).WithField("ServiceID", serviceID).Error("failed to query inference service")
		ctx.Error(c, err)
		return
	}

	// delete inference service info in db
	if err := DeleteInferenceService(gc, col, serviceID); err != nil {
		log.WithError(err).Error("failed to delete service")
		ctx.Error(c, err)
		return
	}

	// remove pods info
	m.podsMap.Delete(genServiceName(GetNamespace(service.ProjectID), service.Name, KserveWebhookLabelValuePredictor))

	// remove deployed service
	if err := DeleteKFService(newKserveClient(m.DynamicClient, gc.GetAuthProjectID()), service); err != nil {
		log.WithError(err).Error("failed to remove deployed service")
	}

	ctx.SuccessNoContent(c)
}

func (m *Mgr) UpdateInferenceServiceReason(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	updateInfo := &types.UpdateInferenceServiceReason{}
	if err := gc.GetRequestJSONBody(updateInfo); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if err := UpdateInferenceServiceReason(m.getInferenceCollection(gc), serviceID, gc.GetAuthProjectID(), updateInfo.Reason); err != nil {
		log.WithError(err).Error("failed to update inference service info")
		ctx.Error(c, err)
		return
	}

	ctx.SuccessNoContent(c)
}

// ListInferenceServiceAPILogs lists inference service with query options
func (m *Mgr) ListInferenceServiceAPILogs(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	query, err := buildListQuery(gc)
	if err != nil {
		log.WithError(err).Error("failed to buildListQuery")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	res, err := FindInferenceServiceAPILogs(m.getInferenceLogCollection(gc), query, serviceID)
	if err != nil {
		log.WithError(err).Error("failed to ListInferenceServiceAPILogs")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}

	ctx.Success(c, res)
}

func (m *Mgr) InferenceDefaultResource(c *gin.Context) {
	ctx.Success(c, getDefaultResources(m))
}

func (m *Mgr) ReleaseByUser(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	req := gc.GetReleaseByUserRequest()
	if req == nil {
		log.Error("failed to GetReleaseByUserRequest")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	fake := ginlib.NewMockGinContext()
	fake.SetTenantName(req.TenantID)

	col := m.getInferenceCollection(fake)
	services, err := ListInferenceServiceByUser(col, req.UserName)
	if err != nil {
		log.WithError(err).Error("failed to ListInferenceServiceByUser")
		ctx.Error(c, err)
		return
	}

	for _, svc := range services {
		fake.SetAuthProjectID(svc.ProjectID)
		// delete inference service info in db
		if err := DeleteInferenceService(fake, col, svc.ID); err != nil {
			log.WithError(err).Error("failed to delete service")
			ctx.Error(c, err)
			return
		}

		// remove pods info
		m.podsMap.Delete(genServiceName(GetNamespace(svc.ProjectID), svc.Name, KserveWebhookLabelValuePredictor))

		// remove deployed service
		if err := DeleteKFService(newKserveClient(m.DynamicClient, svc.ProjectID), svc); err != nil {
			log.WithError(err).Error("failed to remove deployed service")
		}
	}
	ctx.SuccessNoContent(c)
}

func (m *Mgr) Healthz(c *gin.Context) {
	ginlib.NewGinContext(c).ResponseOK()
}
