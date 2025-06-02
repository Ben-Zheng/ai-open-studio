package quota

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slables "k8s.io/apimachinery/pkg/labels"
	k8sselector "k8s.io/apimachinery/pkg/selection"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

func reconcileObjs(cm *ControllerManager, gvr metav1.GroupVersionResource, ns, tenant string, resync bool) error {
	logger := log.WithFields(log.Fields{
		"gvr":       gvr.String(),
		"namespace": ns,
		"tenant":    tenant,
		"resync":    resync,
	})
	logger.Infof("[Quota_reconcile] step1: reconcile started...")
	// 由于 一个 tenant 包含多个 project，每个 project 对应一个 namespace
	// 不能只统计该 namespace 的资源占用
	ns = ""
	startTime := time.Now()
	defer func() {
		logger.Infof("[Quota_reconcile] step over: cost %d ms, reconcile finished...", time.Since(startTime).Milliseconds())
	}()

	logger.Infof("[Quota_reconcile] step2: list tenant and system workloads")
	// 包含团队占用 和 系统占用 systemadmin 的 Workloads 列表
	labelSelector, err := buildSelector(tenant)
	if err != nil {
		logger.WithError(err).Error("[Quota_reconcile] failed to build selector")
		return err
	}
	sharedObjs, err := getObjs(cm, gvr, tenant, ns, labelSelector)
	if err != nil {
		logger.WithError(err).Errorf("[Quota_reconcile] failed to getObjs: %s", labelSelector.String())
		return err
	}
	if len(sharedObjs) > tooManyPodsWarningThreshold {
		logger.Warnf("[Quota_reconcile] too many objs reconcile warning: %d", len(sharedObjs))
	}

	logger.Infof("[Quota_reconcile] step3: caculate resource actually used and charged")
	// 可以先不考虑效率 全部统计一遍
	chargedQuota := quotaTypes.NilQuota()
	usedQuota := quotaTypes.NilQuota()
	usedResource := quotaTypes.NilQuota()
	users := getTenantUsers(tenant, resync)
	var rareGPUType map[string]bool
	if features.IsQuotaSupportGPUGroupEnabled() {
		if rareGPUType, err = GetRareGPUTypeMap(ginlib.NewMockGinContext()); err != nil {
			return err
		}
	}
	calculateUsageWithObjs(gvr, sharedObjs, chargedQuota, usedQuota, usedResource, users, rareGPUType)

	// 更新团队配额占用，将实际占用和使用替换数据库已有数据
	logger.Infof("[Quota_reconcile] step4 overwrite tenant detail, charged quota: %s, UsedQuota: %s, UsedResource: %s", chargedQuota, usedQuota, usedResource)
	tenantDetail, err := GetTenantDetail(ginlib.NewMockGinContext(), tenant)
	if err != nil {
		logger.WithError(err).Error("get tenant detail failed")
		return err
	}
	// 这里的竞态条件，由 enqueue tenantKey 有序入队处理保证
	logger.Infof("[Quota_reconcile] step4.1 before, ChargedQuota: %+v", chargedQuota)
	OverwriteQuota(gvr, chargedQuota, usedQuota, usedResource, tenantDetail.ChargedQuota, tenantDetail.UsedQuota, tenantDetail.Used)
	logger.Infof("[Quota_reconcile] step4.2 after, ChargedQuota: %+v", chargedQuota)
	if err = UpdateChargedAndUsedQuota(ginlib.NewMockGinContext(), tenant, chargedQuota, usedQuota, usedResource); err != nil {
		logger.WithError(err).Error("update tenant detail failed")
		return err
	}

	// 更新用户配额占用
	logger.Infof("[Quota_reconcile] step5 overwrite all user detail in this tenant")
	if tenant != aisTypes.AISSystemAdminTenantName {
		for userName, user := range users {
			logger = logger.WithFields(log.Fields{"user": userName})
			userDetail, err := GetUserDetail(ginlib.NewMockGinContext(), tenant, userName)
			if err != nil {
				logger.WithError(err).Error("get user detail failed")
				continue
			}
			OverwriteQuota(gvr, user.ChargedQuota, user.UsedQuota, user.UsedResource, userDetail.ChargedQuota, userDetail.UsedQuota, userDetail.Used)
			logger.Infof("[Quota_reconcile] overwrite user detail, charged quota: %s, UsedQuota: %s, UsedResource: %s", user.ChargedQuota, user.UsedQuota, user.UsedResource)
			if err = UpdateUserChargedAndUsedQuota(ginlib.NewMockGinContext(), tenant, userName, user.ChargedQuota, user.UsedQuota, user.UsedResource); err != nil {
				logger.WithError(err).WithField("user", userName).Error("update user detail failed")
				continue
			}
		}
	}
	return nil
}

