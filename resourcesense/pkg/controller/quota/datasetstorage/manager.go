package datasetstorage

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"

	datasetTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/types/dataset"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/utils"
)

type ControllerManager struct {
	datasourceIDQueue workqueue.RateLimitingInterface
	datasetQueue      workqueue.RateLimitingInterface
	cacheLock         *sync.Mutex
	kubeClient        *clientset.Clientset
}

var once sync.Once
var controllerManager *ControllerManager

const (
	reSyncPeriodTime           = 409 * time.Second
	datasetQuotaControllerName = "ais-dataset-quota-controller"
)

func GetControllerManager(kubeClient *clientset.Clientset) *ControllerManager {
	once.Do(func() {
		controllerManager = &ControllerManager{
			datasourceIDQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "dataset_source_queue"),
			datasetQueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "dataset_queue"),
			cacheLock:         &sync.Mutex{},
			kubeClient:        kubeClient,
		}
	},
	)

	return controllerManager
}

func (c *ControllerManager) Run(ctx context.Context) {
	go c.runDatasource(1, ctx.Done())
	go c.runDataset(1, ctx.Done())

	run := func(ctx context.Context) {
		log.Infof("sync dataset storage period")
		wait.Until(func() { c.reSync() }, reSyncPeriodTime, ctx.Done())
	}

	cmLeader, err := aisUtils.GetLeaderElector(c.kubeClient, datasetQuotaControllerName, "", run)
	if err != nil {
		log.Errorf("failed to GetLeaderElector")
		panic(err)
	}
	go cmLeader.Run(ctx)

	<-ctx.Done()
	log.Info("[Quota_dataset] Shutdown dataset reconcile...")
}

func (c *ControllerManager) reSync() {
	tenants, err := quota.ListAllTenantDetails()
	if err != nil {
		log.WithError(err).Error("failed to ListAllTenantDetails")
		return
	}
	for i := range tenants {
		log.Infof("[Quota_dataset] reSync all tenants: %s", tenants[i].TenantID)
		c.EnqueueDataset(&DatasetQueue{
			Tenant:     tenants[i].TenantID,
			ActionType: quotaTypes.ActionTypeReSync,
		})
	}
}

func (c *ControllerManager) runDatasource(workers int, stopCh <-chan struct{}) {
	for i := 0; i < workers; i++ {
		go wait.Until(c.datasourceWorker, time.Second*5, stopCh)
	}
}

func (c *ControllerManager) runDataset(workers int, stopCh <-chan struct{}) {
	for i := 0; i < workers; i++ {
		go wait.Until(c.datasetWorker, time.Second*11, stopCh)
	}
}

func (c *ControllerManager) datasourceWorker() {
	log.Infof("[Quota_dataset] start to sync datasource")
	for c.processNextDatasourceEvent() {
	}
}
func (c *ControllerManager) datasetWorker() {
	log.Infof("[Quota_dataset] start to sync dataset")
	for c.processNextDatasetEvent() {
	}
}

func (c *ControllerManager) processNextDatasetEvent() bool {
	obj, quit := c.datasetQueue.Get()
	if quit {
		return false
	}

	err := func(obj interface{}) error {
		defer c.datasetQueue.Done(obj)
		dsKey, ok := obj.(*DatasetQueue)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("invalid key: %+v, forget it", obj))
			c.datasetQueue.Forget(obj)
			return fmt.Errorf("invalid key: %+v", obj)
		}
		if err := c.syncHandler(dsKey); err != nil {
			c.datasetQueue.AddRateLimited(obj)
			return fmt.Errorf("dataset error syncing %+v, error: %s", obj, err.Error())
		}
		c.datasetQueue.Forget(obj)
		log.Infof("[Quota_dataset] dataset successfully synced resourceName: %+v", obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(fmt.Errorf("%v failed with : %v", obj, err))
		return true
	}

	return true
}

