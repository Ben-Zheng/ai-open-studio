package quota

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sQuota "k8s.io/kubernetes/pkg/quota/v1/evaluator/core"
	"k8s.io/utils/clock"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	k8sUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/k8s"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

func podLimits(pod *corev1.Pod, rareGPUType map[string]bool) corev1.ResourceList {
	limits := corev1.ResourceList{}
	for i := range pod.Spec.Containers {
		limits = SumResourceList(limits, pod.Spec.Containers[i].Resources.Limits)
	}
	for i := range pod.Spec.InitContainers {
		limits = MaxResourceList(limits, pod.Spec.InitContainers[i].Resources.Limits)
	}
	if pod.Spec.Overhead != nil {
		limits = SumResourceList(limits, pod.Spec.Overhead)
	}
	// Wrap GPUType sub resources
	fillGPUTypeResources(pod, limits, rareGPUType)
	return limits
}

// fillGPUTypeResources 对于指定节点类型的任务，需要统一指定规则和校验规则，只允许指定一种亲和类型
func fillGPUTypeResources(pod *corev1.Pod, limits corev1.ResourceList, rareGPUType map[string]bool) {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		return
	}

	gpuTypes := k8sUtils.GetPodGPUTypes(pod)
	// gpuTypes 情况说明，已在getRareGPUTypeLevelAndPatch进行了相关限制和处理
	// 数量为0：已添加反亲和性，使用普通卡配额
	// 数量为1：已添加亲和性，若为普通卡则使用普通卡配额，若为稀缺卡使用稀缺卡配额
	// 数量为2或更多：已添加反亲和性，全部都为普通卡，使用普通卡配额
	if value, exists := limits[types.KubeResourceGPU]; exists {
		switch len(gpuTypes) {
		case 0:
			limits[types.ResourceNameWithGPUType(types.AisNormalGPUType)] = value.DeepCopy()
		case 1:
			if rareGPUType[gpuTypes[0]] {
				// 填充稀有类型
				limits[types.ResourceNameWithGPUType(gpuTypes[0])] = value.DeepCopy()
			} else {
				// 填充 AisNormalGPUType 类型
				limits[types.ResourceNameWithGPUType(types.AisNormalGPUType)] = value.DeepCopy()
			}
		default:
			limits[types.ResourceNameWithGPUType(types.AisNormalGPUType)] = value.DeepCopy()
		}
	}
	// 校验 npu
	apuConfigs, _ := LoadAPUConfigs()
	for _, apuc := range apuConfigs {
		if apuc.Kind != aisTypes.HUAWEINPUResourceName {
			continue
		}
		value, exist := limits[apuc.ResourceName]
		if !exist || value.Value() == 0 {
			continue
		}

		limits[types.KubeResourceNPU] = value.DeepCopy()
		if rareGPUType[apuc.Product] {
			limits[types.ResourceNameWithNPUType(apuc.Product)] = value.DeepCopy()
		} else {
			limits[types.ResourceNameWithNPUType(types.AisNormalGPUType)] = value.DeepCopy()
		}
		break
	}
}

func CheckIfPodUseGPU(pod *corev1.Pod) bool {
	limits := podLimits(pod, nil)
	quantity := limits[types.KubeResourceGPU]
	return !quantity.IsZero()
}

func CheckPodLimits(pod *corev1.Pod) (bool, error) {
	noLimits := true
	var cpuNum, memNum int64
	for index := range pod.Spec.Containers {
		if pod.Spec.Containers[index].Resources.Limits == nil {
			continue
		}
		noLimits = false
		cpuNum += pod.Spec.Containers[index].Resources.Limits.Cpu().MilliValue() // 这样计算后的结果为 xxx m
		memNum += pod.Spec.Containers[index].Resources.Limits.Memory().Value()   // 这样计算后的结果为 xxx bytes
	}

	if noLimits {
		err := fmt.Errorf("[Quota] resource should not be empty")
		return false, err
	}

	var limitserr []string
	if memNum == 0 {
		limitserr = append(limitserr, "memory cannot be 0")
	}
	if cpuNum == 0 {
		limitserr = append(limitserr, "cpu cannot be 0")
	}
	if len(limitserr) != 0 {
		err := errors.New(strings.Join(limitserr, ", "))
		return false, err
	}
	return true, nil
}

