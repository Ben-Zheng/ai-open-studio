package quota

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aistypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/resource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx/errors"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

func ValidGPUTypeLevel(gpuTypeLevel string) bool {
	return gpuTypeLevel == quotaTypes.GPUTypeLevelRare || gpuTypeLevel == quotaTypes.GPUTypeLevelNormal
}

func UpdateGPUGroupPreCheck(gc *ginlib.GinContext, req []*quotaTypes.GPUGroup) error {
	gpuTypes, gpuTypeToNum, err := resource.AggregateClusterAPUTypes(gc)
	if err != nil {
		return err
	}
	gpuTypeMap := make(map[string]int8, len(gpuTypes))
	for i := range gpuTypes {
		gpuTypeMap[gpuTypes[i]] = 0
	}

	log.Infof("UpdateGPUGroupPreCheck gpu map: %+v", gpuTypeToNum)
	var toBeRareGPUNum int64
	for i := range req {
		// 检查GPU类型是否存在
		if _, exist := gpuTypeMap[req[i].GPUType]; !exist {
			log.Errorf("GPU type not exist: %s", req[i].GPUType)
			return errors.ErrParamsInvalid
		}
		gpuTypeMap[req[i].GPUType]++

		// 检查GPU类型等级是否正确
		if !ValidGPUTypeLevel(req[i].GPUTypeLevel) {
			log.Errorf("GPU type level is invalid: %s", req[i].GPUTypeLevel)
			return errors.ErrParamsInvalid
		}

		// 检查GPU型号是否只出现一次，不允许同一型号同时提交多条修改
		if gpuTypeMap[req[i].GPUType] > 1 {
			log.Errorf("the same GPU type can only appear once: %s", req[i].GPUType)
			return errors.ErrParamsInvalid
		}

		if req[i].GPUTypeLevel == quotaTypes.GPUTypeLevelRare {
			toBeRareGPUNum += int64(gpuTypeToNum[req[i].GPUType])
		}
	}

	aq, err := quota.AggregateTenantQuota(gc, "") // 集群已分配配额、已占用配额、实际使用配额
	if err != nil {
		log.WithError(err).Error("failed to AggregateTenantQuota")
		return errors.ErrInternal
	}
	cq := &quotaTypes.ClusterQuota{}
	cq.TotalQuota, _ = quota.GetClusterTotalQuota(gc) // 集群总配额
	if !consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		systemAdmin, err := quota.GetTenantDetail(gc, aistypes.AISSystemAdminTenantName)
		if err != nil {
			log.WithError(err).Error("failed to GetTenantDetail")
			return errors.ErrInternal
		}
		cq.AssignedQuota = aq.TotalQuota.Add(systemAdmin.Used) // 已分配配额（所有团队已分配配额 + 系统占用）
	} else {
		cq.AssignedQuota = aq.TotalQuota // 已分配配额
	}

	log.Infof("normal gpu is insufficient: cluster quota: %+v, toBErareGPU: %d", cq, toBeRareGPUNum)
	totalGPU := cq.TotalQuota[aistypes.NVIDIAGPUResourceName]
	assignedNormalGPU := cq.AssignedQuota[types.ResourceNameWithGPUType(types.AisNormalGPUType)]
	if totalGPU.Value()-toBeRareGPUNum < assignedNormalGPU.Value() {
		log.Errorf("normal gpu is insufficient: totalGPU: %d, toBErareGPU: %d", totalGPU.Value(), assignedNormalGPU.Value())
		return errors.ErrReduceChargedNormalGPUQuota
	}

	return nil
}

