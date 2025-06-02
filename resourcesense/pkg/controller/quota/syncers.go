package quota

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	coreListers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	authClientv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/components/pkg/controller/util"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

type UserCharged struct {
	ChargedQuota quotaTypes.Quota
	UsedQuota    quotaTypes.Quota
	UsedResource quotaTypes.Quota
}

type Syncer interface {
	Run()
}

type NamespaceSyncer struct {
	authClient      authClientv1.Client
	clientset       *kubernetes.Clientset
	namespaceLister coreListers.NamespaceLister
}

func NewNamespaceSyncer(cm *ControllerManager, authClient authClientv1.Client, clientset *kubernetes.Clientset) *NamespaceSyncer {
	return &NamespaceSyncer{
		authClient:      authClient,
		clientset:       clientset,
		namespaceLister: cm.namespaceLister,
	}
}

// Run 周期性保证namespace具有quota.aiservice.brainpp.cn/webhook标签，当ns具有该标签时，pod才会进行quota校验
func (ns *NamespaceSyncer) Run() {
	log.Infof("periodicity sync namespace, with interval: %+v", periodSyncNamespaceInterval)
	if !features.IsQuotaEnabled() {
		log.Info("namespace syncer quota feature isn't enabled!")
		return
	}

	if !features.IsNamespacePerTenant() {
		log.Info("namespace syncer namespace per tenant feature isn't enabled!")
		return
	}

	ticker := time.NewTicker(periodSyncNamespaceInterval)
	defer ticker.Stop()
	for {
		for range ticker.C {
			log.Info("[Quota_loop] periodicity sync namespace running")
			_, tenants, err := ns.authClient.ListTenant(nil)
			if err != nil {
				log.WithError(err).Error("failed to list auth tenants")
				continue
			}
			for _, tenant := range tenants {
				projects, err := ns.authClient.ListProjectByTenant(nil, tenant.ID)
				if err != nil {
					log.WithError(err).Errorf("namespace syncer failed to list auth projects: %s", tenant.ID)
					continue
				}
				for _, project := range projects {
					namespace := features.GetProjectWorkloadNamespace(project.ID)
					kns, err := ns.namespaceLister.Get(namespace)
					if err != nil {
						log.WithError(err).Errorf("namespace syncer failed to get namespace: %s/%s", tenant.ID, project.Name)
						continue
					}
					if _, ok := kns.Labels[aisTypes.AISQuotaAdmissionNSLabel]; !ok {
						kns.Labels[aisTypes.AISQuotaAdmissionNSLabel] = aisTypes.QuotaEnabled
						if _, err := ns.clientset.CoreV1().Namespaces().Update(context.TODO(), kns, metav1.UpdateOptions{}); err != nil {
							log.WithError(err).Errorf("namespace syncer failed to update namespace: %s", kns.Name)
							continue
						}
						log.Infof("namespace syncer update namespace: %s quota label successed", kns.Name)
					}
				}
			}
		}
	}
}

type TenantSyncer struct {
	authClient       authClientv1.Client
	availableTenants map[string]bool
}

func NewTenantSyncer(authClient authClientv1.Client) *TenantSyncer {
	return &TenantSyncer{
		authClient:       authClient,
		availableTenants: make(map[string]bool),
	}
}

func (ts *TenantSyncer) LoadAvailableTenants(gc *ginlib.GinContext) error {
	_, tenants, err := ListTenantDetails(gc, "", nil, nil, true)
	if err != nil {
		log.WithError(err).Error("failed to list tenants!")
		return err
	}
	for _, t := range tenants {
		ts.availableTenants[t.TenantID] = false
	}
	return nil
}

// Run 周期同步 auth tenants 到 TenantDetail 表(为保证实时性可能需要auth主动创建)
func (ts *TenantSyncer) Run() {
	gc := ginlib.NewMockGinContext()
	ts.LoadAvailableTenants(gc)

	log.Infof("periodicity sync tenants, with interval: %+v", periodSyncTenantInterval)
	_, tenants, err := ts.authClient.ListTenant(nil)
	if err != nil {
		log.WithError(err).Error("failed to list auth tenants")
	} else {
		ts.SyncToDB(gc, tenants...)
	}

	ticker := time.NewTicker(periodSyncTenantInterval)
	defer ticker.Stop()
	for {
		for range ticker.C {
			log.Info("[Quota_loop] periodicity sync tenant running")
			gc := ginlib.NewMockGinContext()
			_, tenants, err := ts.authClient.ListTenant(nil)
			if err != nil {
				log.WithError(err).Error("failed to list auth tenants")
				continue
			}
			ts.SyncToDB(gc, tenants...)
		}
	}
}

