package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8squota "k8s.io/apiserver/pkg/quota/v1"
	clientSet "k8s.io/client-go/kubernetes"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aistype "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

func UpdateSystemNamespace(kubeClient clientSet.Interface) map[string]bool {
	systemNamespace := make(map[string]bool)
	nslist, err := kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.WithError(err).Error("[ais] get ns list for metricsAPI failed.")
		return systemNamespace
	}

	systemNamespace[consts.KubeSystemNamespace] = true
	for index := range nslist.Items {
		if val, ok := nslist.Items[index].Labels[consts.SystemNamespace]; ok && val == "true" {
			systemNamespace[nslist.Items[index].Name] = true
		}
	}

	return systemNamespace
}

type markNodeResource struct {
	resource.Node
	mark         bool
	isUpdateDisk bool
}

func NodeSync(ctx context.Context) {
	interval := 30
	t := time.NewTicker(time.Second * time.Duration(interval))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Infof("shutdown node sync")
			return
		case <-t.C:
			log.Info("[Resource_loop] periodicity sync node running")
			nodeCollect, err := getNodeMap()
			if err != nil {
				log.WithError(err).Error("failed to getNodeMap")
				continue
			}

			if err := updateNode(nodeCollect); err != nil {
				log.WithError(err).Error("failed to updateNode")
				continue
			}

			if err := createNode(nodeCollect); err != nil {
				log.WithError(err).Error("failed to createNode")
				continue
			}
		}
	}
}

func updateNode(nodeCollect map[string]*markNodeResource) error {
	gc := ginlib.NewMockGinContext()
	_, ns, err := ListNode(gc, "", "", false, false, false, nil)
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		return err
	}

	for index := range ns {
		if ns[index].IsDeleted {
			log.WithField("nodeName", ns[index].Name).Debugln("node has been deleted")
			continue
		}

		if ns[index].UUID == "" {
			if err := DeleteNode(gc, &ns[index], ""); err != nil {
				log.WithError(err).Error("failed to DeleteNode")
			}
			continue
		}

		var isUpdateDisk bool
		_, ok := nodeCollect[ns[index].UID]
		if ok && !nodeCollect[ns[index].UID].mark {
			nodeCollect[ns[index].UID].mark = true
			nodeCollect[ns[index].UID].Node.UUID = ns[index].UUID
			ns[index] = nodeCollect[ns[index].UID].Node
			if len(ns[index].SSDs) != 0 || len(ns[index].HDDs) != 0 {
				isUpdateDisk = true
			}
		} else {
			var excludeUUID string
			if ok {
				excludeUUID = nodeCollect[ns[index].UID].Node.UUID
			}

			ns[index].IsDeleted = true
			if err := DeleteNode(gc, &ns[index], excludeUUID); err != nil {
				log.WithError(err).Error("failed to DeleteNode")
			}
			continue
		}

		if err := UpdateNode(gc, &ns[index], isUpdateDisk); err != nil {
			log.WithError(err).Error("failed to UpdateNode")
			continue
		}
	}

	return nil
}

func createNode(nodeCollect map[string]*markNodeResource) error {
	gc := ginlib.NewMockGinContext()

	for uid := range nodeCollect {
		if !nodeCollect[uid].mark {
			if err := CreateNode(gc, &nodeCollect[uid].Node); err != nil {
				log.WithError(err).WithField("uid", uid).WithField("nodeName", nodeCollect[uid].Name).Error("failed to CreateNode")
				continue
			}
		}
	}

	return nil
}

