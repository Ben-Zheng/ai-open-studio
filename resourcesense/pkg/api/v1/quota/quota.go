package quota

import (
	errors2 "errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/dataselect"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/resource"
	logic "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/logic/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx/errors"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

func ListEvents(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantName")
	selfUid := gc.GetUserID()
	if selfUid == "" {
		log.Error("get users failed")
		ctx.Error(c, errors2.New("get users failed"))
		return
	}
	err := quota.CheckAuth(gc, tenantID, selfUid)
	if err != nil {
		log.WithError(err).Error("failed to CheckAuth")
		ctx.Error(c, errors2.New("failed to CheckAuth"))
		return
	}

	start, err := strconv.ParseInt(c.Query("startTime"), 10, 64)
	if err != nil {
		log.WithError(err).Error("bad start param")
	}
	start /= 1000

	end, err := strconv.ParseInt(c.Query("endTime"), 10, 64)
	if err != nil {
		log.WithError(err).Error("bad end param")
	}
	end /= 1000
	total, rs, err := quota.ListResourceEvents(gc, &quota.ListResourceEventRequest{
		TenantID:            tenantID,
		CustomResourceTypes: gc.QueryArray("customResourceType"),
		Creator:             gc.Query("creator"),
		StartTime:           start,
		EndTime:             end,
	}, gc.GetPagination(), gc.GetSort())
	if err != nil {
		log.WithError(err).Error("failed to ListResourceEvents")
		ctx.Error(c, err)
		return
	}

	for i := range rs {
		if aisUtils.IsPhoneNum(rs[i].Creator) {
			rs[i].Creator = aisUtils.EncryptPhone(rs[i].Creator)
		}
	}

	ctx.Success(c, &quotaTypes.ListEventsResponse{
		Total: total,
		Items: rs,
	})
}

func GetClusterAPU(c *gin.Context) {
	nodes, err := consts.ResourceClient.NodeLister.List(consts.ResourceClient.NodeSelector)
	if err != nil {
		log.WithError(err).Error("get details, list nodes failed")
		ctx.Error(c, err)
		return
	}
	var apucs []*aisTypes.APUConfig
	for _, apu := range consts.ConfigMap.APUConfigs {
		if apu.Product != "" {
			apucs = append(apucs, apu)
			continue
		}

		labelMap := map[string]bool{}
		for _, node := range nodes {
			if _, ok := node.Status.Capacity[apu.ResourceName]; !ok {
				continue
			}
			if _, ok := node.Labels[apu.NodeLabel]; !ok {
				continue
			}
			if labelMap[node.Labels[apu.NodeLabel]] {
				continue
			}

			nc := *apu
			nc.Product = node.Labels[apu.NodeLabel]
			labelMap[node.Labels[apu.NodeLabel]] = true
			apucs = append(apucs, &nc)
		}
	}

	gpuGroup, _, err := quota.ListGPUGroup(ginlib.NewGinContext(c), &quota.ListGPUGroupParam{}, false)
	if err != nil {
		log.WithError(err).Error("failed to ListGPUGroup")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	var apuTypeMap = map[string]bool{}
	for _, apu := range gpuGroup {
		if apu.GPUTypeLevel == quotaTypes.GPUTypeLevelRare {
			apuTypeMap[apu.GPUType] = true
		}
	}
	for _, apu := range apucs {
		apu.IsAisNormal, apu.Title = true, fmt.Sprintf("%s.AisNormal", apu.Kind)
		if apuTypeMap[apu.Product] {
			apu.IsAisNormal, apu.Title = false, fmt.Sprintf("%s.%s", apu.Kind, apu.Product)
		}
	}

	ctx.Success(c, apucs)
}

func GetClusterQuota(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	for key, value := range gc.Request.Header {
		log.Printf("----------Header-------------- '%s': %s\n", key, value)
	}

	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	cq := &quotaTypes.ClusterQuota{}
	aq, err := quota.AggregateTenantQuota(gc, "") // 集群已分配配额、已占用配额、实际使用配额
	if err != nil {
		log.WithError(err).Error("failed to AggregateTenantQuota")
		ctx.Error(c, err)
		return
	}

	cq.TotalQuota, cq.Total = quota.GetClusterTotalQuota(gc) // 集群总配额

	if !consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		systemAdmin, err := quota.GetTenantDetail(gc, aisTypes.AISSystemAdminTenantName)
		if err != nil {
			log.WithError(err).Error("failed to GetTenantDetail")
			ctx.Error(c, err)
			return
		}
		cq.AssignedQuota = aq.TotalQuota.Add(systemAdmin.Used)  // 已分配配额（所有团队已分配配额 + 系统占用）
		cq.ChargedQuota = aq.ChargedQuota.Add(systemAdmin.Used) // 配额占用（所有团队配额占用 + 系统占用）
	} else {
		cq.AssignedQuota = aq.TotalQuota  // 已分配配额
		cq.ChargedQuota = aq.ChargedQuota // 配额占用（所有团队配额占用）
	}

	cq.Used = aq.Used           // 资源占用（团队资源占用 + 系统占用 ）
	cq.UsedQuota = aq.UsedQuota // 配额实际使用（包含调度出来的， 目前没有用到）
	cq.AvailableQuota = cq.TotalQuota.
		Minus(float64(consts.ConfigMap.QuotaConfig.InflationRate) / 100.0).
		Sub(cq.AssignedQuota).NoNegative() // 剩余可分配配额

	if err = logic.TransformClusterQuota(gc, cq); err != nil {
		log.WithError(err).Error("failed to transformData")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, cq)
}

func GetClusterGPUGroup(c *gin.Context) {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		log.Error("not support gpu group")
		ctx.Error(c, errors.ErrNotSupportGPUGroup)
		return
	}

	gc := ginlib.NewGinContext(c)

	gpuTypeLevel := gc.Query("gpuTypeLevel")
	if gpuTypeLevel != "" && !logic.ValidGPUTypeLevel(gpuTypeLevel) {
		log.Errorf("invalid GPUTypeLevel: %s", gpuTypeLevel)
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	gpuGroup, total, err := quota.ListGPUGroup(gc, &quota.ListGPUGroupParam{GPUTypeLevel: gpuTypeLevel}, false)
	if err != nil {
		log.WithError(err).Error("failed to ListGPUGroup")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	res, notExistGPUTypes, err := logic.AssembleGPUGroup(gc, gpuGroup, gpuTypeLevel, total)
	if err != nil {
		log.WithError(err).Error("failed to listGPUTypeReCheck")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	// 删除已经不存在的GPU类型
	if err = quota.DeleteGPUGroupByGPUType(gc, notExistGPUTypes); err != nil {
		log.WithError(err).Errorf("failed to DeleteGPUGroupByGPUType: %+v", notExistGPUTypes)
	}

	ctx.Success(c, &quotaTypes.ListGPUGroupResp{GPUTypeLevelGroup: res})
}

func UpdateClusterGPUGroup(c *gin.Context) {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		log.Error("not support gpu group")
		ctx.Error(c, errors.ErrNotSupportGPUGroup)
		return
	}

	gc := ginlib.NewGinContext(c)

	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	var req []*quotaTypes.GPUGroup
	if err := gc.ShouldBind(&req); err != nil {
		log.WithError(err).Error("failed to parse request")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := logic.UpdateGPUGroupPreCheck(gc, req); err != nil {
		log.WithError(err).Error("failed to checkGPUTypeExist")
		ctx.Error(c, err)
		return
	}

	if err := logic.UpdateGPUGroup(gc, req); err != nil {
		log.WithError(err).Error("failed to RemoveChargedQuotRareGPUType")
		ctx.Error(c, err)
		return
	}

	ctx.SuccessNoContent(c)
}

func ListTenantQuotas(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	total, rs, err := quota.ListTenantDetails(gc, gc.Query("tenantName"), gc.GetPagination(), gc.GetSort(), false)
	if err != nil {
		log.WithError(err).Error("failed to ListTenantDetails")
		ctx.Error(c, err)
		return
	}

	for i := range rs {
		if rs[i].TenantType != aisTypes.AISSystemAdminTenantName {
			continue
		}
		rs[i].TenantName = aisTypes.AISSystemAdminTenantDisplayName
		rs[i].ChargedQuota = rs[i].Used
		rs[i].TotalQuota = rs[i].Used
		rs[i].UsedQuota = rs[i].Used
		rs[i].ToData()
	}

	if err = logic.TransformTenantQuota(gc, rs); err != nil {
		log.WithError(err).Error("failed to TransformTenantQuota")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, &quotaTypes.ListTenantDetailsResponse{
		Total: total,
		Items: rs,
	})
}

func UpdateTenantQuota(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	tenantID := gc.Param("tenantName")
	var reqData quotaTypes.QuotaData
	if err := gc.BindJSON(&reqData); err != nil {
		log.WithError(err).Error("failed to BindJSON")
		ctx.Error(c, err)
		return
	}

	req := quotaTypes.BuildQuotaFromData(reqData)
	if err := logic.UpdateTenantQuotaPreCheck(gc, req); err != nil {
		log.WithError(err).Errorf("failed to UpdateTenantQuotaPreCheck")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	aq, err := quota.AggregateTenantQuota(gc, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to AggregateTenantQuota")
		ctx.Error(c, errors.ErrInternal)
		return
	}
	if err = logic.TransformTenantQuota(gc, []*quotaTypes.TenantDetail{aq}); err != nil {
		log.WithError(err).Error("failed to TransformTenantQuota")
		ctx.Error(c, errors.ErrInternal)
		return
	}
	totalQuota, _ := quota.GetClusterTotalQuota(gc)
	if err = logic.TransformQuota(gc, totalQuota); err != nil {
		log.WithError(err).Error("failed to TransformQuota")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	availableQuota := totalQuota.Minus(float64(consts.ConfigMap.QuotaConfig.InflationRate) / 100.0).Sub(aq.TotalQuota).NoNegative()

	// 检查申请配额小于等于可用配额
	log.Debugf("assign tenant quota, tenant: %s, available: %+v, request: %+v", tenantID, availableQuota, req)
	if !req.LessThan(availableQuota) {
		ctx.Error(c, errors.ErrInsufficientAvailableQuota)
		return
	}

	if err = quota.UpdateTenantQuota(gc, tenantID, req); err != nil {
		log.WithError(err).Error("failed to CreateOrUpdateTenantQuota")
		ctx.Error(c, err)
		return
	}
	// 返回成功响应
	ctx.SuccessNoContent(c)
}

func UpdateTenantChargedQuota(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var req quotaTypes.TenantChargedQuotaUpdateReq
	if err := gc.ShouldBind(&req); err != nil {
		log.WithError(err).Error("failed to parse request")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if !logic.CheckAndCalculateChargedQuotaReq(&req) {
		log.Error("failed to CheckAndCalculateChargedQuotaReq")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := logic.UpdateTenantChargedQuota(gc, req); err != nil {
		log.WithError(err).Errorf("failed to UpdateTenantChargedQuota")
		ctx.Error(c, err)
		return
	}

	ctx.SuccessNoContent(c)
}

func GetTenantDetail(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	if !strings.Contains(gc.Request.Host, "admin") {
		selfUid := gc.GetUserID()
		if selfUid == "" {
			log.Error("get users failed")
			ctx.Error(c, errors2.New("get users failed"))
			return
		}
		err := quota.CheckAuth(gc, gc.Param("tenantName"), selfUid)
		if err != nil {
			log.WithError(err).Error("failed to CheckAuth")
			ctx.Error(c, errors2.New("failed to CheckAuth"))
			return
		}
	} else {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}
	
	tenantDetail, err := quota.GetTenantDetail(gc, gc.Param("tenantName"))
	if err != nil {
		log.WithError(err).Error("failed to GetTenantDetail")
		ctx.Error(c, err)
		return
	}

	if err = logic.TransformTenantQuota(gc, []*quotaTypes.TenantDetail{tenantDetail}); err != nil {
		log.WithError(err).Error("failed to TransformTenantQuota")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, tenantDetail)
}

// GetCurrentTenantDetail 用户端查看配额情况
func GetCurrentTenantDetail(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantDetail, err := quota.GetTenantDetail(gc, gc.GetAuthTenantID())
	if err != nil {
		log.WithError(err).Error("failed to GetTenantDetail")
		ctx.Error(c, err)
		return
	}

	tenantDetail.MemberCount = 0
	if err = logic.TransformTenantQuota(gc, []*quotaTypes.TenantDetail{tenantDetail}); err != nil {
		log.WithError(err).Error("failed to TransformTenantQuota")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, tenantDetail)
}

func GetCurrentTenantWorkloads(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	tenantID := gc.GetAuthTenantID()
	workloadType := gc.Query("workloadType")
	query, err := dataselect.ParseFilterQuery(gc)
	if err != nil {
		log.WithError(err).Error("failed to parse filter query")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}
	selfUid := gc.GetUserID()
	if selfUid == "" {
		log.Error("get users failed")
		ctx.Error(c, errors2.New("get users failed"))
		return
	}
	err = quota.CheckAuth(gc, tenantID, selfUid)
	if err != nil {
		log.WithError(err).Error("failed to CheckAuth")
		ctx.Error(c, errors2.New("failed to CheckAuth"))
		return
	}

	var workloads []*quotaTypes.Workload
	var total int64
	switch workloadType {
	case string(quotaTypes.ResourcePodResource):
		total, workloads, err = consts.WorkloadLister.ListPodWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	case string(quotaTypes.ResourcePVCResource):
		total, workloads, err = consts.WorkloadLister.ListPVCWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	case string(quotaTypes.ResourceDatasetStorage):
		total, workloads, err = consts.WorkloadLister.ListDatasetWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	default:
		total, workloads, err = consts.WorkloadLister.ListAllWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	}
	if err != nil {
		log.WithError(err).Error("failed to list workloads")
		ctx.Error(c, err)
		return
	}

	for i := range workloads {
		if err = logic.TransformQuota(gc, workloads[i].ChargedQuota); err != nil {
			log.WithError(err).Error("failed to chargedQuota")
			ctx.Error(c, errors.ErrInternal)
			return
		}
		workloads[i].DChargedQuota = workloads[i].ChargedQuota.ToData()
	}

	ctx.Success(c, &quotaTypes.ListTenantWorkloadsResponse{
		Total: total,
		Items: workloads,
	})
}

type ListQuotaAuditsResponse struct {
	Items []*quotaTypes.TenantQuotaAudit `json:"items"`
	Total int64                          `json:"total"`
}

func ListTenantQuotaAudits(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	total, rs, err := quota.ListTenantQuotaAudits(gc, gc.Param("tenantName"), gc.Query("creator"), gc.GetPagination())
	if err != nil {
		log.WithError(err).Error("failed to ListTenantQuotaAudits")
		ctx.Error(c, err)
		return
	}

	if err = logic.TransformTenantQuotaAudit(rs); err != nil {
		log.WithError(err).Error("failed to TransformTenantQuotaAudit")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, &ListQuotaAuditsResponse{
		Total: total,
		Items: rs,
	})
}

func ListTenantUsers(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantName := gc.Param("tenantName")
	roleName := string(authTypes.RoleTenantOwner)
	domainType := gc.GetHeader(authTypes.AISDomainTypeHeader)

	if domainType != "admin" {
		//tenantName = gc.GetAuthTenantID()
		roleName = getUserRole(gc.GetUserID(), gc.GetAuthTenantID())
	}

	selfUid := gc.GetUserID()
	if selfUid == "" {
		log.Error("get users failed")
		ctx.Error(c, errors2.New("get users failed"))
		return
	}
	err := quota.CheckAuth(gc, tenantName, selfUid)
	if err != nil {
		log.WithError(err).Error("failed to CheckAuth")
		ctx.Error(c, errors2.New("failed to CheckAuth"))
		return
	}

	total, rs, err := quota.ListUserDetails(gc, tenantName, gc.Query("creator"), gc.GetPagination(), gc.GetSort())
	if err != nil {
		log.WithError(err).Error("failed to ListUserDetails")
		ctx.Error(c, err)
		return
	}

	if err = logic.TransformUserDetail(gc, rs, roleName); err != nil {
		log.WithError(err).Error("failed to TransformTenantQuotaAudit")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, &quotaTypes.ListTenantUsersResponse{
		Total: total,
		Items: rs,
	})
}

func getUserRole(userID, tenantID string) string {
	role := string(authTypes.RoleTenantViewer)
	us, err := resource.GetUserByID(userID)
	if err != nil {
		log.WithError(err).Error("failed to GetUserByID")
		return role
	}

	if us.Ban.Ban {
		return role
	}

	for _, tenant := range us.Tenants {
		if tenant.ID == tenantID && tenant.Role != nil {
			return tenant.Role.Name
		}
	}

	return role
}

func ListWorkloads(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantName")
	workloadType := gc.Query("workloadType")
	query, err := dataselect.ParseFilterQuery(gc)
	if err != nil {
		log.WithError(err).Error("failed to parse filter query")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	selfUid := gc.GetUserID()
	if selfUid == "" {
		log.Error("get users failed")
		ctx.Error(c, errors2.New("get users failed"))
		return
	}
	err = quota.CheckAuth(gc, tenantID, selfUid)
	if err != nil {
		log.WithError(err).Error("failed to CheckAuth")
		return
	}

	var workloads []*quotaTypes.Workload
	var total int64
	switch workloadType {
	case string(quotaTypes.ResourcePodResource):
		total, workloads, err = consts.WorkloadLister.ListPodWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	case string(quotaTypes.ResourcePVCResource):
		total, workloads, err = consts.WorkloadLister.ListPVCWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	case string(quotaTypes.ResourceDatasetStorage):
		total, workloads, err = consts.WorkloadLister.ListDatasetWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	default:
		total, workloads, err = consts.WorkloadLister.ListAllWorkloads(tenantID, query, gc.GetPagination(), gc.GetSort())
	}
	if err != nil {
		log.WithError(err).Error("failed to list workloads")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	for i := range workloads {
		if err = logic.TransformQuota(gc, workloads[i].ChargedQuota); err != nil {
			log.WithError(err).Error("failed to chargedQuota")
			ctx.Error(c, errors.ErrInternal)
			return
		}
		workloads[i].DChargedQuota = workloads[i].ChargedQuota.ToData()
	}

	ctx.Success(c, &quotaTypes.ListTenantWorkloadsResponse{
		Total: total,
		Items: workloads,
	})
}

func ReleaseUserResources(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantName")
	userID := gc.Param("userID")
	if tenantID == "" || userID == "" {
		log.Error("failed to parse tenantName or userName")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	user, err := quota.GetUserDetailByID(gc, tenantID, userID)
	if err != nil {
		log.WithError(err).Error("failed to GetTenantDetail")
		ctx.Error(c, err)
		return
	}

	go release(tenantID, user.UserID, user.UserName)
	ctx.SuccessNoContent(c)
}

func release(tenantID, userID, userName string) {
	req := &ginlib.ReleaseByUserRequest{
		TenantID: tenantID,
		UserID:   userID,
		UserName: userName,
	}
	logger := log.WithFields(log.Fields{"req": req})
	if err := consts.ReleaseClient.AlgoCabinsReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放算法仓资源失败")
	}
	if err := consts.ReleaseClient.AlgoCardsReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放算法卡片资源失败")
	}
	if err := consts.ReleaseClient.AutoLearnReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放自动学习资源失败")
	}
	if err := consts.ReleaseClient.CodebaseReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放算法管理资源失败")
	}
	if err := consts.ReleaseClient.DataCleanReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放数据处理任务资源失败")
	}
	if err := consts.ReleaseClient.EvaluationReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放评测资源失败")
	}
	if err := consts.ReleaseClient.WorkspaceReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放工作空间资源失败")
	}
	if err := consts.ReleaseClient.AILabelReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放智能标注资源失败")
	}
	if err := consts.ReleaseClient.ModelReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放模型资源失败")
	}
	if err := consts.ReleaseClient.DatasetReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放数据集资源失败")
	}
	if err := consts.ReleaseClient.PairReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放训验对资源失败")
	}
	if err := consts.ReleaseClient.InferenceReleaseByUser(req); err != nil {
		logger.WithError(err).Error("释放推理资源失败")
	}
	logger.Info("释放所有资源成功!")
}
