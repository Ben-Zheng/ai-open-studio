package quota

import (
	"context"
	"time"

	"github.com/apex/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	corev1 "k8s.io/api/core/v1"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

const (
	ColNameResourceEvents = "resource_events"
)

// EventRecorder 记录资源事件到 DB
type EventRecorder struct {
	cm *ControllerManager
}

func NewEventRecorder(cm *ControllerManager) *EventRecorder {
	return &EventRecorder{cm: cm}
}

// RecordPodEvent 这里可能会因为 informer 重试机制导致事件重复, 可以考虑 insert 前检查一下
func (er *EventRecorder) RecordPodEvent(pod *corev1.Pod, action quota.ResourceEventType) {
	var err error
	var rareGPUType map[string]bool
	if features.IsQuotaSupportGPUGroupEnabled() {
		if rareGPUType, err = GetRareGPUTypeMap(ginlib.NewMockGinContext()); err != nil {
			log.WithError(err).Error("failed to GetRareGPUTypeMap")
		}
	}

	labels := pod.GetLabels()
	annotations := pod.GetAnnotations()
	chargedQuota := quota.NilQuota()
	IncreasePodQuota(chargedQuota, pod, rareGPUType)
	log.Infof("[Quota_podReconcile] pod: %s %s, charge quota: %v", action, pod.Name, chargedQuota)

	event := quota.ResourceEvent{
		EventType:          action,
		ResourceType:       quota.ResourcePodResource,
		ResourceName:       pod.Name,
		CustomResourceType: annotations[aisTypes.AISResourceType],
		CustomResourceName: annotations[aisTypes.AISResourceName],
		CustomResourceID:   annotations[aisTypes.AISResourceID],
		ChargedQuota:       chargedQuota,
		TenantName:         labels[aisTypes.AISTenantName],
		ProjectName:        labels[aisTypes.AISProjectName],
		Creator:            labels[aisTypes.AISUserName],
		CreatorID:          labels[aisTypes.AISUserID],
		CreatedAt:          time.Now().Unix(),
	}

	if err := InsertResourceEvent(ginlib.NewMockGinContext(), &event); err != nil {
		log.WithError(err).Error("failed to insert resource event")
		return
	}
}

func (er *EventRecorder) RecordPVCEvent(pvc *corev1.PersistentVolumeClaim, action quota.ResourceEventType) {
	// workspace 产生的 pvc 不做记录
	if pvc.GetAnnotations()[aisTypes.AISResourceType] == string(aisTypes.AIServiceTypeWorkspace) {
		return
	}

	labels := pvc.GetLabels()
	annotations := pvc.GetAnnotations()
	chargedQuota := quota.NilQuota()
	IncreasePVCQuota(chargedQuota, pvc)
	event := quota.ResourceEvent{
		EventType:          action,
		ResourceType:       quota.ResourcePVCResource,
		ResourceName:       pvc.Name,
		CustomResourceType: annotations[aisTypes.AISResourceType],
		CustomResourceName: annotations[aisTypes.AISResourceName],
		CustomResourceID:   annotations[aisTypes.AISResourceID],
		ChargedQuota:       chargedQuota,
		TenantName:         labels[aisTypes.AISTenantName],
		ProjectName:        labels[aisTypes.AISProjectName],
		Creator:            labels[aisTypes.AISUserName],
		CreatorID:          labels[aisTypes.AISUserID],
		CreatedAt:          time.Now().Unix(),
	}
	if err := InsertResourceEvent(ginlib.NewMockGinContext(), &event); err != nil {
		return
	}
}

func InsertResourceEventsV2(ctx *ginlib.GinContext, events []interface{}) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		log.Error("failed to getMongoClient")
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameResourceEvents)

	if _, err := col.InsertMany(context.TODO(), events); err != nil {
		log.Error("failed to InsertMany")
		return err
	}

	return nil
}