func GetPodUsageToQuota(pod *corev1.Pod, rareGPUType map[string]bool) quotaTypes.Quota {
	quota := quotaTypes.Quota{}
	limits := podLimits(pod, rareGPUType)
	for resourceName, quantity := range limits {
		quota[resourceName] = quantity
	}

	log.Debugf("[Quota] obtain pod resource limit: %s, %s", pod.Name, quota)
	return quota
}

func GetPVCUsageToQuota(pvc *corev1.PersistentVolumeClaim) quotaTypes.Quota {
	quota := quotaTypes.Quota{}
	if pvc.Spec.Resources.Limits != nil {
		if pvcStorage, ok := pvc.Spec.Resources.Limits[types.KubeResourceStorage]; ok {
			quota[types.KubeResourceStorage] = pvcStorage
		}
		return quota
	}
	if pvc.Spec.Resources.Requests != nil {
		if pvcStorage, ok := pvc.Spec.Resources.Requests[types.KubeResourceStorage]; ok {
			quota[types.KubeResourceStorage] = pvcStorage
		}
	}
	return quota
}

func GetPodUsageToUsed(pod *corev1.Pod, rareGPUType map[string]bool) quotaTypes.Quota {
	used := quotaTypes.Quota{}
	limits := podLimits(pod, rareGPUType)
	for resourceName, quantity := range limits {
		used[resourceName] = quantity
	}
	if !used.IsZero() {
		log.Debugf("[Quota] pod usage resource: %s, %s", pod.Name, used)
	}
	return used
}

func GetPVCUsageToUsed(pvc *corev1.PersistentVolumeClaim) quotaTypes.Quota {
	used := quotaTypes.Quota{}
	if pvc.Spec.Resources.Limits != nil {
		if pvcStorage, ok := pvc.Spec.Resources.Limits[types.KubeResourceStorage]; ok {
			used[types.KubeResourceStorage] = pvcStorage
		}
		return used
	}
	if pvc.Spec.Resources.Requests != nil {
		if pvcStorage, ok := pvc.Spec.Resources.Requests[types.KubeResourceStorage]; ok {
			used[types.KubeResourceStorage] = pvcStorage
		}
	}
	return used
}

func IncreasePodQuota(quota quotaTypes.Quota, pod *corev1.Pod, rareGPUType map[string]bool) {
	quota.InplaceAdd(GetPodUsageToQuota(pod, rareGPUType))
}

func IncreasePodResource(used quotaTypes.Quota, pod *corev1.Pod, rareGPUType map[string]bool) {
	used.InplaceAdd(GetPodUsageToUsed(pod, rareGPUType))
}

func IncreasePVCQuota(quota quotaTypes.Quota, pvc *corev1.PersistentVolumeClaim) {
	quota.InplaceAdd(GetPVCUsageToQuota(pvc))
}

func IncreasePVCResource(cur quotaTypes.Quota, pvc *corev1.PersistentVolumeClaim) {
	cur.InplaceAdd(GetPVCUsageToUsed(pvc))
}

func SumResourceList(a, b corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for key, value := range a {
		quantity := value.DeepCopy()
		if other, found := b[key]; found {
			quantity.Add(other)
		}
		result[key] = quantity
	}
	for key, value := range b {
		if _, found := result[key]; !found {
			result[key] = value.DeepCopy()
		}
	}
	return result
}

func MaxResourceList(a corev1.ResourceList, b corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for key, value := range a {
		if other, found := b[key]; found {
			if value.Cmp(other) <= 0 {
				result[key] = other.DeepCopy()
				continue
			}
		}
		result[key] = value.DeepCopy()
	}
	for key, value := range b {
		if _, found := result[key]; !found {
			result[key] = value.DeepCopy()
		}
	}
	return result
}

func LessThanOrEqual(chargedQuota, totalQuota quotaTypes.Quota, names map[corev1.ResourceName]bool) error {
	errMsg := ""
	msgFunc := func(resourceName corev1.ResourceName, cur, total k8sresource.Quantity) string {
		var curValue, totalValue string
		if resourceName == types.KubeResourceStorage || resourceName == types.KubeResourceMemory {
			curValue = aisUtils.ConvertByteToGiB(cur.Value()) + "Gi"
			totalValue = aisUtils.ConvertByteToGiB(total.Value()) + "Gi"
		} else {
			curValue = fmt.Sprintf("%1d", cur.Value())
			totalValue = fmt.Sprintf("%1d", total.Value())
		}
		return fmt.Sprintf("%v: chargedValue(%v)/totalValue(%v)", resourceName, curValue, totalValue)
	}

	for resourceName := range names {
		if !IsResourcesToaBeCheck(resourceName) {
			continue
		}
		cur := chargedQuota.GetByResourceName(resourceName)
		total := totalQuota.GetByResourceName(resourceName)
		if cur.Cmp(total) > 0 {
			errMsg += msgFunc(resourceName, cur, total)
			continue
		}
	}

	if errMsg != "" {
		err := errors.New(errMsg)
		return err
	}
	return nil
}

