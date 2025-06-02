package quota

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreInformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	authv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
)

const (
	QuotaControllerName = "ais-quota-controller"
)

const (
	periodSyncNamespaceInterval = time.Minute * 60 // 同步 namespace 的 quota label
	periodSyncTenantInterval    = time.Minute * 1
	pderiodSyncUserInterval     = time.Second * 10
	defaultResyncPeriod         = time.Minute * 5
	defaultResyncSince          = time.Hour * 24
	defaultResyncAllPeriod      = time.Minute * 30
)

type TenantQueuedKey struct {
	Namespace string // 一个 tenant 可能对应多个 namepsace，这里为空
	Tenant    string
	Kind      ResourceKind
	Resync    bool // 是否是全量同步 (会重新统计该租户下的所有用户配额占用，否则只会更新当前有资源的用户)
}

type SyncerQueuedKey struct {
	Key    string // namespace/name
	Tenant string
}

type ControllerManager struct {
	podLister corelister.PodLister
	podSynced cache.InformerSynced

	pvcLister corelister.PersistentVolumeClaimLister
	pvcSynced cache.InformerSynced

	namespaceLister corelister.NamespaceLister
	namespaceSynced cache.InformerSynced

	keyQueue workqueue.RateLimitingInterface // 用于同步 tenant quota

	// FIXME: 目前只用于 resync 逻辑，后续可以基于 cache 优化更新逻辑
	cache               map[TenantQueuedKey]time.Time
	cacheLock           *sync.Mutex
	resyncPeriod        time.Duration
	resyncSinceDuration time.Duration
	lastResyncAll       time.Time     // 从 DB 全量同步一次
	resyncAllPeriod     time.Duration // 从 DB 全量同步一次

	syncers []Syncer
}

func NewControllerManager(
	podInformer coreInformers.PodInformer,
	pvcInformer coreInformers.PersistentVolumeClaimInformer,
	namespaceInformer coreInformers.NamespaceInformer,
	kubeclient *clientset.Clientset,
	authClient authv1.Client,
) *ControllerManager {
	tenantQueue := workqueue.NewNamedRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	), "tenant_quota_queue")
	controller := &ControllerManager{
		podLister:           podInformer.Lister(),
		podSynced:           podInformer.Informer().HasSynced,
		pvcLister:           pvcInformer.Lister(),
		pvcSynced:           pvcInformer.Informer().HasSynced,
		namespaceLister:     namespaceInformer.Lister(),
		namespaceSynced:     namespaceInformer.Informer().HasSynced,
		keyQueue:            tenantQueue,
		cache:               make(map[TenantQueuedKey]time.Time),
		cacheLock:           &sync.Mutex{},
		resyncPeriod:        defaultResyncPeriod,
		resyncSinceDuration: defaultResyncSince,
		resyncAllPeriod:     defaultResyncAllPeriod,
		lastResyncAll:       time.Now().Add(-(defaultResyncAllPeriod + time.Minute*1)),
	}

	podSyncer := &PodSyncer{
		cm:            controller,
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pod_syncer_queue"),
		eventRecorder: NewEventRecorder(controller),
	}
	pvcSyncer := &PVCSyncer{
		cm:            controller,
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pvc_syncer_queue"),
		eventRecorder: NewEventRecorder(controller),
	}
	namespaceSyncer := NewNamespaceSyncer(
		controller,
		authClient,
		kubeclient,
	)
	userSyncer := NewUserSyncer(authClient)
	tenantSyncer := NewTenantSyncer(authClient)

	controller.syncers = append(controller.syncers, podSyncer, pvcSyncer, userSyncer, tenantSyncer, namespaceSyncer)

	podInformer.Informer().AddEventHandler(podSyncer.Handles())
	pvcInformer.Informer().AddEventHandler(pvcSyncer.Handles())
	return controller
}

func NewAndRunControllerManager(
	ctx context.Context,
	podInformer coreInformers.PodInformer,
	pvcInformer coreInformers.PersistentVolumeClaimInformer,
	namespaceInformer coreInformers.NamespaceInformer,
	kubeclient *clientset.Clientset,
	authClient authv1.Client,
) error {
	run := func(ctx context.Context) {
		cm := NewControllerManager(podInformer, pvcInformer, namespaceInformer, kubeclient, authClient)
		if err := cm.Run(ctx); err != nil {
			log.WithError(err).Error("Error running controller")
		}
	}

	cmLeader, err := aisUtils.GetLeaderElector(kubeclient, QuotaControllerName, "", run)
	if err != nil {
		log.Errorf("failed to GetLeaderElector")
		return err
	}

	go cmLeader.Run(ctx)

	return nil
}