func AssembleGPUGroup(gc *ginlib.GinContext, gpuGroups []*quotaTypes.GPUGroup, gpuTypeLevel string, total int64) ([]string, []string, error) {
	gpuTypes, _, err := resource.AggregateClusterAPUTypes(gc)
	if err != nil {
		log.WithError(err).Error("failed to AggregateClusterAPUTypes")
		return nil, nil, err
	}

	// 如果当前gpuGroup表为空，则默认全部为普通类型
	if total == 0 && (gpuTypeLevel == quotaTypes.GPUTypeLevelNormal || gpuTypeLevel == "") {
		return gpuTypes, nil, nil
	}

	// 检查现有集群中是否还存在该型号GPU
	gpuTypeMap := make(map[string]bool)
	for i := range gpuTypes {
		gpuTypeMap[gpuTypes[i]] = true
	}

	var res []string
	var gpuTypeNotExist []string
	for i := range gpuGroups {
		if gpuTypeMap[gpuGroups[i].GPUType] {
			res = append(res, gpuGroups[i].GPUType)
		} else {
			gpuTypeNotExist = append(gpuTypeNotExist, gpuGroups[i].GPUType)
		}
	}

	return res, gpuTypeNotExist, nil
}

func TransformClusterQuota(gc *ginlib.GinContext, clusterQuota *quotaTypes.ClusterQuota) error {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		clusterQuota.ToData()
		return nil
	}

	gpuTypeLevelRare, err := quota.GetRareGPUTypeMap(gc)
	if err != nil {
		return err
	}

	quota.ReCalculateNormalGPU(clusterQuota.AssignedQuota, gpuTypeLevelRare, true)
	quota.ReCalculateNormalGPU(clusterQuota.AvailableQuota, gpuTypeLevelRare, true)
	quota.ReCalculateNormalGPU(clusterQuota.ChargedQuota, gpuTypeLevelRare, true)
	quota.ReCalculateNormalGPU(clusterQuota.TotalQuota, gpuTypeLevelRare, true)
	quota.ReCalculateNormalGPU(clusterQuota.UsedQuota, gpuTypeLevelRare, true)
	quota.ReCalculateNormalGPU(clusterQuota.Used, gpuTypeLevelRare, true)
	quota.ReCalculateNormalGPU(clusterQuota.Total, gpuTypeLevelRare, true)

	clusterQuota.ToData()
	return nil
}