func calculateUsageWithObjs(gvr metav1.GroupVersionResource, objs []any, chargedQuota, usedQuota quotaTypes.Quota, usedResource quotaTypes.Quota, users map[string]*UserCharged, rareGPUType map[string]bool) {
	switch gvr {
	case ResourcesGVRMapInstance[PodsResource]:
		pods := make([]*corev1.Pod, len(objs))
		for index := range objs {
			pods[index] = objs[index].(*corev1.Pod)
		}
		calculateUsageWithPods(pods, chargedQuota, usedQuota, usedResource, users, rareGPUType)
	case ResourcesGVRMapInstance[PersistentVolumeClaimsResource]:
		pvcs := make([]*corev1.PersistentVolumeClaim, len(objs))
		for index := range objs {
			pvcs[index] = objs[index].(*corev1.PersistentVolumeClaim)
		}
		calculateUsageWithPVCs(pvcs, chargedQuota, usedQuota, usedResource, users)
	default:
		log.Warningf("[Quota] invalid gvr:%+v", gvr)
	}
}

func calculateUsageWithPVCs(pvcs []*corev1.PersistentVolumeClaim, chargedQuota, usedQuota quotaTypes.Quota, usedResource quotaTypes.Quota, users map[string]*UserCharged) {
	for _, pvc := range pvcs {
		if IsPVCChargeQuota(pvc, false) {
			log.Debugf("[Quota] pvc charged quota: %s", pvc.Name)
			// FIXME: 暂时先不考虑 PVC 被销毁但是 PV 依旧占用的情况 （目前没有这种业务场景）
			IncreasePVCQuota(chargedQuota, pvc)
			IncreasePVCQuota(usedQuota, pvc)
		}

		// pvc 资源占用
		if IsPVCUsedResource(pvc, false) {
			IncreasePVCResource(usedResource, pvc)
		}

		// 统计用户相关的配额占用
		userName := pvc.GetLabels()[aisTypes.AISUserName]
		user := getOrSetUser(users, userName)
		if user != nil && IsPVCUserChargedQuota(pvc) {
			IncreasePVCQuota(user.ChargedQuota, pvc)
			if IsPVCChargeQuota(pvc, false) {
				IncreasePVCQuota(user.UsedQuota, pvc)
			}
			if IsPVCUsedResource(pvc, false) {
				IncreasePVCResource(user.UsedResource, pvc)
			}
		}
	}
}

func calculateUsageWithPods(pods []*corev1.Pod, chargedQuota, usedQuota quotaTypes.Quota, usedResource quotaTypes.Quota, users map[string]*UserCharged, rareGPUType map[string]bool) {
	// 该团队的所有的 pods | 或者系统占用的所有 pods
	for _, pod := range pods {
		// 团队配额相关
		if IsPodChargedQuota(pod) {
			log.Debugf("[Quota] pod charged tenant quota: %s", pod.Name)
			IncreasePodQuota(chargedQuota, pod, rareGPUType)
		}

		if IsPodUsedQuota(pod) {
			log.Debugf("[Quota] pod used tenant quota: %s", pod.Name)
			IncreasePodQuota(usedQuota, pod, rareGPUType)
		}

		// 针对团队而言，这里可以认为统计的 UsedResource == UsedQuota ?
		if IsPodUsedResource(pod) {
			log.Debugf("[Quota] pod used tenant resource: %s", pod.Name)
			IncreasePodResource(usedResource, pod, rareGPUType)
			log.Debugf("[Quota] tenant used resource: %s", usedResource)
		}

		// 统计用户相关的配额占用
		userName := pod.GetLabels()[aisTypes.AISUserName]
		user := getOrSetUser(users, userName)
		if user != nil && IsPodUserChargedQuota(pod) {
			log.Debugf("[Quota] pod charged user quota: %s", pod.Name)
			IncreasePodQuota(user.ChargedQuota, pod, rareGPUType)
			if IsPodUsedQuota(pod) {
				log.Debugf("[Quota] pod used user quota: %s", pod.Name)
				IncreasePodQuota(user.UsedQuota, pod, rareGPUType)
			}
			if IsPodUsedResource(pod) {
				log.Debugf("[Quota] pod used user resource: %s", pod.Name)
				IncreasePodResource(user.UsedResource, pod, rareGPUType)
			}
		}
	}
}