// Run 有一个定期的 reconcile 逻辑， 定期全量修正 tenant/user 的配额占用，集群的总资源占用
func (cm *ControllerManager) Run(ctx context.Context) error {
	defer utilruntime.HandleCrash()
	defer cm.keyQueue.ShutDown()

	log.Info("[Quota] Waiting for quota controller caches to be synced")
	if ok := cache.WaitForCacheSync(ctx.Done(), cm.podSynced, cm.pvcSynced, cm.namespaceSynced); !ok {
		log.Errorf("[Quota] failed to wait for caches to be synced")
	}
	log.Info("[Quota] Quota controller caches init successfully")

	log.Info("[Quota] Starting syncers...")
	for _, syncer := range cm.syncers {
		go syncer.Run()
	}

	log.Info("[Quota] Starting reconcile...")
	cm.run(3, ctx.Done())

	<-ctx.Done()
	log.Info("[Quota] Shutdown reconcile...")

	return nil
}

func (cm *ControllerManager) EnqueueWorkload(sqk SyncerQueuedKey, resourceKind ResourceKind) error {
	now := time.Now()
	key := TenantQueuedKey{
		Namespace: "",
		Tenant:    sqk.Tenant,
		Kind:      resourceKind,
	}
	cm.cacheLock.Lock()
	defer cm.cacheLock.Unlock()
	log.Debugf("[Quota_reconcile] enqueue workload: %+v", key)
	cm.cache[key] = now
	cm.keyQueue.Add(key)
	return nil
}

func (cm *ControllerManager) run(workers int, stopCh <-chan struct{}) {
	log.Infof("[Quota] start to reconcile tenant quotas...")
	for i := 0; i < workers; i++ {
		go wait.Until(cm.runWorker, time.Second, stopCh)
	}
	go wait.Until(func() { cm.resync() }, cm.resyncPeriod, stopCh)
}

func (cm *ControllerManager) runWorker() {
	for cm.processNextWorkItem() {
	}
}

func (cm *ControllerManager) processNextWorkItem() bool {
	obj, shutdown := cm.keyQueue.Get()
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer cm.keyQueue.Done(obj)
		if err := cm.syncHandler(obj.(TenantQueuedKey)); err != nil {
			cm.keyQueue.AddRateLimited(obj)
			return fmt.Errorf("error syncing '%v': %s, requeuing", obj, err.Error())
		}

		cm.keyQueue.Forget(obj)
		log.Info("Successfully synced", "resourceName", obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(fmt.Errorf("%v failed with : %v", obj, err))
		return true
	}

	return true
}

func (cm *ControllerManager) syncHandler(key TenantQueuedKey) error {
	return reconcileObjs(cm, ResourcesGVRMapInstance[key.Kind], key.Namespace, key.Tenant, key.Resync)
}

func (cm *ControllerManager) resync() {
	now := time.Now()
	log.Infof("[Quota_resyncLoop] quota controller resyncing from cache, total: %d", len(cm.cache))
	cm.cacheLock.Lock()
	for key := range cm.cache {
		if cm.resyncSinceDuration > 0 && now.Sub(cm.cache[key]) > cm.resyncSinceDuration {
			delete(cm.cache, key)
			cm.keyQueue.Add(TenantQueuedKey{
				Tenant:    key.Tenant,
				Namespace: key.Namespace,
				Kind:      key.Kind,
				Resync:    true,
			})
		}
	}
	cm.cacheLock.Unlock()

	log.Infof("---Quota_resyncLoop---: %1fmin, %s, %1fmin", cm.resyncAllPeriod.Minutes(), cm.lastResyncAll.String(), cm.resyncPeriod.Minutes())
	if cm.resyncAllPeriod > 0 && now.Sub(cm.lastResyncAll) > cm.resyncAllPeriod {
		log.Infof("[Quota_resyncLoop] quota controller resyncing all")
		if tenants, err := ListAllTenantDetails(); err == nil {
			for _, t := range tenants {
				log.Infof("[Quota_resyncLoop] quota controller resyncing tenant: %s", t.TenantID)
				cm.keyQueue.Add(TenantQueuedKey{
					Tenant:    t.TenantID,
					Namespace: "",
					Kind:      PodsResource,
					Resync:    true,
				})
				cm.keyQueue.Add(TenantQueuedKey{
					Tenant:    t.TenantID,
					Namespace: "",
					Kind:      PersistentVolumeClaimsResource,
					Resync:    true,
				})
			}
			cm.lastResyncAll = time.Now()
		}
	}
}