func getNodeMap() (map[string]*markNodeResource, error) {
	nodes, err := consts.ResourceClient.NodeLister.List(consts.ResourceClient.NodeSelector)
	if err != nil {
		log.WithError(err).Error("get details, list nodes failed")
		return nil, err
	}
	pods, err := consts.ResourceClient.PodLister.List(labels.Everything())
	if err != nil {
		log.WithError(err).Error("get details, list pods failed")
		return nil, err
	}

	systemNamespace := UpdateSystemNamespace(consts.ResourceClient.Clientset)
	machines, podMap := GetNodeResources(systemNamespace, nodes, pods)

	nodeCollect := make(map[string]*markNodeResource)
	for i := range nodes {
		nr := markNodeResource{}
		nr.UID = string(nodes[i].UID)
		nr.Name = nodes[i].Name

		for _, addr := range nodes[i].Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nr.IPAddr = addr.Address
			}
		}
		nr.GPUType = nodes[i].Labels[consts.GPUType]
		nr.NPUType = nodes[i].Labels[consts.NPUType]
		nr.CPUArch = nodes[i].Labels[consts.CPUArch]
		nr.APUType = nr.GPUType
		if nr.NPUType != "" {
			nr.APUType = nr.NPUType
		}
		nr.Total = convert(machines[nodes[i].Name].Total)

		if val, ok := podMap[nodes[i].Name]; ok {
			nr.Pods = getPodResource(val)
		}

		nr.Status = string(corev1.ConditionUnknown)
		if len(nodes[i].Status.Conditions) > 0 {
			nr.Status = string(nodes[i].Status.Conditions[len(nodes[i].Status.Conditions)-1].Status)
		}

		nr.UnSchedulable = nodes[i].Spec.Unschedulable

		nodeSystem := convert(machines[nodes[i].Name].System)
		nodeUsed := convert(machines[nodes[i].Name].Used)
		nodeReleasing := convert(machines[nodes[i].Name].Releasing)
		nodeTotal := convert(machines[nodes[i].Name].Total)
		nodeUsable := convert(machines[nodes[i].Name].Usable)
		nr.CPU = resource.Category{
			Releasing: nodeReleasing.CPU,
			System:    nodeSystem.CPU,
			Usable:    nodeUsable.CPU,
			Used:      nodeUsed.CPU,
			Total:     nodeTotal.CPU,
		}
		nr.GPU = resource.Category{
			Releasing: nodeReleasing.GPU,
			System:    nodeSystem.GPU,
			Usable:    nodeUsable.GPU,
			Used:      nodeUsed.GPU,
			Total:     nodeTotal.GPU,
		}
		nr.VirtGPU = resource.Category{
			Releasing: nodeReleasing.VirtGPU,
			System:    nodeSystem.VirtGPU,
			Usable:    nodeUsable.VirtGPU,
			Used:      nodeUsed.VirtGPU,
			Total:     nodeTotal.VirtGPU,
		}
		nr.NPU = resource.Category{
			Releasing: nodeReleasing.NPU,
			System:    nodeSystem.NPU,
			Usable:    nodeUsable.NPU,
			Used:      nodeUsed.NPU,
			Total:     nodeTotal.NPU,
		}
		nr.APU = resource.Category{
			Releasing: nodeReleasing.NPU + nodeReleasing.GPU,
			System:    nodeSystem.NPU + nodeSystem.GPU,
			Usable:    nodeUsable.NPU + nodeUsable.GPU,
			Used:      nodeUsed.NPU + nodeUsed.GPU,
			Total:     nodeTotal.NPU + nodeTotal.GPU,
		}
		nr.Memory = resource.Category{
			Releasing: nodeReleasing.Memory,
			System:    nodeSystem.Memory,
			Usable:    nodeUsable.Memory,
			Used:      nodeUsed.Memory,
			Total:     nodeTotal.Memory,
		}

		nr.CreationTimestamp = nodes[i].CreationTimestamp.Unix()
		nr.StartTimestamp = nodes[i].CreationTimestamp.Unix()
		if len(nodes[i].ManagedFields) > 0 {
			nr.StartTimestamp = nodes[i].ManagedFields[0].Time.Unix()
		}

		var isUpdateDisk bool
		if val, ok := podMap[nodes[i].Name]; ok {
			disks, err := getDiskCategory(val)
			if err != nil {
				isUpdateDisk = true
				log.WithError(err).Error("failed to get getDiskCategory")
			}
			for _, disk := range disks {
				diskCategory := resource.Category{
					Name:      disk.Name,
					Releasing: 0,
					System:    0,
					Usable:    float64(disk.Size - disk.Used),
					Used:      float64(disk.Used),
					Total:     float64(disk.Size),
				}

				if disk.IsHDD {
					nr.Total.HDD += float64(disk.Size)
					nr.HDDs = append(nr.HDDs, diskCategory)
				} else {
					nr.Total.SSD += float64(disk.Size)
					nr.SSDs = append(nr.SSDs, diskCategory)
				}
			}
		}
		nr.isUpdateDisk = isUpdateDisk

		nodeCollect[nr.UID] = &nr
	}

	return nodeCollect, nil
}