func IsResourcesToaBeCheck(resourceName corev1.ResourceName) bool {
	return resourcesToBeCheck[resourceName] || types.IsGPUResourceName(resourceName) || types.IsVirtGPUResourceName(resourceName) || types.IsNPUResourceName(resourceName)
}

func GetUsedResourceNames(obj any, group string, rareGPUType map[string]bool) map[corev1.ResourceName]bool {
	res := make(map[corev1.ResourceName]bool)
	switch group {
	case PodsResource:
		pod := obj.(*corev1.Pod)
		limits := podLimits(pod, rareGPUType)
		for k, v := range limits {
			if !v.IsZero() {
				res[k] = true
			}
		}
	case PersistentVolumeClaimsResource:
		pvc := obj.(*corev1.PersistentVolumeClaim)
		if pvc.Spec.Resources.Limits != nil {
			if pvcStorage, ok := pvc.Spec.Resources.Limits[types.KubeResourceStorage]; ok {
				if !pvcStorage.IsZero() {
					res[types.KubeResourceStorage] = true
					return res
				}
			}
		}
		if pvc.Spec.Resources.Requests != nil {
			if pvcStorage, ok := pvc.Spec.Resources.Requests[types.KubeResourceStorage]; ok {
				if !pvcStorage.IsZero() {
					res[types.KubeResourceStorage] = true
					return res
				}
			}
		}
	case DatasetStorageResource:
		res[types.KubeResourceDatasetStorage] = true
	default:
		return res
	}
	return res
}

func IsPodQuotaEnabled(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.GetLabels()[aisTypes.AISQuotaLabelKey] != aisTypes.AISQuotaLabelValue {
		return false
	}
	return true
}

// 判断 Pod 当前是否占用配额
func IsPodChargedQuota(pod *corev1.Pod) bool {
	if !IsPodQuotaEnabled(pod) {
		return false
	}
	// 过滤了处于终态的 Pod
	return k8sQuota.QuotaV1Pod(pod, clock.RealClock{})
}

// 判断 Pod 当前是否用户占用配额
func IsPodUserChargedQuota(pod *corev1.Pod) bool {
	if IsPodChargedQuota(pod) && pod.GetLabels()[aisTypes.AISUserName] != "" {
		return true
	}
	return false
}

// 判断 Pod 当前是否在实际使用配额
func IsPodUsedQuota(pod *corev1.Pod) bool {
	if pod.Spec.NodeName == "" {
		return false
	}

	return IsPodChargedQuota(pod)
}

// 判断 Pod 当前是否占用资源
func IsPodUsedResource(pod *corev1.Pod) bool {
	if pod.Spec.NodeName == "" {
		return false
	}
	return k8sQuota.QuotaV1Pod(pod, clock.RealClock{})
}

func IsPodQuotaConsumeChanged(oldPod, curPod *corev1.Pod) bool {
	// 1. 使用资源 或者 使用配额 => 未使用资源 （对应：Pod 从创建到创建失败 或者从运行中到终态）
	// 2. 未使用资源 => 使用资源 (对应：调度成功)
	return ((IsPodUsedResource(oldPod) || IsPodChargedQuota(oldPod)) && !IsPodUsedResource(curPod)) ||
		(!IsPodUsedResource(oldPod) && IsPodUsedResource(curPod))
}

func IsPVCQuotaEnabled(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc == nil {
		return false
	}
	if pvc.GetLabels()[aisTypes.AISQuotaLabelKey] != aisTypes.AISQuotaLabelValue {
		return false
	}

	if pvc.GetAnnotations()[aisTypes.AISResourceType] == string(aisTypes.AIServiceTypeWorkspace) {
		return false
	}

	return true
}