func InsertResourceEvents(ctx *ginlib.GinContext, events []mongo.WriteModel) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		log.Error("failed to getMongoClient")
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameResourceEvents)

	if _, err := col.BulkWrite(context.TODO(), events, options.BulkWrite().SetOrdered(false)); err != nil {
		log.Error("failed to InsertMany")
		return err
	}

	return nil
}

func InsertResourceEvent(ctx *ginlib.GinContext, event *quota.ResourceEvent) error {
	event.ToDB()
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameResourceEvents)

	filter := bson.M{
		"eventType":    event.EventType,
		"resourceType": event.ResourceType,
		"resourceName": event.ResourceName,
	}

	logger := log.WithFields(log.Fields{
		"event": event,
	})

	// 设置团队配额的时候，tenantdetail 一定是已经先同步好的
	cnt, err := col.CountDocuments(context.TODO(), filter)
	if err != nil {
		logger.WithError(err).Error("count resource event failed")
		return err
	}

	if cnt > 0 {
		// 事件重复记录
		return nil
	}

	if _, err := col.InsertOne(context.TODO(), event); err != nil {
		logger.WithError(err).Error("insert resource event failed")
		return err
	}
	return nil
}

type ListResourceEventRequest struct {
	TenantID            string
	Creator             string
	CustomResourceTypes []string
	StartTime           int64
	EndTime             int64
}

func ListResourceEvents(ctx *ginlib.GinContext, param *ListResourceEventRequest, pagination *ginlib.Pagination, sorter *ginlib.Sort) (int64, []*quota.ResourceEvent, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection(ColNameResourceEvents)

	filter := bson.M{
		"tenantName": param.TenantID,
		"eventType": bson.M{
			"$nin": []string{
				string(quota.ResourceEventStart),
				string(quota.ResourceEventFinish),
				string(quota.ResourceEventDelete),
			},
		},
	}
	if param.Creator != "" {
		filter["creator"] = bson.M{
			"$regex": primitive.Regex{
				Pattern: param.Creator,
				Options: "i",
			},
		}
	}

	if param.StartTime != 0 && param.EndTime != 0 {
		filter["createdAt"] = bson.M{
			"$gt": param.StartTime,
			"$lt": param.EndTime,
		}
	} else if param.StartTime != 0 {
		filter["createdAt"] = bson.M{
			"$gt": param.StartTime,
		}
	} else if param.EndTime != 0 {
		filter["createdAt"] = bson.M{
			"$lt": param.EndTime,
		}
	}

	if len(param.CustomResourceTypes) > 0 {
		filter["customResourceType"] = bson.M{
			"$in": param.CustomResourceTypes,
		}
	}

	findOptions := options.Find()
	findOptions.SetSkip(pagination.Skip)
	findOptions.SetLimit(pagination.Limit)

	if sorter == nil {
		sorter = &ginlib.Sort{
			By:    "createdAt",
			Order: -1,
		}
	}
	findOptions.SetSort(bson.D{{
		Key:   sorter.By,
		Value: sorter.Order,
	}})
	cursor, err := col.Find(context.TODO(), filter, findOptions)
	if err != nil {
		return 0, nil, err
	}
	var rs []*quota.ResourceEvent
	if err := cursor.All(context.TODO(), &rs); err != nil {
		return 0, nil, err
	}
	total, err := col.CountDocuments(context.TODO(), filter)
	if err != nil {
		return 0, nil, err
	}

	for i := range rs {
		rs[i].FromDB()
	}
	return total, rs, nil
}

func FirePodEvent(oldState, newState corev1.PodPhase) quota.ResourceEventType {
	if oldState != corev1.PodRunning && newState == corev1.PodRunning {
		return quota.ResourceEventStart
	}

	if (oldState != corev1.PodSucceeded && oldState != corev1.PodFailed) &&
		(newState == corev1.PodSucceeded || newState == corev1.PodFailed) {
		return quota.ResourceEventFinish
	}
	return ""
}

// FirePVCEvent 暂时忽略 PVC Bound 之类的细节问题
func FirePVCEvent(oldState, newState corev1.PersistentVolumeClaimPhase) quota.ResourceEventType {
	return ""
}
