package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aistype "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

type markResource struct {
	resource.Resource
	mark bool
}

func Sync(ctx context.Context) {
	interval := 30
	t := time.NewTicker(time.Second * time.Duration(interval))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Infof("shutdown resource sync...")
			return
		case <-t.C:
			log.Infof("[Resource_loop] periodicity sync pod running")

			podList, err := getPodList()
			if err != nil {
				log.WithError(err).Error("failed to getPodList")
				continue
			}

			resourceMap := getResourceMap(podList)
			if err := loadUserInfo(resourceMap, false); err != nil {
				log.WithError(err).Error("failed to loadStorage")
				continue
			}

			if err := updateResource(resourceMap); err != nil {
				log.WithError(err).Error("failed to updateResource")
				continue
			}

			if err := createResource(resourceMap); err != nil {
				log.WithError(err).Error("failed to createResource")
				continue
			}
		}
	}
}

func getPodList() ([]*corev1.Pod, error) {
	var podList []*corev1.Pod
	nodeMap := make(map[string]bool)
	nodes, err := consts.ResourceClient.NodeLister.List(consts.ResourceClient.NodeSelector)
	if err != nil {
		log.WithError(err).Error("get details, list nodes failed")
		return nil, err
	}
	for i := range nodes {
		nodeMap[nodes[i].Name] = true
	}

	rowPodList, err := consts.ResourceClient.PodLister.List(labels.Everything())
	if err != nil {
		log.WithError(err).Error("failed to list pod")
		return nil, err
	}

	for i := range rowPodList {
		if _, ok := nodeMap[rowPodList[i].Spec.NodeName]; ok {
			podList = append(podList, rowPodList[i])
		}
	}

	return podList, nil
}

func createResource(resourceMap map[string]map[string]*markResource) error {
	gc := ginlib.NewMockGinContext()

	for userID := range resourceMap {
		for tenantID := range resourceMap[userID] {
			if !resourceMap[userID][tenantID].mark {
				resourceMap[userID][tenantID].Resource.UUID = uuid.New().String()
				if err := CreateResource(gc, &resourceMap[userID][tenantID].Resource); err != nil {
					log.WithError(err).WithField("userID", userID).WithField("tenantID", tenantID).Error("failed to CreateResource")
					continue
				}
			}
		}
	}

	return nil
}

func updateResource(resourceMap map[string]map[string]*markResource) error {
	page := 1
	pageSize := 100
	gc := ginlib.NewMockGinContext()
	for {
		_, rs, err := ListAllResource(gc, page, pageSize, false)
		if err != nil {
			log.WithError(err).Error("failed to ListAllResource")
			return err
		}

		for index := range rs {
			if rs[index].IsDeleted {
				continue
			}

			var isExist bool
			if _, ok := resourceMap[rs[index].UserID]; ok {
				if _, ok := resourceMap[rs[index].UserID][rs[index].TenantID]; ok {
					isExist = true
				}
			}

			if isExist && !resourceMap[rs[index].UserID][rs[index].TenantID].mark {
				resourceMap[rs[index].UserID][rs[index].TenantID].mark = true

				rs[index].UserName = resourceMap[rs[index].UserID][rs[index].TenantID].UserName
				rs[index].TenantName = resourceMap[rs[index].UserID][rs[index].TenantID].TenantName
				rs[index].TenantType = resourceMap[rs[index].UserID][rs[index].TenantID].TenantType
				rs[index].UsedQuota.CPU = resourceMap[rs[index].UserID][rs[index].TenantID].UsedQuota.CPU
				rs[index].UsedQuota.GPU = resourceMap[rs[index].UserID][rs[index].TenantID].UsedQuota.GPU
				rs[index].UsedQuota.VirtGPU = resourceMap[rs[index].UserID][rs[index].TenantID].UsedQuota.VirtGPU
				rs[index].UsedQuota.Memory = resourceMap[rs[index].UserID][rs[index].TenantID].UsedQuota.Memory
				if rs[index].CreatedAt == 0 || rs[index].CreatedAt > resourceMap[rs[index].UserID][rs[index].TenantID].CreatedAt {
					rs[index].CreatedAt = resourceMap[rs[index].UserID][rs[index].TenantID].CreatedAt
				}
			} else {
				rs[index].IsDeleted = true
				rs[index].UsedQuota = resource.Used{}
			}
			if rs[index].UUID == "" {
				if err := DeleteResource(gc, &rs[index]); err != nil {
					log.WithError(err).Error("failed to UpdateResource")
					continue
				}

				rs[index].UUID = uuid.New().String()
				if err := CreateResource(gc, &rs[index]); err != nil {
					log.WithError(err).Error("failed to UpdateResource")
					continue
				}
			} else if err := UpdateResourceByUUID(gc, &rs[index]); err != nil {
				log.WithError(err).Error("failed to UpdateResource")
				continue
			}
		}

		if len(rs) < pageSize {
			break
		}
		page++
	}

	return nil
}