func (c *ControllerManager) syncHandler(key *DatasetQueue) error {
	// todo 加锁
	logger := log.WithFields(log.Fields{
		"tenant":     key.Tenant,
		"datasetID":  key.DatasetID,
		"revisionID": key.RevisionID,
	})

	switch key.ActionType {
	case quotaTypes.ActionTypeCharge:
		// 判断数据集所有数据源是否处于终态
		gc := ginlib.NewMockGinContext()
		gc.SetAuthProjectID(key.Project)
		gc.SetTenantName(key.Tenant)
		datasetID, _ := primitive.ObjectIDFromHex(key.DatasetID)
		revisionID, _ := primitive.ObjectIDFromHex(key.RevisionID)
		resp, err := consts.DatahubClient.GetRevision(gc, datasetID, revisionID, aisTypes.ProjectLevel)
		if err != nil {
			logger.Info("failed to GetDatasetState")
			return err
		}
		// 如果未处于终态则不做更新，等待最后一个终态数据源进入后重算
		if !resp.IsFinalState {
			log.Infof("[Quota_dataset] dataset is still in not in final state, waiting next key")
			return nil
		}
	case quotaTypes.ActionTypeFree, quotaTypes.ActionTypeReSync:
	default:
		log.Warningf("invalid actionType: %s, forget it", key.ActionType)
		return nil
	}

	// 如果处于终态或删除操作则重算租户内配额信息
	unlock, err := utils.EtcdLocker(consts.EtcdClient, fmt.Sprintf(quotaTypes.DatasetStorageLock, key.Tenant), quotaTypes.LockTimeout)
	if err != nil {
		logger.WithError(err).Error("failed to EtcdLocker")
		return err
	}
	logger.Infof("[Quota_dataset] get tenant lock")
	defer unlock()

	quotaResp, err := consts.DatahubClient.GetDatasetChargedQuotaInOneTenant(key.Tenant)
	if err != nil {
		log.WithError(err).Error("failed to GetDatasetChargedQuotaInOneTenant")
		return err
	}
	chargedQuota := quotaTypes.BuildQuotaFromData(quotaTypes.QuotaData{types.KubeResourceDatasetStorage: float64(quotaResp.ChargedQuotaBytes)})
	usedQuota := chargedQuota
	usedResource := chargedQuota
	tenantDetail, err := quota.GetTenantDetail(ginlib.NewMockGinContext(), key.Tenant)
	if err != nil {
		log.WithError(err).Error("failed to GetTenantDetail")
		return err
	}
	logger.Infof("[Quota_dataset] overwrite, actural chargedQuota: %+v, database chargedQuota: %+v", chargedQuota, tenantDetail.ChargedQuota)
	quota.OverwriteQuota(quota.ResourcesGVRMapInstance[quota.DatasetStorageResource], chargedQuota, usedQuota, usedResource, tenantDetail.ChargedQuota, tenantDetail.UsedQuota, tenantDetail.Used)
	if err = quota.UpdateChargedAndUsedQuota(ginlib.NewMockGinContext(), key.Tenant, chargedQuota, usedQuota, usedResource); err != nil {
		logger.WithError(err).Error("update tenant detail failed")
		return err
	}

	userCharged := make(map[string]*quota.UserCharged)
	for name, quotaBytes := range quotaResp.UserChargedQuota {
		userCharged[name] = &quota.UserCharged{ChargedQuota: quotaTypes.BuildQuotaFromData(quotaTypes.QuotaData{types.KubeResourceDatasetStorage: float64(quotaBytes)})}
		userCharged[name].UsedResource = userCharged[name].ChargedQuota
		userCharged[name].UsedQuota = userCharged[name].ChargedQuota
	}
	for userName, userQuota := range userCharged {
		logger = logger.WithField("user", userName)
		userDetail, err := quota.GetUserDetail(ginlib.NewMockGinContext(), key.Tenant, userName)
		if err != nil {
			logger.WithError(err).Error("failed to GetUserDetail")
			continue
		}
		quota.OverwriteQuota(quota.ResourcesGVRMapInstance[quota.DatasetStorageResource], userQuota.ChargedQuota, userQuota.UsedQuota, userQuota.UsedResource, userDetail.ChargedQuota, userDetail.UsedQuota, userDetail.Used)
		if err = quota.UpdateUserChargedAndUsedQuota(ginlib.NewMockGinContext(), key.Tenant, userName, userQuota.ChargedQuota, userQuota.UsedQuota, userQuota.UsedResource); err != nil {
			logger.WithError(err).WithField("user", userName).Error("update user detail failed")
			continue
		}
	}

	return nil
}