// 判断 PVC 是否占用配额
func IsPVCChargeQuota(pvc *corev1.PersistentVolumeClaim, createPVC bool) bool {
	if !IsPVCQuotaEnabled(pvc) {
		return false
	}

	// pvc consumeForPod 的情况
	if !createPVC { // nolint
		// 如果 Pending 状态的 PVC 不纳入 Quota 统计，那么当前实现可能存在绕过限制的情况（例如针对 WaitForConsumeer
		// if pvc.Status.Phase != corev1.ClaimBound {
		// 	return false
		// }
	}

	if pvc.DeletionTimestamp != nil && pvc.DeletionGracePeriodSeconds != nil {
		now := clock.RealClock{}.Now()
		deletionTime := pvc.DeletionTimestamp.Time
		gracePeriod := time.Duration(*pvc.DeletionGracePeriodSeconds) * time.Second
		if now.After(deletionTime.Add(gracePeriod)) {
			return false
		}
	}
	if pvc.DeletionTimestamp != nil {
		return false
	}
	return true
}

// 判断 PVC 是否用户占用配额
func IsPVCUserChargedQuota(pvc *corev1.PersistentVolumeClaim) bool {
	if IsPVCChargeQuota(pvc, false) && pvc.GetLabels()[aisTypes.AISUserName] != "" {
		return true
	}
	return false
}

// 判断 PVC 是否占用资源
func IsPVCUsedResource(pvc *corev1.PersistentVolumeClaim, createPVC bool) bool {
	if pvc == nil {
		return false
	}
	if !createPVC {
		if pvc.Status.Phase != corev1.ClaimBound {
			return false
		}
	}

	if pvc.DeletionTimestamp != nil && pvc.DeletionGracePeriodSeconds != nil {
		now := clock.RealClock{}.Now()
		deletionTime := pvc.DeletionTimestamp.Time
		gracePeriod := time.Duration(*pvc.DeletionGracePeriodSeconds) * time.Second
		if now.After(deletionTime.Add(gracePeriod)) {
			return false
		}
	}
	if pvc.DeletionTimestamp != nil {
		return false
	}
	return true
}

func OverwriteQuota(gvr metav1.GroupVersionResource, chargedQuota, usedQuota, used, oChargedQuota, oUsedQuota, oUsed quotaTypes.Quota) {
	switch gvr {
	case ResourcesGVRMapInstance[PodsResource]:
		resourceNames := []corev1.ResourceName{types.KubeResourceStorage, types.KubeResourceDatasetStorage}
		chargedQuota.Overwrite(oChargedQuota, resourceNames, nil)
		usedQuota.Overwrite(oUsedQuota, resourceNames, nil)
		used.Overwrite(oUsed, resourceNames, nil)
	case ResourcesGVRMapInstance[PersistentVolumeClaimsResource]:
		resourceNames := []corev1.ResourceName{types.KubeResourceStorage}
		chargedQuota.Overwrite(oChargedQuota, nil, resourceNames)
		usedQuota.Overwrite(oUsedQuota, nil, resourceNames)
		used.Overwrite(oUsed, nil, resourceNames)
	case ResourcesGVRMapInstance[DatasetStorageResource]:
		resourceNames := []corev1.ResourceName{types.KubeResourceDatasetStorage}
		chargedQuota.Overwrite(oChargedQuota, nil, resourceNames)
		usedQuota.Overwrite(oUsedQuota, nil, resourceNames)
		used.Overwrite(oUsed, nil, resourceNames)
	}
}

func getOldPod(ar admissionv1.AdmissionReview) (*corev1.Pod, error) {
	raw := ar.Request.OldObject.Raw
	deserializer := codecs.UniversalDeserializer()
	oldPod := &corev1.Pod{}
	if _, _, err := deserializer.Decode(raw, nil, oldPod); err != nil {
		log.WithError(err).Error("[Quota] fail to decode raw into oldPod")
		return nil, err
	}
	return oldPod, nil
}

func getOldPvc(ar admissionv1.AdmissionReview) (*corev1.PersistentVolumeClaim, error) {
	raw := ar.Request.OldObject.Raw
	deserializer := codecs.UniversalDeserializer()
	oldPvc := &corev1.PersistentVolumeClaim{}
	if _, _, err := deserializer.Decode(raw, nil, oldPvc); err != nil {
		log.WithError(err).Error("[Quota] fail to decode raw into oldPod")
		return nil, err
	}
	return oldPvc, nil
}