func getResourceMap(podList []*corev1.Pod) map[string]map[string]*markResource {
	resourceMap := make(map[string]map[string]*markResource)

	for podListIndex := range podList {
		if podList[podListIndex].Status.Phase == corev1.PodSucceeded ||
			podList[podListIndex].Status.Phase == corev1.PodFailed ||
			podList[podListIndex].Spec.NodeName == "" {
			// not scheduled，succeeded, failed: no resource
			continue
		}

		aisUserName := helpDefaultEmpty("", false)
		aisTenantName := helpDefaultEmpty("", false)
		aisUserID := helpDefaultEmpty(podList[podListIndex].Labels[aistype.AISUserID], true)
		aisTenantID := helpDefaultEmpty(podList[podListIndex].Labels[aistype.AISTenantID], true)

		if _, ok := resourceMap[aisUserID]; !ok {
			tempMap := make(map[string]*markResource)
			resourceMap[aisUserID] = tempMap
		}
		if _, ok := resourceMap[aisUserID][aisTenantID]; !ok {
			mr := markResource{
				Resource: resource.Resource{
					UserID:     aisUserID,
					TenantID:   aisTenantID,
					UserName:   aisUserName,
					TenantName: aisTenantName,
					CreatedAt:  time.Now().Unix(),
					TenantType: consts.TenantTypeOther,
				},
			}
			resourceMap[aisUserID][aisTenantID] = &mr
		}

		containers := append(podList[podListIndex].Spec.Containers, podList[podListIndex].Spec.InitContainers...)
		for containersIndex := range containers {
			requestCPUCores := float64(containers[containersIndex].Resources.Requests.Cpu().MilliValue()) / 1000
			requestMemoryBytes := float64(containers[containersIndex].Resources.Requests.Memory().MilliValue()) / 1000
			var limitGPU float64
			if val, ok := containers[containersIndex].Resources.Requests[types.KubeResourceGPU]; ok {
				limitGPU = float64(val.MilliValue()) / 1000
			}
			var limitVirtGPU float64
			if val, ok := containers[containersIndex].Resources.Requests[types.KubeResourceVirtGPU]; ok {
				limitVirtGPU = float64(val.MilliValue()) / 1000
			}
			resourceMap[aisUserID][aisTenantID].UsedQuota.CPU += requestCPUCores
			resourceMap[aisUserID][aisTenantID].UsedQuota.Memory += requestMemoryBytes
			resourceMap[aisUserID][aisTenantID].UsedQuota.GPU += limitGPU
			resourceMap[aisUserID][aisTenantID].UsedQuota.VirtGPU += limitVirtGPU
		}
	}
	return resourceMap
}

func helpDefaultEmpty(str string, isID bool) string {
	if str != "" {
		return str
	}

	if isID {
		return consts.NoneID
	}

	return "系统占用"
}