type Disk struct {
	Name  string `json:"name"`
	IsHDD bool   `json:"isHDD"`
	Size  int64  `json:"size"`
	Used  int64  `json:"used"`
}

func getDiskCategory(pods []*corev1.Pod) ([]Disk, error) {
	lab := strings.Split(consts.ConfigMap.DiskExporterLabel, ":")
	if len(lab) != 2 || lab[0] == "" || lab[1] == "" {
		log.WithField("disk-exporter-label", consts.ConfigMap.DiskExporterLabel).Warn("params error")
		return nil, nil
	}

	var podIP string
	for _, pod := range pods {
		if val, ok := pod.Labels[lab[0]]; ok && val == lab[1] {
			podIP = pod.Status.PodIP
			break
		}
	}
	if podIP == "" {
		log.WithField("disk-exporter-label", consts.ConfigMap.DiskExporterLabel).Warn("podIP not exist")
		return nil, nil
	}

	disks, err := getDisks(fmt.Sprintf("http://%s:%d/disks", podIP, consts.ConfigMap.DiskExporterPort))
	if err != nil {
		return nil, err
	}

	return disks, nil
}

func getDisks(diskURI string) ([]Disk, error) {
	var response struct {
		Total int64
		Items []Disk
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", diskURI, nil)
	if err != nil {
		log.WithError(err).Error("failed to get node disks request")
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("get node disks failed")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("node disks get request with status code: %d", resp.StatusCode)
		log.WithError(err).WithField("request", req).Error("request get node disks failed ")
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	if err := json.Unmarshal(body, &response); err != nil {
		log.WithError(err).Error("unmarshal node disks get response failed")
		return nil, err
	}

	return response.Items, nil
}

func getPodResource(pods []*corev1.Pod) []resource.Pod {
	podResource := make([]resource.Pod, len(pods))
	for i := range pods {
		aisUserID := helpDefaultEmpty(pods[i].Labels[aistype.AISUserID], true)
		aisTenantID := helpDefaultEmpty(pods[i].Labels[aistype.AISTenantID], true)

		podResource[i].UID = string(pods[i].UID)
		podResource[i].Name = pods[i].Name
		podResource[i].Resourcetype = pods[i].Labels[aistype.AISResourceType]
		podResource[i].UserID = aisUserID
		podResource[i].UserName = getUserNameByUserID(aisUserID, pods[i].Labels[aistype.AISUserName])
		podResource[i].TenantID = aisTenantID
		podResource[i].UsedQuota = getPodsUsed(pods[i])
		podResource[i].Phase = string(pods[i].Status.Phase)
		podResource[i].CreationTimestamp = pods[i].CreationTimestamp.Unix()
		if pods[i].DeletionTimestamp != nil {
			podResource[i].IsDeleting = true
			podResource[i].DeletionTimestamp = pods[i].DeletionTimestamp.Unix()
		}
	}
	return podResource
}

func getUserNameByUserID(userID, userNameInLabel string) string {
	if consts.NoneID == userID {
		return helpDefaultEmpty("", false)
	}

	if userNameInLabel != "" {
		return userNameInLabel
	}

	u, err := GetUserByID(userID)
	if err != nil {
		log.WithError(err).Error("failed to getUserByID")
		return helpDefaultEmpty("", false)
	}

	return u.Name
}

func getPodsUsed(pod *corev1.Pod) resource.Used {
	var rq resource.Used

	containers := append(pod.Spec.Containers, pod.Spec.InitContainers...)
	for containersIndex := range containers {
		requestCPUCores := float64(containers[containersIndex].Resources.Requests.Cpu().MilliValue()) / 1000
		requestMemoryBytes := float64(containers[containersIndex].Resources.Requests.Memory().MilliValue()) / 1000
		var requestGPU float64
		var requestNPU float64
		var requestVirtGPU float64

		if val, ok := containers[containersIndex].Resources.Requests[types.KubeResourceGPU]; ok {
			requestGPU = float64(val.MilliValue()) / 1000
		}
		if val, ok := containers[containersIndex].Resources.Requests[types.KubeResourceVirtGPU]; ok {
			requestVirtGPU = float64(val.MilliValue()) / 1000
		}
		for _, apuc := range consts.ConfigMap.APUConfigs {
			if apuc.ResourceName == types.KubeResourceGPU || apuc.ResourceName == types.KubeResourceVirtGPU {
				continue
			}
			val, ok := containers[containersIndex].Resources.Requests[apuc.ResourceName]
			if !ok {
				continue
			}
			if apuc.Kind == aistype.HUAWEINPUResourceName {
				requestNPU = float64(val.MilliValue()) / 1000
			}
		}
		rq.CPU += requestCPUCores
		rq.Memory += requestMemoryBytes
		rq.GPU += requestGPU
		rq.NPU += requestNPU
		rq.APU += requestGPU + requestNPU
		rq.VirtGPU += requestVirtGPU
	}

	return rq
}

func convert(qm map[corev1.ResourceName]k8sresource.Quantity) resource.Used {
	var rq resource.Used
	if val, ok := qm[types.KubeResourceCPU]; ok {
		rq.CPU = float64(val.MilliValue()) / 1000
	}
	if val, ok := qm[types.KubeResourceGPU]; ok {
		rq.GPU = float64(val.MilliValue()) / 1000
	}
	if val, ok := qm[types.KubeResourceVirtGPU]; ok {
		rq.VirtGPU = float64(val.MilliValue()) / 1000
	}
	if val, ok := qm[types.KubeResourceMemory]; ok {
		rq.Memory = float64(val.MilliValue()) / 1000
	}

	for _, apu := range consts.ConfigMap.APUConfigs {
		if apu.ResourceName != types.KubeResourceGPU && apu.ResourceName != types.KubeResourceVirtGPU {
			if val, ok := qm[apu.ResourceName]; ok {
				rq.NPU += float64(val.MilliValue()) / 1000
			}
		}
	}
	return rq
}

type NodeResources struct {
	NodeName  string                                       `json:"nodeName,omitempty"`
	System    map[corev1.ResourceName]k8sresource.Quantity `json:"system,omitempty"`
	Used      map[corev1.ResourceName]k8sresource.Quantity `json:"used,omitempty"`
	Releasing map[corev1.ResourceName]k8sresource.Quantity `json:"releasing,omitempty"`
	Total     map[corev1.ResourceName]k8sresource.Quantity `json:"total,omitempty"`
	Usable    map[corev1.ResourceName]k8sresource.Quantity `json:"usable,omitempty"`
}

func GetResourceFromNode(node *corev1.Node) *NodeResources {
	initResources := corev1.ResourceList{
		corev1.ResourceCPU:        k8sresource.MustParse("0"),
		corev1.ResourceMemory:     k8sresource.MustParse("0"),
		types.KubeResourceGPU:     k8sresource.MustParse("0"),
		types.KubeResourceVirtGPU: k8sresource.MustParse("0"),
	}
	// make corev1.ResourceCPU, corev1.ResourceMemory, KubeResourceGPU exists in resourceList
	total := k8squota.Add(node.Status.Capacity, initResources)
	return &NodeResources{
		NodeName:  node.Name,
		System:    k8squota.SubtractWithNonNegativeResult(total, node.Status.Allocatable),
		Total:     total,
		Usable:    initResources,
		Releasing: initResources,
		Used:      initResources,
	}
}

func getResourceFromContainers(containers []corev1.Container, containerStatuses []corev1.ContainerStatus) (resourceList corev1.ResourceList) {
	// obtain  container.Resources.Requests
	resourceList = make(corev1.ResourceList)
	for containerIndex := range containers {
		request := containers[containerIndex].Resources.Requests
		resourceList = k8squota.Add(resourceList, request)
	}
	return
}

func GetResourceFromPod(pod *corev1.Pod) corev1.ResourceList {
	// obtain container status and container.Resources.Requests
	var requests corev1.ResourceList
	requests = getResourceFromContainers(pod.Spec.Containers, pod.Status.ContainerStatuses)
	for i := range pod.Spec.InitContainers {
		requests = k8squota.Max(requests, pod.Spec.InitContainers[i].Resources.Requests)
	}
	if pod.Spec.Overhead != nil {
		requests = k8squota.Add(requests, pod.Spec.Overhead)
	}
	return requests
}

func GetNodeResources(systemNamespace map[string]bool, nodes []*corev1.Node, pods []*corev1.Pod) (map[string]*NodeResources, map[string][]*corev1.Pod) {
	machines := make(map[string]*NodeResources)
	podMap := make(map[string][]*corev1.Pod)

	for _, node := range nodes {
		machines[node.Name] = GetResourceFromNode(node)
	}

	for _, pod := range pods {
		nodeName := pod.Spec.NodeName
		podMap[nodeName] = append(podMap[nodeName], pod)

		if nodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			// not scheduled，succeeded, failed: no resource
			continue
		}
		if _, ok := machines[nodeName]; !ok {
			continue
		}
		if pod.DeletionTimestamp != nil && !pod.DeletionTimestamp.IsZero() {
			// Releasing resource
			machines[nodeName].Releasing = k8squota.Add(machines[nodeName].Releasing, GetResourceFromPod(pod))
			continue
		}
		var flag bool
		for i := range pod.Status.ContainerStatuses {
			if !(*pod.Status.ContainerStatuses[i].Started) && pod.Status.ContainerStatuses[i].State.Terminated != nil {
				// Releasing resource
				machines[nodeName].Releasing = k8squota.Add(machines[nodeName].Releasing, GetResourceFromPod(pod))
				flag = true
				break
			}
		}
		if flag {
			continue
		}
		// systemUsed
		if ok, value := systemNamespace[pod.Namespace]; ok && value {
			machines[nodeName].System = k8squota.Add(machines[nodeName].System, GetResourceFromPod(pod))
			continue
		}
		// Used resource
		machines[nodeName].Used = k8squota.Add(machines[nodeName].Used, GetResourceFromPod(pod))
	}

	for nodeName := range machines {
		machines[nodeName] = CalUsable(machines[nodeName])
	}

	return machines, podMap
}

func CalUsable(nr *NodeResources) *NodeResources {
	usable := k8squota.SubtractWithNonNegativeResult(nr.Total, nr.System)
	usable = k8squota.SubtractWithNonNegativeResult(usable, nr.Used)
	usable = k8squota.SubtractWithNonNegativeResult(usable, nr.Releasing)
	nr.Usable = usable
	return nr
}

type aggNodeAPUType struct {
	APUType  string `bson:"_id"`
	TotalGPU int    `bson:"totalGPU"`
	TotalNPU int    `bson:"totalNPU"`
}

func AggregateClusterAPUTypes(ctx *ginlib.GinContext) ([]string, map[string]int, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")

	filter := bson.M{"isDeleted": false}
	group := primitive.M{
		"_id":      "$apuType",
		"totalGPU": bson.M{"$sum": "$total.gpu"},
		"totalNPU": bson.M{"$sum": "$total.npu"},
	}

	pipeline := []primitive.M{
		{"$match": filter},
		{"$group": group},
		{"$sort": bson.M{
			"_id": 1,
		}},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, nil, err
	}

	var rs []aggNodeAPUType
	if err := cursor.All(ctx, &rs); err != nil {
		return nil, nil, err
	}

	ret := make([]string, 0)
	apuTypeToGPUNum := make(map[string]int)
	for i := range rs {
		if rs[i].APUType != "" {
			ret = append(ret, rs[i].APUType)
			apuTypeToGPUNum[rs[i].APUType] = rs[i].TotalGPU + rs[i].TotalNPU
		}
	}

	for _, apuc := range consts.ConfigMap.APUConfigs {
		if apuc.Product == "" {
			continue
		}
		if _, ok := apuTypeToGPUNum[apuc.Product]; ok {
			continue
		}

		ret = append(ret, apuc.Product)
		apuTypeToGPUNum[apuc.Product] = 0
	}

	return ret, apuTypeToGPUNum, nil
}

func GetNodeTotal(ctx *ginlib.GinContext) (*resource.Used, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")

	filter := bson.M{"isDeleted": false}

	group := primitive.M{
		"_id":     nil,
		"cpu":     primitive.M{"$sum": "$total.cpu"},
		"memory":  primitive.M{"$sum": "$total.memory"},
		"gpu":     primitive.M{"$sum": "$total.gpu"},
		"virtgpu": primitive.M{"$sum": "$total.virtgpu"},
		"ssd":     primitive.M{"$sum": "$total.ssd"},
		"hdd":     primitive.M{"$sum": "$total.hdd"},
	}

	pipeline := []primitive.M{
		{"$match": filter},
		{"$group": group},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var rs []resource.Used
	if err := cursor.All(ctx, &rs); err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return &resource.Used{}, nil
	}

	return &rs[0], nil
}

func ListNode(ctx *ginlib.GinContext, nodeID, nodeName string, isRegex, excludeDeleted, onlyUnSchedulable bool, pagination *ginlib.Pagination) (int64, []resource.Node, error) {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return 0, nil, err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")

	filter := bson.M{}
	if excludeDeleted {
		filter["isDeleted"] = false
	}

	if onlyUnSchedulable {
		filter["unSchedulable"] = true
	}

	if nodeName != "" {
		if isRegex {
			filter["name"] = bson.M{"$regex": ".*" + nodeName + ".*", "$options": "i"}
		} else {
			filter["name"] = nodeName
		}
	}
	if nodeID != "" {
		filter["uid"] = nodeID
	}

	findOptions := options.Find()
	if pagination != nil {
		findOptions.SetSkip(pagination.Skip)
		findOptions.SetLimit(pagination.Limit)
	}
	findOptions.SetSort(bson.M{"creationTimestamp": -1})

	cursor, err := col.Find(ctx, filter, findOptions)
	if err != nil {
		return 0, nil, err
	}
	var ns []resource.Node
	if err := cursor.All(ctx, &ns); err != nil {
		return 0, nil, err
	}

	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, nil, err
	}

	return total, ns, nil
}