func getObjTenant(obj metav1.Object) string {
	tenant := obj.GetLabels()[aisTypes.AISTenantName]
	if tenant == "" {
		return aisTypes.AISSystemAdminTenantName
	}
	return tenant
}

func buildSelector(tenant string) (k8slables.Selector, error) {
	labelSelector := k8slables.NewSelector()
	if tenant == aisTypes.AISSystemAdminTenantName {
		// 这里的正确性保证， 由 quota admission control 保证拒绝掉这些 labels 不合法的 Pod 创建
		noQuotaRequirement, err := k8slables.NewRequirement(aisTypes.AISQuotaLabelKey, k8sselector.DoesNotExist, nil)
		if err != nil {
			log.WithError(err).Error("[Quota] failed to new requirement")
			return nil, err
		}
		labelSelector = labelSelector.Add(*noQuotaRequirement)
	} else {
		quotaRequirement, err := k8slables.NewRequirement(aisTypes.AISQuotaLabelKey, k8sselector.Equals, []string{aisTypes.AISQuotaLabelValue})
		if err != nil {
			log.WithError(err).Error("[Quota] failed to new requirement")
			return nil, err
		}
		tenantRequirement, err := k8slables.NewRequirement(aisTypes.AISTenantName, k8sselector.Equals, []string{tenant})
		if err != nil {
			log.WithError(err).Error("[Quota] failed to new requirement")
			return nil, err
		}
		labelSelector = labelSelector.Add(*quotaRequirement, *tenantRequirement)
	}
	return labelSelector, nil
}

func getObjs(cm *ControllerManager, gvr metav1.GroupVersionResource, tenant, ns string, labelSelector k8slables.Selector) (res []any, err error) {
	switch gvr {
	case ResourcesGVRMapInstance[PodsResource]:
		var pods []*corev1.Pod
		var err error
		// nolint:revive // TODO
		if tenant == aisTypes.AISSystemAdminTenantName && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
			// 暂时不统计系统占用 （由于 Pod 过多有些性能因素而且需要考虑 kubebrain 部分）
		} else if tenant == aisTypes.AISSystemAdminTenantName || ns == "" {
			pods, err = cm.podLister.List(labelSelector)
		} else {
			pods, err = cm.podLister.Pods(ns).List(labelSelector)
		}
		if err != nil {
			log.WithError(err).Error("[Quota] list pods")
			return nil, err
		}
		for index := range pods {
			res = append(res, pods[index])
		}
	case ResourcesGVRMapInstance[PersistentVolumeClaimsResource]:
		var pvcs []*corev1.PersistentVolumeClaim
		var err error
		// nolint:revive // TODO
		if tenant == aisTypes.AISSystemAdminTenantName && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
			// 暂时不统计系统占用 （由于 PVC 过多有些性能因素而且需要考虑 kubebrain 部分）
		} else if tenant == aisTypes.AISSystemAdminTenantName || ns == "" {
			pvcs, err = cm.pvcLister.List(labelSelector)
		} else {
			pvcs, err = cm.pvcLister.PersistentVolumeClaims(ns).List(labelSelector)
		}
		if err != nil {
			log.WithError(err).Error("[Quota] list pvcs")
			return nil, err
		}
		for index := range pvcs {
			res = append(res, pvcs[index])
		}
	default:
		return nil, fmt.Errorf("invalid gvr:%+v", gvr)
	}
	return
}

func getOrSetUser(users map[string]*UserCharged, userName string) *UserCharged {
	if userName == "" {
		return nil
	}
	user, exist := users[userName]
	if !exist {
		user = &UserCharged{
			ChargedQuota: quotaTypes.NilQuota(),
			UsedQuota:    quotaTypes.NilQuota(),
			UsedResource: quotaTypes.NilQuota(),
		}
		users[userName] = user
	}
	return user
}

func getTenantUsers(tenant string, resync bool) map[string]*UserCharged {
	ret := make(map[string]*UserCharged)
	if !resync {
		return ret
	}

	ctx := ginlib.NewMockGinContext()
	if _, users, err := ListUserDetails(ctx, tenant, "", nil, nil); err != nil {
		for _, u := range users {
			getOrSetUser(ret, u.UserName)
		}
	}
	return ret
}