func AggregateUsedResource(ctx *ginlib.GinContext, groupBy, userID, tenantID string) *resource.Used {
	_, rs, err := ListResource(ctx, 1, 1, groupBy, "_id", "", "", userID, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to ListResource")
		return resource.NilUsed()
	}
	if len(rs) > 0 {
		return &rs[0].UsedQuota
	}

	return resource.NilUsed()
}

func AggregateAccumulateResource(ctx *ginlib.GinContext, groupBy, sortBy, userID, tenantID string) *resource.Used {
	bil, err := GetBillByGroup(ctx, groupBy, userID, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetBillByGroup")
		return resource.NilUsed()
	}
	return &resource.Used{
		CPU:     bil.Accumulate.CPU + bil.AccumulateAdvance.CPU*consts.AccumulateAdvanceCPU,
		GPU:     bil.Accumulate.GPU + bil.AccumulateAdvance.GPU*consts.AccumulateAdvanceGPU,
		VirtGPU: bil.Accumulate.GPU + bil.AccumulateAdvance.VirtGPU*consts.AccumulateAdvanceGPU,
		Memory:  bil.Accumulate.Memory + bil.AccumulateAdvance.Memory*consts.AccumulateAdvanceMemory,
		SSD:     bil.Accumulate.SSD + bil.AccumulateAdvance.SSD*consts.AccumulateAdvanceSSD,
		HDD:     bil.Accumulate.HDD + bil.AccumulateAdvance.CPU*consts.AccumulateAdvanceHDD,
	}
}

func AggregateRecentAccumulateResource(ctx *ginlib.GinContext, groupBy, sortBy, userID, tenantID string) *resource.Used {
	ch, err := GetRecentAccumulate(ctx, groupBy, userID, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetRecentAccumulate")
		return resource.NilUsed()
	}
	return &ch.UsedQuota
}

func increaseUsed(used *resource.Used, node *resource.Node) {
	used.CPU += node.Total.CPU
	used.GPU += node.Total.GPU
	used.NPU += node.Total.NPU
	used.Memory += node.Total.Memory
	used.Storage += node.Total.Storage
	used.VirtGPU += node.Total.VirtGPU
	used.SSD += node.Total.SSD
	used.HDD += node.Total.HDD
}

func increseGPUUsed(npuTotal, gpuTotal, virtgpuTotal map[string]float64, node *resource.Node) {
	// 处理卡类型资源
	if node.GPUType != "" {
		if _, found := gpuTotal[node.GPUType]; !found {
			gpuTotal[node.GPUType] = 0
		}
		gpuTotal[node.GPUType] += node.Total.GPU

		if _, found := virtgpuTotal[node.GPUType]; !found {
			virtgpuTotal[node.GPUType] = 0
		}
		virtgpuTotal[node.GPUType] += node.Total.VirtGPU
	}

	if node.NPUType != "" {
		if _, found := npuTotal[node.NPUType]; !found {
			npuTotal[node.NPUType] = 0
		}
		npuTotal[node.NPUType] += node.Total.NPU
	}
}

func fillGPUTypeQuota(total quotaTypes.Quota, npuTotal, gpuTotal, virtgpuTotal map[string]float64) {
	for gpuType := range gpuTotal {
		quantity := gpuTotal[gpuType]
		total[types.ResourceNameWithGPUType(gpuType)] = *k8sresource.NewQuantity(int64(quantity*1000)/1000, k8sresource.DecimalSI)
	}
	for gpuType := range virtgpuTotal {
		quantity := virtgpuTotal[gpuType]
		total[types.ResourceNameWithVirtGPUType(gpuType)] = *k8sresource.NewQuantity(int64(quantity*1000)/1000, k8sresource.DecimalSI)
	}
	for npuType := range npuTotal {
		quantity := npuTotal[npuType]
		total[types.ResourceNameWithNPUType(npuType)] = *k8sresource.NewQuantity(int64(quantity*1000)/1000, k8sresource.DecimalSI)
	}
}