func ReCalculateNormalGPU(q quotaTypes.Quota, rareGPUType map[string]bool, fillAllRareGPUType bool) {
	// 1. 根据最新的稀有卡类型，将原为稀有卡型号的GPU数转移至普通卡，并删除原稀有卡key，用于用户的和团队GPU正在占用的修改
	toBeDeleted := make(map[corev1.ResourceName]bool)
	for k := range q {
		resourceName, gpuType := getAPUType(k)
		switch resourceName {
		case aisTypes.NVIDIAGPUResourceName:
			if rareGPUType[gpuType] || gpuType == types.AisNormalGPUType {
				continue
			}
			normalGPUValue := q[types.ResourceNameWithGPUType(types.AisNormalGPUType)]
			normalGPUValue.Add(q[k])
			q[types.ResourceNameWithGPUType(types.AisNormalGPUType)] = normalGPUValue
			toBeDeleted[k] = true
		case aisTypes.NVIDIAVirtGPUResourceName:
			if rareGPUType[gpuType] || gpuType == types.AisNormalGPUType {
				continue
			}
			normalGPUValue := q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)]
			normalGPUValue.Add(q[k])
			q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)] = normalGPUValue
			toBeDeleted[k] = true
		case aisTypes.HUAWEINPUResourceName:
			if rareGPUType[gpuType] || gpuType == types.AisNormalGPUType {
				continue
			}
			normalGPUValue := q[types.ResourceNameWithNPUType(types.AisNormalGPUType)]
			normalGPUValue.Add(q[k])
			q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)] = normalGPUValue
			toBeDeleted[k] = true
		}
	}

	for k := range toBeDeleted {
		delete(q, k)
	}

	// 2. 当开关开启时，给前端使用，填充稀有卡类型，如果稀有卡类型不存在则填充0值
	if fillAllRareGPUType {
		apuConfigs, _ := LoadAPUConfigs()
		for _, ac := range apuConfigs {
			if _, ok := rareGPUType[ac.Product]; !ok {
				continue
			}
			switch ac.Kind {
			case aisTypes.NVIDIAGPUResourceName:
				if _, exist := q[types.ResourceNameWithGPUType(ac.Product)]; !exist {
					q[types.ResourceNameWithGPUType(ac.Product)] = k8sresource.Quantity{}
				}
			case aisTypes.NVIDIAVirtGPUResourceName:
				if _, exist := q[types.ResourceNameWithVirtGPUType(ac.Product)]; !exist {
					q[types.ResourceNameWithVirtGPUType(ac.Product)] = k8sresource.Quantity{}
				}
			case aisTypes.HUAWEINPUResourceName:
				if _, exist := q[types.ResourceNameWithNPUType(ac.Product)]; !exist {
					q[types.ResourceNameWithNPUType(ac.Product)] = k8sresource.Quantity{}
				}
			}
		}
	}

	// 3. 当开关开启时，兼容旧版切换至新版时GPU配额一致性，保证 nvidia.com/gpu = nvidia.com/gpu.AisNormal + nvidia.com/gpu.稀缺卡型号
	if features.IsQuotaSupportGPUGroupEnabled() {
		var rareNvidiaGPU, rareVirtGPU, rareNPU k8sresource.Quantity
		for k := range q {
			resourceName, gpuType := getAPUType(k)
			switch resourceName {
			case aisTypes.NVIDIAGPUResourceName:
				if !rareGPUType[gpuType] {
					continue
				}
				rareNvidiaGPU.Add(q[k])
			case aisTypes.NVIDIAVirtGPUResourceName:
				if !rareGPUType[gpuType] {
					continue
				}
				rareVirtGPU.Add(q[k])
			case aisTypes.HUAWEINPUResourceName:
				if !rareGPUType[gpuType] {
					continue
				}
				rareNPU.Add(q[k])
			}
		}
		rareNvidiaGPU.Add(q[types.ResourceNameWithGPUType(types.AisNormalGPUType)])
		if !rareNvidiaGPU.Equal(q[aisTypes.NVIDIAGPUResourceName]) {
			log.Warnf("number of rare plus normal does not equal the total gpu, please update tenant quota manually: %+v", q)
			totalTemp := q[aisTypes.NVIDIAGPUResourceName]
			rareNvidiaGPU.Sub(q[types.ResourceNameWithGPUType(types.AisNormalGPUType)])
			totalTemp.Sub(rareNvidiaGPU)
			q[types.ResourceNameWithGPUType(types.AisNormalGPUType)] = totalTemp
		}
		rareVirtGPU.Add(q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)])
		if !rareVirtGPU.Equal(q[aisTypes.NVIDIAVirtGPUResourceName]) {
			log.Warnf("number of rare plus normal does not equal the total virt gpu, please update tenant quota manually: %+v", q)
			totalTemp := q[aisTypes.NVIDIAVirtGPUResourceName]
			rareVirtGPU.Sub(q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)])
			totalTemp.Sub(rareVirtGPU)
			q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)] = totalTemp
		}
		rareNPU.Add(q[types.ResourceNameWithNPUType(types.AisNormalGPUType)])
		if !rareNPU.Equal(q[aisTypes.HUAWEINPUResourceName]) {
			log.Warnf("number of rare plus normal does not equal the total npu, please update tenant quota manually: %+v", q)
			totalTemp := q[aisTypes.HUAWEINPUResourceName]
			rareNPU.Sub(q[types.ResourceNameWithNPUType(types.AisNormalGPUType)])
			totalTemp.Sub(rareNPU)
			q[types.ResourceNameWithNPUType(types.AisNormalGPUType)] = totalTemp
		}
	}

	if features.IsQuotaSupportGPUGroupEnabled() {
		if _, exist := q[types.ResourceNameWithGPUType(types.AisNormalGPUType)]; !exist {
			q[types.ResourceNameWithGPUType(types.AisNormalGPUType)] = k8sresource.Quantity{}
		}
		if _, exist := q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)]; !exist {
			q[types.ResourceNameWithVirtGPUType(types.AisNormalGPUType)] = k8sresource.Quantity{}
		}
		if _, exist := q[types.ResourceNameWithNPUType(types.AisNormalGPUType)]; !exist {
			q[types.ResourceNameWithNPUType(types.AisNormalGPUType)] = k8sresource.Quantity{}
		}
	}
}