func DeleteNode(ctx *ginlib.GinContext, nd *resource.Node, excludeUUID string) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")

	filter := bson.M{"uid": nd.UID}
	if excludeUUID != "" {
		filter["uuid"] = bson.M{"$ne": excludeUUID}
	}

	if _, err := col.DeleteMany(ctx, filter); err != nil {
		return err
	}

	return nil
}

func UpdateNode(ctx *ginlib.GinContext, nd *resource.Node, isUpdateDisk bool) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")

	filter := bson.M{"uuid": nd.UUID}
	setM := bson.M{
		"ipAddr":            nd.IPAddr,
		"gpuType":           nd.GPUType,
		"total":             nd.Total,
		"pods":              nd.Pods,
		"status":            nd.Status,
		"unSchedulable":     nd.UnSchedulable,
		"cpu":               nd.CPU,
		"memory":            nd.Memory,
		"gpu":               nd.GPU,
		"cpuArch":           nd.CPUArch,
		"npu":               nd.NPU,
		"apu":               nd.APU,
		"apuType":           nd.APUType,
		"npuType":           nd.NPUType,
		"virtgpu":           nd.VirtGPU,
		"isDeleted":         nd.IsDeleted,
		"startTimestamp":    nd.StartTimestamp,
		"creationTimestamp": nd.CreationTimestamp,
		"updateAt":          time.Now().Unix(),
	}
	if isUpdateDisk {
		setM["hdds"] = nd.HDDs
		setM["ssds"] = nd.SSDs
	}

	update := bson.M{"$set": setM}
	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func SetNodeUnschedulable(ctx *ginlib.GinContext, id string, unschedulable bool) error {
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}
	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")

	filter := bson.M{"uuid": id}
	update := bson.M{
		"$set": bson.M{
			"unSchedulable": unschedulable,
		},
	}

	if _, err := col.UpdateOne(ctx, filter, update); err != nil {
		return err
	}

	return nil
}

func CreateNode(ctx *ginlib.GinContext, nd *resource.Node) error {
	nd.UUID = uuid.New().String()
	mgoClient, err := consts.MongoClient.GetMongoClient(ctx)
	if err != nil {
		return err
	}

	col := mgoClient.Database(consts.ConfigMap.MgoConf.MongoDBName).Collection("node")
	if _, err := col.InsertOne(ctx, nd); err != nil {
		return err
	}

	return nil
}