func (ts *TenantSyncer) SyncToDB(gc *ginlib.GinContext, tenants ...*authTypes.Tenant) error {
	for _, tenant := range tenants {
		ts.availableTenants[tenant.ID] = true
		if err := CreateOrUpdateTenantDetail(gc, tenant); err != nil { // 这里可以考虑使用 availableTenants 作比较然后判断是否更新
			log.WithError(err).Errorf("failed to upsert tenant detail: %s", tenant.Name)
		}
	}

	systemadmin := authTypes.Tenant{
		ID:          aisTypes.AISSystemAdminTenantName,
		Name:        "系统占用",
		DisplayName: "系统占用",
		TenantType:  authTypes.TenantType(aisTypes.AISSystemAdminTenantName),
		CreatedAt:   time.Now().Unix(),
	}
	ts.availableTenants[systemadmin.ID] = true

	if err := CreateOrUpdateTenantDetail(gc, &systemadmin); err != nil {
		log.WithError(err).Errorf("failed to upsert systemadmin tenant detail: %s", systemadmin.Name)
	}

	// 处理在两次轮询期间删除的 tenants
	for t, available := range ts.availableTenants {
		if t != aisTypes.AISSystemAdminTenantName && !available {
			if err := DeleteTenantDetail(gc, t); err != nil {
				log.WithError(err).Errorf("failed to delete unavaiable tenant detail: %s", t)
			}
		}
	}

	// 重置为不可用，下一次轮询如果 auth 没有返回则认为被删除了
	for t := range ts.availableTenants {
		ts.availableTenants[t] = false
	}
	return nil
}

type UserSyncer struct {
	authClient authClientv1.Client
}

func NewUserSyncer(authClient authClientv1.Client) *UserSyncer {
	return &UserSyncer{
		authClient: authClient,
	}
}

// Run 周期同步 auth users 到 UserDetail 表(为保证实时性可能需要auth主动创建)
func (ts *UserSyncer) Run() {
	gc := ginlib.NewMockGinContext()

	log.Infof("[Quota] Periodicity sync users, with interval: %+v", pderiodSyncUserInterval)
	page := 1
	pageSize := 9000
	_, users, err := ts.authClient.ListUser(nil, "createdAt", "desc", "", "", page, pageSize)
	if err != nil {
		log.WithError(err).Error("failed to list auth users")
	} else {
		ts.SyncToDB(gc, users...)
	}

	ticker := time.NewTicker(pderiodSyncUserInterval)
	defer ticker.Stop()
	for {
		for range ticker.C {
			log.Info("[Quota_loop] periodicity sync user running")
			gc := ginlib.NewMockGinContext()
			_, users, err := ts.authClient.ListUser(nil, "createdAt", "desc", "", "", page, pageSize)
			if err != nil {
				log.WithError(err).Error("[Quota] failed to list auth users")
				continue
			}
			ts.SyncToDB(gc, users...)
		}
	}
}

func (ts *UserSyncer) SyncToDB(gc *ginlib.GinContext, users ...*authTypes.User) error {
	for _, user := range users {
		if err := CreateOrUpdateUserDetail(gc, user); err != nil {
			log.WithError(err).Errorf("[Quota] failed to upsert tenant detail: %s", user.Name)
		}
	}
	return nil
}

type PodSyncer struct {
	cm            *ControllerManager
	queue         workqueue.RateLimitingInterface
	eventRecorder *EventRecorder
}

func (ps *PodSyncer) Run() {
	log.Infof("[Quota] start to reconcile tenants")
	for ps.processNextEvent() {
	}
}

func (ps *PodSyncer) Handles() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    ps.handleAddEvent,
		UpdateFunc: ps.handleUpdateEvent,
		DeleteFunc: ps.handleDeleteEvent,
	}
}

func (ps *PodSyncer) handleAddEvent(obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("[Quota] object is not a pod %+v", pod))
		return
	}

	if !IsPodQuotaEnabled(pod) && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		log.Warnf("[Quota_podReconcile] ignore pod: %s", pod.Name)
		return
	}

	ps.eventRecorder.RecordPodEvent(pod, quotaTypes.ResourceEventCreate)
	ps.enqueue(nil, pod, cache.Added)
}

func (ps *PodSyncer) handleUpdateEvent(old, cur any) {
	curPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}
	if curPod.DeletionTimestamp != nil {
		log.Infof("[Quota] pod %s is terminating", curPod.GetName())
	}

	if !IsPodQuotaEnabled(curPod) && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		return
	}
	event := FirePodEvent(oldPod.Status.Phase, curPod.Status.Phase)
	if event != "" {
		ps.eventRecorder.RecordPodEvent(curPod, event)
	}

	// 这里比较 Pod 资源占用是否有变更，没有的话则不触发统计优化性能
	if IsPodQuotaConsumeChanged(oldPod, curPod) {
		ps.enqueue(oldPod, curPod, cache.Updated)
	}
}