func AggregateTotalResource(ctx *ginlib.GinContext) (quotaTypes.Quota, quotaTypes.Quota) {
	total := &resource.Used{}
	schedulable := &resource.Used{}
	_, nds, err := ListNode(ctx, "", "", false, true, false, nil)
	if err != nil {
		log.WithError(err).Error("failed to GetNodeTotal")
		return quotaTypes.NilQuota(), quotaTypes.NilQuota()
	}

	npuTotal, gpuTotal, virtgpuTotal := make(map[string]float64), make(map[string]float64), make(map[string]float64)
	schedulableNPUTotal, schedulableGPUTotal, schedulableVirtgpuTotal := make(map[string]float64), make(map[string]float64), make(map[string]float64)

	for i := range nds {
		node := &nds[i]
		increaseUsed(total, node)
		increseGPUUsed(npuTotal, gpuTotal, virtgpuTotal, node)
		if !node.UnSchedulable {
			increaseUsed(schedulable, node)
			increseGPUUsed(schedulableNPUTotal, schedulableGPUTotal, schedulableVirtgpuTotal, node)
		}
	}

	total.Storage = total.SSD
	schedulable.Storage = schedulable.SSD

	totalQuota := quotaTypes.BuildQuotaFromUsed(total)
	schedulableQuota := quotaTypes.BuildQuotaFromUsed(schedulable)

	fillGPUTypeQuota(totalQuota, npuTotal, gpuTotal, virtgpuTotal)
	fillGPUTypeQuota(schedulableQuota, schedulableNPUTotal, schedulableGPUTotal, schedulableVirtgpuTotal)
	fmt.Println("==============================")
	v, _ := json.Marshal(totalQuota)
	fmt.Println(string(v))
	fmt.Println("==============================")
	return totalQuota, schedulableQuota
}

func ListAllResource(ctx *ginlib.GinContext, page, pageSize int, isFilterDeleted bool) (int64, []resource.Resource, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("resource")

	filter := bson.M{}
	if isFilterDeleted {
		filter["isDeleted"] = false
	}
	findOptions := options.Find()
	findOptions.SetSkip(int64((page - 1) * pageSize))
	findOptions.SetLimit(int64(pageSize))

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return 0, nil, err
	}
	var rs []resource.Resource
	if err := cursor.All(ctx, &rs); err != nil {
		return 0, nil, err
	}

	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}

	return total, rs, nil
}