func TransformTenantQuota(gc *ginlib.GinContext, tenantDetails []*quotaTypes.TenantDetail) error {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		return nil
	}

	gpuTypeLevelRare, err := quota.GetRareGPUTypeMap(gc)
	if err != nil {
		return err
	}

	for i := range tenantDetails {
		// 重新计算GPU数量
		quota.ReCalculateNormalGPU(tenantDetails[i].TotalQuota, gpuTypeLevelRare, true)
		quota.ReCalculateNormalGPU(tenantDetails[i].ChargedQuota, gpuTypeLevelRare, true)
		quota.ReCalculateNormalGPU(tenantDetails[i].Used, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(tenantDetails[i].UsedQuota, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(tenantDetails[i].RecentAccumulate, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(tenantDetails[i].Accumulate, gpuTypeLevelRare, false)
		tenantDetails[i].ToData()

		// 返回tenant已分配的稀有GPU类型
		getTenantAllocatedRareGPUType(tenantDetails[i])
	}

	return nil
}

func getTenantAllocatedRareGPUType(tenantDetail *quotaTypes.TenantDetail) {
	tempMap := make(map[string]bool)
	for k, v := range tenantDetail.TotalQuota {
		_, gpuType := getAPUType(k)
		if gpuType == "" || gpuType == types.AisNormalGPUType || v.Value() == 0 {
			continue
		}
		tempMap[gpuType] = true
	}

	for k := range tempMap {
		tenantDetail.AllocatedRareGPUType = append(tenantDetail.AllocatedRareGPUType, k)
	}
}

func TransformQuota(gc *ginlib.GinContext, q quotaTypes.Quota) error {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		return nil
	}

	gpuTypeLevelRare, err := quota.GetRareGPUTypeMap(gc)
	if err != nil {
		return err
	}

	quota.ReCalculateNormalGPU(q, gpuTypeLevelRare, false)

	return nil
}

func TransformTenantQuotaAudit(qas []*quotaTypes.TenantQuotaAudit) error {
	for i := range qas {
		if utils.IsPhoneNum(qas[i].Creator) {
			qas[i].Creator = utils.EncryptPhone(qas[i].Creator)
		}
	}

	for i := range qas {
		qas[i].ToData()
	}

	return nil
}

func TransformUserDetail(gc *ginlib.GinContext, userDetails []*quotaTypes.UserDetail, roleName string) error {
	for i := range userDetails {
		if utils.IsPhoneNum(userDetails[i].UserName) {
			userDetails[i].UserName = utils.EncryptPhone(userDetails[i].UserName)
		}
		userDetails[i].RoleName = roleName
	}

	if !features.IsQuotaSupportGPUGroupEnabled() {
		return nil
	}

	gpuTypeLevelRare, err := quota.GetRareGPUTypeMap(gc)
	if err != nil {
		return err
	}

	for i := range userDetails {
		quota.ReCalculateNormalGPU(userDetails[i].ChargedQuota, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(userDetails[i].UsedQuota, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(userDetails[i].Used, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(userDetails[i].Accumulate, gpuTypeLevelRare, false)
		quota.ReCalculateNormalGPU(userDetails[i].RecentAccumulate, gpuTypeLevelRare, false)
		userDetails[i].ToData()
	}

	return nil
}

func UpdateTenantQuotaPreCheck(gc *ginlib.GinContext, req quotaTypes.Quota) error {
	if !CheckResourceNameValid(req) {
		return fmt.Errorf("invalied resource name")
	}

	if !features.IsQuotaSupportGPUGroupEnabled() {
		for k := range req {
			resourceName, _ := getAPUType(k)
			if resourceName != "" {
				return fmt.Errorf("not support GPU type: %s", k)
			}
		}
		return nil
	}

	gpuTypeLevelRare, err := quota.GetRareGPUTypeMap(gc)
	if err != nil {
		return err
	}
	// 该变量为True时支持设置GPU类型
	for k := range req {
		resourceName, gpuType := getAPUType(k)
		switch resourceName {
		case aistypes.NVIDIAGPUResourceName, aistypes.NVIDIAVirtGPUResourceName, aistypes.HUAWEINPUResourceName:
			if gpuType == types.AisNormalGPUType {
				continue
			}
			if !gpuTypeLevelRare[gpuType] {
				return fmt.Errorf("not rare GPU type: %s", gpuType)
			}
		}
	}
	// 该变量为True时，nvidia.com/gpu = nvidia.com/gpu.<rare type> + nvidia.com/gpu.AisNormal
	if req.GPU() == 0 {
		req[aistypes.NVIDIAGPUResourceName] = k8sresource.MustParse(strconv.FormatInt(req.SumGPUType(), 10))
	}
	if req.VirtGPU() == 0 {
		req[aistypes.NVIDIAVirtGPUResourceName] = k8sresource.MustParse(strconv.FormatInt(req.SumVirtGPUType(), 10))
	}
	if req.NPU() == 0 {
		req[aistypes.HUAWEINPUResourceName] = k8sresource.MustParse(strconv.FormatInt(req.SumNPUType(), 10))
	}
	if req.SumGPUType() != req.GPU() {
		return fmt.Errorf("gpu quantity is not consistent, total gpu: %d", req.GPU())
	}
	if req.SumVirtGPUType() != req.VirtGPU() {
		return fmt.Errorf("virt gpu quantity is not consistent, total virt gpu: %d", req.VirtGPU())
	}
	if req.SumNPUType() != req.NPU() {
		return fmt.Errorf("virt gpu quantity is not consistent, total virt gpu: %d", req.VirtGPU())
	}

	return nil
}

func getAPUType(resourceName corev1.ResourceName) (string, string) {
	gpuType := types.GPUTypeFromResourceName(resourceName)
	if gpuType != "" {
		return aistypes.NVIDIAGPUResourceName, gpuType
	}

	gpuType = types.VirtGPUTypeFromResourceName(resourceName)
	if gpuType != "" {
		return aistypes.NVIDIAVirtGPUResourceName, gpuType
	}

	npuType := types.NPUTypeFromResourceName(resourceName)
	if npuType != "" {
		return aistypes.HUAWEINPUResourceName, npuType
	}

	return "", ""
}

func UpdateGPUGroup(gc *ginlib.GinContext, req []*quotaTypes.GPUGroup) error {
	updateTenantDetails, tenantAudits, err := reduceRareGPUInChargedQuota(gc, req)
	if err != nil {
		log.Error("failed to reduceRareGPUInChargedQuota")
		return err
	}
	log.Debugf("UpdateGPUGroup: %d, %d", len(updateTenantDetails), len(tenantAudits))

	if err := quota.UpdateTenantQuotaAndGPUGroup(gc, updateTenantDetails, tenantAudits, req); err != nil {
		log.Error("failed to UpdateTenantQuotaAndGPUGroup")
		return err
	}

	return nil
}

func reduceRareGPUInChargedQuota(gc *ginlib.GinContext, req []*quotaTypes.GPUGroup) ([]*quotaTypes.TenantDetail, []interface{}, error) {
	currentRareGPUType, err := quota.GetRareGPUTypeMap(gc)
	if err != nil {
		return nil, nil, err
	}
	changedGPUType := make(map[corev1.ResourceName]bool)
	for i := range req {
		if req[i].GPUTypeLevel == quotaTypes.GPUTypeLevelNormal && currentRareGPUType[req[i].GPUType] {
			changedGPUType[types.ResourceNameWithGPUType(req[i].GPUType)] = true
			changedGPUType[types.ResourceNameWithVirtGPUType(req[i].GPUType)] = true
		}
	}

	_, tenantDetails, err := quota.ListTenantDetails(gc, "", nil, nil, true)
	if err != nil {
		return nil, nil, err
	}

	var backupTotalQuota quotaTypes.Quota
	updateTenantDetails := make([]*quotaTypes.TenantDetail, 0)
	tenantAudits := make([]interface{}, 0)
	for i := range tenantDetails {
		tenantDetails[i].TotalQuota = quotaTypes.BuildQuotaFromData(tenantDetails[i].DTotalQuota.FromMongo())
		backupTotalQuota = tenantDetails[i].TotalQuota.Add(nil)
		for k := range tenantDetails[i].TotalQuota {
			// 获取更新后的团队配额
			if !changedGPUType[k] {
				continue
			}
			tempKey := corev1.ResourceName(aistypes.NVIDIAGPUResourceName)
			tempValue := tenantDetails[i].TotalQuota[aistypes.NVIDIAGPUResourceName]
			if types.IsVirtGPUResourceName(k) {
				tempKey = aistypes.NVIDIAVirtGPUResourceName
				tempValue = tenantDetails[i].TotalQuota[aistypes.NVIDIAVirtGPUResourceName]
			}
			tempValue.Sub(tenantDetails[i].TotalQuota[k])
			tenantDetails[i].TotalQuota[tempKey] = tempValue
		}
		for k := range changedGPUType {
			delete(tenantDetails[i].TotalQuota, k)
		}

		diffTotalQuota := tenantDetails[i].TotalQuota.Sub(backupTotalQuota)
		if diffTotalQuota.IsZero() {
			continue
		}

		// 组装团队更新后配额和配额操作事件
		updateTenantDetails = append(updateTenantDetails, tenantDetails[i])
		tenantAudits = append(tenantAudits, &quotaTypes.TenantQuotaAudit{
			ID:                primitive.NewObjectID(),
			TenantID:          tenantDetails[i].TenantID,
			TenantName:        tenantDetails[i].TenantName,
			TenantDisplayName: tenantDetails[i].TenantDisplayName,
			DTotalQuota:       tenantDetails[i].TotalQuota.ToData().ToMongo(),
			DDiffQuota:        diffTotalQuota.ToData().ToMongo(),
			CreatedAt:         time.Now().Unix(),
			Creator:           gc.GetUserName(),
			CreatorID:         gc.GetUserID(),
		})
	}

	return updateTenantDetails, tenantAudits, nil
}

func CheckResourceNameValid(q quotaTypes.Quota) bool {
	for k := range q {
		if strings.HasPrefix(string(k), string(types.KubeResourceGPU)) {
			continue
		}
		if strings.HasPrefix(string(k), string(types.KubeResourceVirtGPU)) {
			continue
		}
		if strings.HasPrefix(string(k), string(types.KubeResourceNPU)) {
			continue
		}
		if !types.K8sResourceNameMap[k] {
			return false
		}
	}

	return true
}