func (c *ControllerManager) processNextDatasourceEvent() bool {
	obj, quit := c.datasourceIDQueue.Get()
	if quit {
		return false
	}

	err := func(obj interface{}) error {
		defer c.datasourceIDQueue.Done(obj)
		dsKey, ok := obj.(DatasourceQueueKey)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("invalied object: %+v", obj))
			c.datasourceIDQueue.Forget(obj)
			return nil
		}

		var err error
		resp := &datasetTypes.DataSource{}
		logger := log.WithField("datasource queue", dsKey)
		if dsKey.DatasourceID != "" {
			resp, err = consts.DatahubClient.GetDatasourceState(dsKey.Tenant, dsKey.Project, dsKey.DatasourceID)
			if err != nil {
				logger.WithError(err).Errorf("failed to GetDatasourceState: %s", dsKey.DatasourceID)
				c.datasourceIDQueue.AddAfter(obj, time.Second*10)
				return err
			}
		}
		switch dsKey.ActionType {
		case quotaTypes.ActionTypeCharge:
			// 判断数据源是否处于终态
			// 如果处于终态则出队，加入数据集更新队列
			// 如果没有处于终态则继续入队
			if !resp.IsFinalState {
				logger.Infof("[Quota_dataset] dataSource still not in final state")
				c.datasourceIDQueue.AddRateLimited(obj)
				return nil
			} else {
				logger.Infof("[Quota_dataset] dataSource successfully to final state")
				c.datasourceIDQueue.Forget(obj)
			}

		case quotaTypes.ActionTypeFree:
			c.datasourceIDQueue.Forget(obj)
		default:
			logger.Warningf("invalid actionType: %s, forget it", dsKey.ActionType)
			c.datasourceIDQueue.Forget(obj)
			return nil
		}

		c.EnqueueDataset(&DatasetQueue{
			DatasetID:  resp.DatasetID.Hex(),
			RevisionID: resp.RevisionID.Hex(),
			Tenant:     dsKey.Tenant,
			Project:    dsKey.Project,
			ActionType: dsKey.ActionType,
		})

		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
	}

	return true
}

func (c *ControllerManager) HandleEvent(gc *ginlib.GinContext, req *quotaTypes.TenantChargedQuotaUpdateReq) {
	if len(req.DatasetStorageQuota) == 0 {
		return
	}

	// 记录事件
	c.recordEvent(gc, req)

	// 保证入队
	for i := range req.DatasetStorageQuota {
		c.EnqueueDatasource(DatasourceQueueKey{
			Tenant:       gc.GetAuthTenantID(),
			Project:      gc.GetAuthProjectID(),
			DatasourceID: req.DatasetStorageQuota[i].DatasourceID,
			ActionType:   req.ActionType,
		})
	}
}

func (c *ControllerManager) recordEvent(gc *ginlib.GinContext, req *quotaTypes.TenantChargedQuotaUpdateReq) {
	eventType := quotaTypes.ResourceEventCreate
	if req.ActionType == quotaTypes.ActionTypeFree {
		eventType = quotaTypes.ResourceEventDelete
	}

	models := make([]mongo.WriteModel, 0, len(req.DatasetStorageQuota))

	for i := range req.DatasetStorageQuota {
		// 如果是删除图片则不记录
		primitive.ObjectIDFromHex(req.DatasetStorageQuota[i].DatasourceID)
		if req.DatasetStorageQuota[i].DatasourceID == "" {
			continue
		}
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.D{
				{Key: "customResourceID", Value: req.DatasetStorageQuota[i].DatasourceID},
				{Key: "eventType", Value: eventType},
			}).SetUpdate(
			bson.M{"$set": bson.M{
				"eventType":          eventType,
				"resourceType":       quotaTypes.ResourceDatasetStorage,
				"resourceName":       req.DatasetStorageQuota[i].DatasourceName,
				"customResourceType": string(aisTypes.ResourceTypeDataset),
				"customResourceName": fmt.Sprintf("%s/%s", req.DatasetName, req.RevisionName),
				"customResourceID":   req.DatasetStorageQuota[i].DatasourceID,
				"chargedQuota":       req.DatasetStorageQuota[i].RequestQuota.ToData().ToMongo(),
				"tenantName":         gc.GetAuthTenantID(),
				"projectName":        gc.GetAuthProjectID(),
				"creator":            gc.GetUserName(),
				"creatorID":          gc.GetUserID(),
				"createdAt":          time.Now().Unix(),
			}},
		).SetUpsert(true))
		log.Infof("[Quota_dataset] record events: %+v", models[i])
	}

	// 记录事件
	if err := quota.InsertResourceEvents(gc, models); err != nil {
		log.WithError(err).Error("failed to InsertResourceEvents")
	}
}

type DatasourceQueueKey struct {
	Tenant       string
	Project      string
	DatasourceID string
	ActionType   quotaTypes.ActionType
}

func (c *ControllerManager) EnqueueDatasource(datasourceQueueKey DatasourceQueueKey) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	log.Infof("[Quota_dataset] enqueue datasource: %+v", datasourceQueueKey)
	c.datasourceIDQueue.Add(datasourceQueueKey)
}

type DatasetQueue struct {
	DatasetID  string
	RevisionID string
	Tenant     string
	Project    string
	ActionType quotaTypes.ActionType
}

func (c *ControllerManager) EnqueueDataset(datasetQueueKey *DatasetQueue) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	log.Infof("[Quota_dataset] enqueue dataset: %+v", datasetQueueKey)
	c.datasetQueue.Add(datasetQueueKey)
}
