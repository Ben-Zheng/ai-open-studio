package quota

import (
	"context"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	k8sUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/k8s"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
)

func CheckPodQuota(
	_ context.Context,
	obj any,
	ar admissionv1.AdmissionReview,
	server *WebhookServer,
	dryRun bool,
) *admissionv1.AdmissionResponse {
	pod := obj.(*corev1.Pod)
	tenant := pod.Labels[aisTypes.AISTenantName]
	userName := pod.Labels[aisTypes.AISUserName]
	isCreate := true
	logger := log.WithFields(log.Fields{"namespace": pod.Namespace, "pod": pod.Name, "tenant": tenant, "operation": ar.Request.Operation})
	logger.Info("[Quota_checkPod] step1: check pod quota...")
	switch ar.Request.Operation {
	case admissionv1.Create:
	case admissionv1.Update:
		isCreate = false
		oldPod, err := getOldPod(ar)
		if err != nil {
			logger.WithError(err).Errorf("[Quota_checkPod] fail to decode raw into oldpod")
			return toV1beta1AdmissionDenied(err)
		}

		// 资源占用没有变化
		if !IsPodQuotaConsumeChanged(oldPod, pod) {
			logger.Info("[Quota_checkPod] step over: quota not changed, check pod over...")
			return v1beta1AdmissionAllowed()
		}
		logger.WithError(err).Info("[Quota_checkPod] check pod quota: pod quota consume changed")
	}

	if !isCreate { // 更新操作直接放行，无法编辑 Pod 的 labels 和 resources
		logger.Info("[Quota_checkPod] step over: skipped when pod is updating, check pod over...")
		return v1beta1AdmissionAllowed()
	}
	if ok, err := CheckPodLimits(pod); !ok { // 强制要求资源必须配置的 resources.limits
		logger.Error("[Quota_checkPod] pod must with resource limits field")
		return toV1beta1AdmissionDenied(err)
	}
	if server.lockFunc != nil {
		releaseLock, err := server.lockFunc(tenant)
		if err != nil {
			logger.Error("[Quota_checkPod] get release lock failed")
			return toV1beta1AdmissionDenied(err)
		}
		defer releaseLock()
	}

	logger.Info("[Quota_checkPod] step2: calculate tenant, user quota and obtain pod resource limit value")
	// 获取团队配额和pod资源需求
	tenantDetail, err := GetTenantDetail(ginlib.NewMockGinContext(), tenant)
	if err != nil {
		logger.WithError(err).Errorf("[Quota_checkPod] fail to GetTenantDetail")
		return toV1beta1AdmissionDenied(err)
	}
	userDetail, err := GetUserDetail(ginlib.NewMockGinContext(), tenant, userName)
	if err != nil {
		logger.WithError(err).Errorf("[Quota_checkPod] fail to GetUserDetail")
		return toV1beta1AdmissionDenied(err)
	}
	totalQuota := tenantDetail.TotalQuota
	chargedQuota := tenantDetail.ChargedQuota
	userChargedQuota := userDetail.ChargedQuota

	// 根据当前稀有卡类型重新计算普通卡占用, 添加必要的反亲和性属性
	logger.Info("[Quota_checkPod] step3: obtain pod resource and reCalculate gpu resource")
	rareGPUType, patchBytes, err := getRareGPUTypeLevelAndPatch(pod)
	if err != nil {
		logger.WithError(err).Error("[Quota_checkPod] fail to get rare gpu type")
		return toV1beta1AdmissionDenied(err)
	}
	ReCalculateNormalGPU(totalQuota, rareGPUType, false)
	ReCalculateNormalGPU(chargedQuota, rareGPUType, false)
	ReCalculateNormalGPU(userChargedQuota, rareGPUType, false)
	IncreasePodQuota(chargedQuota, pod, rareGPUType)
	IncreasePodQuota(userChargedQuota, pod, rareGPUType)
	usedResources := GetUsedResourceNames(obj, PodsResource, rareGPUType)

	// 校验资源是否超过了团队的配额
	logger.Info("[Quota_checkPod] step4: compare tenant quota and pod resource limit")
	if err = LessThanOrEqual(chargedQuota, totalQuota, usedResources); err != nil {
		logger.WithError(err).Error("[Quota_checkPod] fail to pass tenant quota check")
		return toV1beta1AdmissionDenied(fmt.Errorf("insufficient tenant quota: %s", err.Error()))
	}

	if dryRun {
		logger.Info("[Quota_checkPod] step over: only dry run, check pod over...")
		return v1beta1AdmissionAllowed()
	}

	// 更新团队配额 (admit 这里不扣减，而是在 sync 逻辑扣减的话，相当于形成了一个绕过配额检查的漏洞)
	logger.Infof("[Quota_checkPod] step5: update tenant and user charged quota, tenant charged: %s, user charged: %s", chargedQuota, userChargedQuota)
	if err = UpdateChargedQuota(ginlib.NewMockGinContext(), tenant, chargedQuota); err != nil {
		log.WithError(err).Error("[Quota_checkPod] fail to update tenant charged quota")
		return toV1beta1AdmissionDenied(fmt.Errorf("[Quota] consume tenant quota failed: %s", err.Error()))
	}
	if err = UpdateUserChargedQuota(ginlib.NewMockGinContext(), tenant, userName, userChargedQuota); err != nil {
		logger.WithError(err).Error("[Quota_checkPod] fail to update user charged quota")
		return toV1beta1AdmissionDenied(fmt.Errorf("[Quota] consume user quota failed: %s", err.Error()))
	}

	logger.Info("[Quota_checkPod] step over: congratulations, check pod over...")
	resp := admissionv1.AdmissionResponse{
		Allowed: true,
	}
	if patchBytes != nil {
		resp.Patch = patchBytes
		resp.PatchType = func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}()
	}
	return &resp
}