func (ps *PodSyncer) handleDeleteEvent(obj any) {
	pod, ok := util.ObjOnDelete(obj).(*corev1.Pod)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("[Quota] tombstone contained object that is not a pod %+v", obj))
		return
	}

	if !IsPodQuotaEnabled(pod) && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		return
	}

	ps.eventRecorder.RecordPodEvent(pod, quotaTypes.ResourceEventDelete)
	ps.enqueue(nil, pod, cache.Deleted)
}

func (ps *PodSyncer) enqueue(old, cur *corev1.Pod, action cache.DeltaType) {
	if cur == nil {
		return
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(cur)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	log.Debugf("[Quota_podReconcile] enqueue pod %s %s event", key, action)
	tenant := getObjTenant(cur)
	ps.queue.Add(SyncerQueuedKey{
		Key:    key,
		Tenant: tenant,
	})
}

func (ps *PodSyncer) processNextEvent() bool {
	key, quit := ps.queue.Get()
	if quit {
		return false
	}
	defer ps.queue.Done(key)
	err := ps.cm.EnqueueWorkload(key.(SyncerQueuedKey), PodsResource)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("%v failed with : %v", key, err))
		ps.queue.AddRateLimited(key)
	} else {
		ps.queue.Forget(key)
	}
	return true
}

type PVCSyncer struct {
	cm            *ControllerManager
	queue         workqueue.RateLimitingInterface
	eventRecorder *EventRecorder
}

func (ps *PVCSyncer) Run() {
	log.Infof("[Quota] start to reconcile pvcs")
	for ps.processNextEvent() {
	}
}

func (ps *PVCSyncer) Handles() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    ps.handleAddEvent,
		UpdateFunc: ps.handleUpdateEvent,
		DeleteFunc: ps.handleDeleteEvent,
	}
}

func (ps *PVCSyncer) handleAddEvent(obj any) {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("[Quota] object is not a pvc %+v", pvc))
		return
	}
	if !IsPVCQuotaEnabled(pvc) && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		return
	}
	ps.eventRecorder.RecordPVCEvent(pvc, quotaTypes.ResourceEventCreate)
	ps.enqueue(nil, pvc, cache.Added)
}

func (ps *PVCSyncer) handleUpdateEvent(old, cur any) {
	curPVC := cur.(*corev1.PersistentVolumeClaim)
	oldPVC := old.(*corev1.PersistentVolumeClaim)
	if curPVC.ResourceVersion == oldPVC.ResourceVersion {
		return
	}
	if !IsPVCQuotaEnabled(curPVC) && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		return
	}

	if curPVC.DeletionTimestamp != nil {
		log.Infof("[Quota] pvc %s is terminating", curPVC.GetName())
	}

	event := FirePVCEvent(oldPVC.Status.Phase, curPVC.Status.Phase)
	if event != "" {
		ps.eventRecorder.RecordPVCEvent(curPVC, event)
	}
	ps.enqueue(oldPVC, curPVC, cache.Updated)
}

func (ps *PVCSyncer) handleDeleteEvent(obj any) {
	pvc, ok := util.ObjOnDelete(obj).(*corev1.PersistentVolumeClaim)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("[Quota] tombstone contained object that is not a tenant %+v", obj))
		return
	}
	if !IsPVCQuotaEnabled(pvc) && consts.ConfigMap.QuotaConfig.IgnoreSystemUsed {
		return
	}
	ps.eventRecorder.RecordPVCEvent(pvc, quotaTypes.ResourceEventDelete)
	ps.enqueue(nil, pvc, cache.Deleted)
}

func (ps *PVCSyncer) enqueue(old, cur *corev1.PersistentVolumeClaim, action cache.DeltaType) {
	if cur == nil {
		return
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(cur)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	log.Debugf("[Quota] enqueue pvc %s %s event", key, action)
	tenant := getObjTenant(cur)
	ps.queue.Add(SyncerQueuedKey{
		Key:    key,
		Tenant: tenant,
	})
}

func (ps *PVCSyncer) processNextEvent() bool {
	key, quit := ps.queue.Get()
	if quit {
		return false
	}
	defer ps.queue.Done(key)
	err := ps.cm.EnqueueWorkload(key.(SyncerQueuedKey), PersistentVolumeClaimsResource)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("[Quota] %v failed with : %v", key, err))
		ps.queue.AddRateLimited(key)
	} else {
		ps.queue.Forget(key)
	}
	return true
}