func ListResource(ctx *ginlib.GinContext, page, pageSize int, groupBy, sortBy, userName, tenantName, userID, tenantID string) (int64, []resource.Resource, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("resource")

	if sortBy != "" {
		sortBy = fmt.Sprintf("usedQuota.%s", sortBy)
	} else {
		sortBy = "_id"
	}

	filter := bson.M{"isDeleted": false}
	if groupBy == consts.CategoryUser {
		if userName != "" {
			filter["userName"] = bson.M{"$regex": ".*" + userName + ".*", "$options": "i"}
		}
		if userID != "" {
			filter["userID"] = userID
		} else {
			filter["userID"] = bson.M{"$ne": "None"}
		}
		filter["tenantType"] = consts.TenantTypeWorkspace
	} else if groupBy == consts.CategoryTenant {
		if tenantName != "" {
			filter["tenantName"] = bson.M{"$regex": ".*" + tenantName + ".*", "$options": "i"}
		}
		if tenantID != "" {
			filter["tenantID"] = tenantID
		}
		filter["tenantType"] = consts.TenantTypeOther
	}

	group := primitive.M{
		"_id":         nil,
		"cpu":         primitive.M{"$sum": "$usedQuota.cpu"},
		"memory":      primitive.M{"$sum": "$usedQuota.memory"},
		"gpu":         primitive.M{"$sum": "$usedQuota.gpu"},
		"virtgpu":     primitive.M{"$sum": "$usedQuota.virtgpu"},
		"ssd":         primitive.M{"$sum": "$usedQuota.ssd"},
		"hdd":         primitive.M{"$sum": "$usedQuota.hdd"},
		"memberCount": primitive.M{"$sum": 1},
		"createdAt":   primitive.M{"$max": "$createdAt"},
	}
	if groupBy == consts.CategoryUser {
		group["_id"] = primitive.M{"userID": "$userID", "userName": "$userName"}
	} else if groupBy == consts.CategoryTenant {
		group["_id"] = primitive.M{"tenantID": "$tenantID", "tenantName": "$tenantName"}
	}

	project := primitive.M{
		"createdAt":   "$createdAt",
		"memberCount": "$memberCount",
		"usedQuota": primitive.M{
			"cpu":     "$cpu",
			"memory":  "$memory",
			"gpu":     "$gpu",
			"virtgpu": "$virtgpu",
			"ssd":     "$ssd",
			"hdd":     "$hdd",
		},
	}
	if groupBy == consts.CategoryUser {
		project["_id"] = "$_id.userID"
		project["userID"] = "$_id.userID"
		project["userName"] = "$_id.userName"
	} else if groupBy == consts.CategoryTenant {
		project["_id"] = "$_id.tenantID"
		project["tenantID"] = "$_id.tenantID"
		project["tenantName"] = "$_id.tenantName"
	}

	sort := bson.D{bson.E{Key: sortBy, Value: -1}}
	if sortBy != "_id" {
		sort = append(sort, bson.E{Key: "_id", Value: 1})
	}

	pipeline := []primitive.M{
		{"$match": filter},
		{"$group": group},
		{"$project": project},
		{"$sort": sort},
		{"$skip": (page - 1) * pageSize},
		{"$limit": pageSize},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, nil, err
	}

	var rs []resource.Resource
	if err := cursor.All(ctx, &rs); err != nil {
		return 0, nil, err
	}

	pipeline = []primitive.M{
		{"$match": filter},
		{"$group": group},
		{"$group": primitive.M{"_id": nil, "total": primitive.M{"$sum": 1}}},
	}

	cursor, err = col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, nil, err
	}

	var total int64
	var to []struct {
		Total int64 `bson:"total" json:"total"`
	}
	if err := cursor.All(ctx, &to); err != nil {
		return 0, nil, err
	}
	if len(to) > 0 {
		total = to[0].Total
	}

	return total, rs, nil
}

func GetResource(ctx *ginlib.GinContext, userID, tenantID string) (*resource.Resource, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("resource")

	filter := bson.M{"isDeleted": false, "userID": userID, "tenantID": tenantID}
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	var rs []resource.Resource
	if err := cursor.All(ctx, &rs); err != nil {
		return nil, err
	}

	if len(rs) == 0 {
		return nil, nil
	}
	return &rs[0], nil
}

func UpdateResourceByUUID(ctx *ginlib.GinContext, nw *resource.Resource) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("resource")

	filter := bson.M{"uuid": nw.UUID}
	update := bson.M{
		"$set": bson.M{
			"userName":   nw.UserName,
			"tenantName": nw.TenantName,
			"usedQuota":  nw.UsedQuota,
			"createdAt":  nw.CreatedAt,
			"isDeleted":  nw.IsDeleted,
			"tenantType": nw.TenantType,
			"updateAt":   time.Now().Unix(),
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func DeleteResource(ctx *ginlib.GinContext, nw *resource.Resource) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("resource")

	filter := bson.M{"userID": nw.UserID, "tenantID": nw.TenantID}
	update := bson.M{
		"$set": bson.M{
			"isDeleted": true,
		},
	}

	if _, err := col.UpdateMany(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func CreateResource(ctx *ginlib.GinContext, nw *resource.Resource) error {
	nw.CreatedAt = time.Now().Unix()
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("resource")

	if _, err := col.InsertOne(ctx, nw); err != nil {
		return err
	}

	return nil
}