func CheckPVCQuota(
	_ context.Context,
	obj any,
	ar admissionv1.AdmissionReview,
	server *WebhookServer,
	dryRun bool,
) *admissionv1.AdmissionResponse {
	log.Debug("[Quota] prepare to check quota...")
	pvc := obj.(*corev1.PersistentVolumeClaim)
	logger := log.WithFields(log.Fields{
		"pvc": pvc.Name,
	})
	switch ar.Request.Operation {
	case admissionv1.Create:
		if !IsPVCChargeQuota(pvc, true) {
			logger.Infof("[Quota] pvc[name:%+v, ns:%+v] does not charge quota", pvc.Name, ar.Request.Namespace)
			return v1beta1AdmissionAllowed()
		}
	case admissionv1.Update:
		oldPvc, err := getOldPvc(ar)
		if err != nil {
			logger.WithError(err).Errorf("[Quota] fail to decode raw into oldpvc")
			return toV1beta1AdmissionDenied(err)
		}

		// resources changed
		if !(oldPvc.Spec.Resources.Limits != nil && pvc.Spec.Resources.Limits != nil &&
			oldPvc.Spec.Resources.Limits[types.KubeResourceStorage] != pvc.Spec.Resources.Limits[types.KubeResourceStorage]) {
			return v1beta1AdmissionAllowed()
		}
	}

	logger.Info("[Quota] check pvc quota...")
	tenant := pvc.Labels[aisTypes.AISTenantName]
	userName := pvc.Labels[aisTypes.AISUserName]
	usedResources := GetUsedResourceNames(obj, PersistentVolumeClaimsResource, nil)

	if server.lockFunc != nil {
		releaseLock, err := server.lockFunc(tenant)
		if err != nil {
			return toV1beta1AdmissionDenied(err)
		}
		defer releaseLock()
	}

	// 获取团队配额
	tenantDetail, err := GetTenantDetail(ginlib.NewMockGinContext(), tenant)
	if err != nil {
		return toV1beta1AdmissionDenied(err)
	}
	userDetail, err := GetUserDetail(ginlib.NewMockGinContext(), tenant, userName)
	if err != nil {
		return toV1beta1AdmissionDenied(err)
	}

	totalQuota := tenantDetail.TotalQuota
	chargedQuota := tenantDetail.ChargedQuota
	userChargedQuota := userDetail.ChargedQuota

	// 增加 ChargedQuota (是否需要考虑一些 竟态条件)
	IncreasePVCQuota(chargedQuota, pvc)
	IncreasePVCQuota(userChargedQuota, pvc)

	// 校验资源是否超过了团队的配额
	if err := LessThanOrEqual(chargedQuota, totalQuota, usedResources); err != nil {
		logger.WithError(err).Error("[Quota] fail to pass tenant quota check")
		err := fmt.Errorf("insufficient tenant quota: %s", err.Error())
		return toV1beta1AdmissionDenied(err)
	}

	if dryRun {
		return v1beta1AdmissionAllowed()
	}

	// 更新团队配额
	logger.Infof("[Quota] check try to update tenant charged quota: %s", chargedQuota)
	if err := UpdateChargedQuota(ginlib.NewMockGinContext(), tenant, chargedQuota); err != nil {
		logger.WithError(err).Error("fail to update tenant charged quota")
		err := fmt.Errorf("consume tenant quota failed: %s", err.Error())
		return toV1beta1AdmissionDenied(err)
	}
	logger.Infof("[Quota] check try to update user charged quota: %s", userChargedQuota)
	if err := UpdateUserChargedQuota(ginlib.NewMockGinContext(), tenant, userName, userChargedQuota); err != nil {
		logger.WithError(err).Error("[Quota] fail to update user charged quota")
		err := fmt.Errorf("[Quota] consume user quota failed: %s", err.Error())
		return toV1beta1AdmissionDenied(err)
	}
	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

func v1beta1AdmissionAllowed() *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

func toV1beta1AdmissionDenied(err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Message: err.Error(),
			// 可以考虑增加一个特殊的 failReason 类型
		},
	}
}