func GetRareGPUTypeMap(gc *ginlib.GinContext) (map[string]bool, error) {
	gpuGroups, _, err := ListGPUGroup(gc, &ListGPUGroupParam{GPUTypeLevel: quotaTypes.GPUTypeLevelRare}, true)
	if err != nil {
		return nil, err
	}

	rareGPUType := make(map[string]bool)
	for i := range gpuGroups {
		rareGPUType[gpuGroups[i].GPUType] = true
	}

	return rareGPUType, err
}

func GetRareGPUTypeMapWithFeatureGate(gc *ginlib.GinContext) (map[string]bool, error) {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		return nil, nil
	}

	gpuGroups, _, err := ListGPUGroup(gc, &ListGPUGroupParam{GPUTypeLevel: quotaTypes.GPUTypeLevelRare}, true)
	if err != nil {
		return nil, err
	}

	rareGPUType := make(map[string]bool)
	for i := range gpuGroups {
		rareGPUType[gpuGroups[i].GPUType] = true
	}

	return rareGPUType, err
}

func getAPUType(resourceName corev1.ResourceName) (string, string) {
	gpuType := types.GPUTypeFromResourceName(resourceName)
	if gpuType != "" {
		return aisTypes.NVIDIAGPUResourceName, gpuType
	}

	gpuType = types.VirtGPUTypeFromResourceName(resourceName)
	if gpuType != "" {
		return aisTypes.NVIDIAVirtGPUResourceName, gpuType
	}

	npuType := types.NPUTypeFromResourceName(resourceName)
	if npuType != "" {
		return aisTypes.HUAWEINPUResourceName, npuType
	}

	return "", ""
}

func LoadAPUConfigs() ([]*aisTypes.APUConfig, error) {
	var apucs, unSupply []*aisTypes.APUConfig
	for _, apu := range consts.ConfigMap.APUConfigs {
		if apu.Product != "" {
			apucs = append(apucs, apu)
		} else {
			unSupply = append(unSupply, apu)
		}
	}
	if consts.ReleaseClient == nil || consts.ResourceClient.NodeLister == nil {
		log.Warnln("get ResourceClient error")
		return consts.ConfigMap.APUConfigs, nil
	}

	nodes, err := consts.ResourceClient.NodeLister.List(consts.ResourceClient.NodeSelector)
	if err != nil {
		log.WithError(err).Error("get details, list nodes failed")
		return nil, err
	}

	for _, apu := range unSupply {
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
	return apucs, nil
}