func getRareGPUTypeLevelAndPatch(pod *corev1.Pod) (map[string]bool, []byte, error) {
	if !features.IsQuotaSupportGPUGroupEnabled() {
		return nil, nil, nil
	}

	rareGPUType, err := GetRareGPUTypeMap(ginlib.NewMockGinContext())
	if err != nil {
		return nil, nil, err
	}

	// 如果集群没有设定稀缺卡，则直接返回
	if len(rareGPUType) == 0 {
		return rareGPUType, nil, nil
	}

	// 如果指定多个GPU型号亲和类型，指定的类型中不允许存在稀有类型
	gpuTypes := k8sUtils.GetPodGPUTypes(pod)
	if len(gpuTypes) > 1 {
		for i := range gpuTypes {
			if rareGPUType[gpuTypes[i]] {
				return nil, nil, fmt.Errorf("only one GPU type can be specified when the specified type includes rare types")
			}
		}
	}

	// 已指定GPU型号亲和性直接返回
	if len(gpuTypes) == 1 {
		return rareGPUType, nil, nil
	}

	// 如果未指定GPU型号亲和性需要添加反亲和性
	type PatchOperation struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value,omitempty"`
	}
	rareGPUTypeArr := make([]string, 0, len(rareGPUType))
	for k := range rareGPUType {
		rareGPUTypeArr = append(rareGPUTypeArr, k)
	}
	patchBytes, err := json.Marshal(createAffinityPatch(pod, rareGPUTypeArr))
	if err != nil {
		return nil, nil, err
	}

	return rareGPUType, patchBytes, nil
}

type patchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func createAffinityPatch(pod *corev1.Pod, rareGPUTypeArr []string) []patchOperation {
	var patches []patchOperation
	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &corev1.Affinity{}
		patches = append(patches, patchOperation{Op: "add", Path: "/spec/affinity", Value: corev1.Affinity{}})
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
		patches = append(patches, patchOperation{Op: "add", Path: "/spec/affinity/nodeAffinity", Value: corev1.NodeAffinity{}})
	}
	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
		patches = append(patches, patchOperation{Op: "add", Path: "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution", Value: corev1.NodeSelector{}})
	}
	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{}
		patches = append(patches, patchOperation{Op: "add", Path: "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution/nodeSelectorTerms", Value: []corev1.NodeSelectorTerm{}})
	}

	patch := []patchOperation{
		{
			Op:   "add",
			Path: "/spec/affinity/nodeAffinity/requiredDuringSchedulingIgnoredDuringExecution/nodeSelectorTerms",
			Value: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      aisTypes.GPUTypeKey,
						Operator: corev1.NodeSelectorOpNotIn,
						Values:   rareGPUTypeArr,
					}},
				},
			},
		},
	}

	patches = append(patches, patch...)
	return patches
}
