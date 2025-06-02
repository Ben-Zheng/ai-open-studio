package consts

import (
	"go.mongodb.org/mongo-driver/mongo"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coocood/freecache"
	clientv3 "go.etcd.io/etcd/client/v3"

	datahubV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/korok"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/kylin"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/release"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

var (
	// GinEngine gin框架全局对象
	GinEngine *ginlib.Gin

	// ConfigMap 管理全局配置信息
	ConfigMap = new(Config)

	// MongoClient 本服务mongo client对象
	MongoClient     *mgolib.MgoClient
	AuthMongoClient *mongo.Client

	WorkloadLister quota.WorkloadsLister

	// OSSSession 对象存储客户端
	OSSSession *session.Session

	EtcdClient *clientv3.Client

	ResourceClient *resource.Client

	ReleaseClient release.Client

	KylinClient *kylin.Client

	KorokClient *korok.Client

	DatahubClient    datahubV1.Client
	DatasetNameCache = freecache.NewCache(1024)
)

const (
	// MongoDBOperationTimeout 是mongodb操作的超时时间
	MongoDBOperationTimeout = 180

	BackgroundJobLock           = "/ais/resourcesense/background"
	BackgroundJobLockGetTimeout = 3 * time.Second

	WebhookPort = ":443"

	KubeSystemNamespace = "kube-system"
	SystemNamespace     = "system-namespace"
	GPUType             = "GPUType"
	NPUType             = "NPUType"
	NoneID              = "None"
	CPUArch             = "kubernetes.io/arch"

	TenantTypeWorkspace = "workspace"
	TenantTypeOther     = "other"

	AccumulateAdvanceCPU    = 60 * 60                      // 进位：核时
	AccumulateAdvanceGPU    = 60 * 60                      // 进位：卡时
	AccumulateAdvanceMemory = 60 * 60 * 1024 * 1024 * 1024 // 进位：Gi时
	AccumulateAdvanceHDD    = 60 * 60 * 1024 * 1024 * 1024 // 进位：Gi时
	AccumulateAdvanceSSD    = 60 * 60 * 1024 * 1024 * 1024 // 进位：Gi时

	DatasetNameCacheTTL = 120
)

const (
	CategoryUser        = "user"
	CategoryTenant      = "tenant"
	CategoryNode        = "node"
	CategoryPod         = "pod"
	CategoryTenantQuota = "tenantquota"

	QueryUserCPU     = `sum (ais_exporter_resources_usage_cpu_cores{ais_user_id="%s"})`
	QueryUserGPU     = `sum (ais_exporter_resources_usage_gpu_cores{ais_user_id="%s"})`
	QueryUserVirtGPU = `sum (ais_exporter_resources_usage_virtgpu_cores{ais_user_id="%s"})`
	QueryUserMemory  = `sum (ais_exporter_resources_usage_memory_bytes{ais_user_id="%s"})`
	QueryUserSSD     = `sum (ais_exporter_resources_usage_nori_speed_bytes{ais_user_id="%s"})`
	QueryUserHDD     = `sum (ais_exporter_resources_usage_oss_bytes{ais_user_id="%s"})`

	QueryTenantCPU     = `sum (ais_exporter_resources_usage_cpu_cores{ais_tenant_id="%s"})`
	QueryTenantGPU     = `sum (ais_exporter_resources_usage_gpu_cores{ais_tenant_id="%s"})`
	QueryTenantVirtGPU = `sum (ais_exporter_resources_usage_virtgpu_cores{ais_tenant_id="%s"})`
	QueryTenantMemory  = `sum (ais_exporter_resources_usage_memory_bytes{ais_tenant_id="%s"})`
	QueryTenantSSD     = `sum (ais_exporter_resources_usage_nori_speed_bytes{ais_tenant_id="%s"})`
	QueryTenantHDD     = `sum (ais_exporter_resources_usage_oss_bytes{ais_tenant_id="%s"})`

	QueryNodeCPU     = `sum (ais_exporter_node_resources_rate_cpu_cores{uid="%s"})`
	QueryNodeGPU     = `sum (ais_exporter_node_resources_rate_gpu_cores{uid="%s"})`
	QueryNodeVirtGPU = `sum (ais_exporter_node_resources_rate_gpu_cores{uid="%s"})`
	QueryNodeMemory  = `sum (ais_exporter_node_resources_rate_memory_bytes{uid="%s"})`
	QueryNodeSSD     = `sum (ais_exporter_node_resources_rate_ssd_bytes{uid="%s"})`
	QueryNodeHDD     = `sum (ais_exporter_node_resources_rate_hdd_bytes{uid="%s"})`

	// 配额相关的监控指标
	QueryTenantQuotaCPU            = `max (ais_tenantquota_charged_cpu_cores{ais_tenant_name="%s"})`
	QueryTenantQuotaGPU            = `max (ais_tenantquota_charged_gpu_cores{ais_tenant_name="%s",sub_type="%s",producer="%s"})`
	QueryTenantQuotaVirtGPU        = `max (ais_tenantquota_charged_virtgpu_cores{ais_tenant_name="%s",sub_type="%s",producer="%s"})`
	QueryTenantQuotaMemory         = `max (ais_tenantquota_charged_memory_bytes{ais_tenant_name="%s"})`
	QueryTenantQuotaStorage        = `max (ais_tenantquota_charged_storage_bytes{ais_tenant_name="%s"})`
	QueryTenantQuotaDatasetStorage = `max (ais_tenantquota_charged_dataset_storage_bytes{ais_tenant_name="%s"})`

	QueryPodCPU     = `sum (ais_exporter_pod_resources_usage_percent_cpu_cores{exported_pod="%s"})`
	QueryPodGPU     = `sum (ais_exporter_gpu_resources_utilization{exported_pod="%s"})`
	QueryPodVirtGPU = `sum (ais_exporter_gpu_resources_utilization{exported_pod="%s"})`
	QueryPodMemory  = `sum (ais_exporter_pod_resources_usage_percent_memory_bytes{exported_pod="%s"})`
)

type PromQuery struct {
	QL     string
	Params []string
}

var QueryProme = map[string]map[string]*PromQuery{
	CategoryPod: {
		types.ResourceCPU:      &PromQuery{QL: QueryPodCPU, Params: []string{"pod"}},
		types.ResourceGPU:      &PromQuery{QL: QueryPodGPU, Params: []string{"pod"}},
		types.ResourceVirtGPU:  &PromQuery{QL: QueryPodVirtGPU, Params: []string{"pod"}},
		types.ResourceMemory:   &PromQuery{QL: QueryPodMemory, Params: []string{"pod"}},
		types.ResourceAPUWs:    &PromQuery{QL: QueryPodVirtGPU, Params: []string{"pod"}},
		types.ResourceAPUOther: &PromQuery{QL: QueryPodGPU, Params: []string{"pod"}},
	},
	CategoryNode: {
		types.ResourceCPU:      &PromQuery{QL: QueryNodeCPU, Params: []string{"nodeID"}},
		types.ResourceGPU:      &PromQuery{QL: QueryNodeGPU, Params: []string{"nodeID"}},
		types.ResourceVirtGPU:  &PromQuery{QL: QueryNodeVirtGPU, Params: []string{"nodeID"}},
		types.ResourceMemory:   &PromQuery{QL: QueryNodeMemory, Params: []string{"nodeID"}},
		types.ResourceSSD:      &PromQuery{QL: QueryNodeSSD, Params: []string{"nodeID"}},
		types.ResourceHDD:      &PromQuery{QL: QueryNodeHDD, Params: []string{"nodeID"}},
		types.ResourceAPUWs:    &PromQuery{QL: QueryNodeVirtGPU, Params: []string{"nodeID"}},
		types.ResourceAPUOther: &PromQuery{QL: QueryNodeGPU, Params: []string{"nodeID"}},
		types.ResourceAPU:      &PromQuery{QL: QueryNodeGPU, Params: []string{"nodeID"}},
	},
	CategoryUser: {
		types.ResourceCPU:      &PromQuery{QL: QueryUserCPU, Params: []string{"userID"}},
		types.ResourceGPU:      &PromQuery{QL: QueryUserGPU, Params: []string{"userID"}},
		types.ResourceVirtGPU:  &PromQuery{QL: QueryUserVirtGPU, Params: []string{"userID"}},
		types.ResourceMemory:   &PromQuery{QL: QueryUserMemory, Params: []string{"userID"}},
		types.ResourceSSD:      &PromQuery{QL: QueryUserSSD, Params: []string{"userID"}},
		types.ResourceHDD:      &PromQuery{QL: QueryUserHDD, Params: []string{"userID"}},
		types.ResourceAPUWs:    &PromQuery{QL: QueryUserVirtGPU, Params: []string{"userID"}},
		types.ResourceAPUOther: &PromQuery{QL: QueryUserGPU, Params: []string{"userID"}},
	},

	CategoryTenant: {
		types.ResourceCPU:      &PromQuery{QL: QueryTenantCPU, Params: []string{"tenantID"}},
		types.ResourceGPU:      &PromQuery{QL: QueryTenantGPU, Params: []string{"tenantID"}},
		types.ResourceVirtGPU:  &PromQuery{QL: QueryTenantVirtGPU, Params: []string{"tenantID"}},
		types.ResourceMemory:   &PromQuery{QL: QueryTenantMemory, Params: []string{"tenantID"}},
		types.ResourceSSD:      &PromQuery{QL: QueryTenantSSD, Params: []string{"tenantID"}},
		types.ResourceHDD:      &PromQuery{QL: QueryTenantHDD, Params: []string{"tenantID"}},
		types.ResourceAPUWs:    &PromQuery{QL: QueryTenantVirtGPU, Params: []string{"tenantID"}},
		types.ResourceAPUOther: &PromQuery{QL: QueryTenantGPU, Params: []string{"tenantID"}},
	},
	CategoryTenantQuota: {
		types.ResourceCPU:                  &PromQuery{QL: QueryTenantQuotaCPU, Params: []string{"tenantID"}},
		types.ResourceGPU:                  &PromQuery{QL: QueryTenantQuotaGPU, Params: []string{"tenantID", "subResourceType"}},
		types.KubeResourceGPU.String():     &PromQuery{QL: QueryTenantQuotaGPU, Params: []string{"tenantID", "subResourceType"}},
		types.ResourceVirtGPU:              &PromQuery{QL: QueryTenantQuotaVirtGPU, Params: []string{"tenantID", "subResourceType"}},
		types.KubeResourceVirtGPU.String(): &PromQuery{QL: QueryTenantQuotaVirtGPU, Params: []string{"tenantID", "subResourceType"}},
		types.ResourceMemory:               &PromQuery{QL: QueryTenantQuotaMemory, Params: []string{"tenantID"}},
		types.ResourceStorage:              &PromQuery{QL: QueryTenantQuotaStorage, Params: []string{"tenantID"}},
		types.ResourceDatasetStorage:       &PromQuery{QL: QueryTenantQuotaDatasetStorage, Params: []string{"tenantID"}},
		types.ResourceAPUWs:                &PromQuery{QL: QueryTenantQuotaVirtGPU, Params: []string{"tenantID", "subResourceType", "producer"}},
		types.ResourceAPUOther:             &PromQuery{QL: QueryTenantQuotaGPU, Params: []string{"tenantID", "subResourceType", "producer"}},
	},
}

var ResourceMap = map[string]aisConst.ResourceType{
	"dataset":        aisConst.ResourceTypeDataset,
	"public-dataset": aisConst.ResourceTypeDataset,
	"label-task":     aisConst.ResourceTypeRelabel,
	"evalhub":        aisConst.ResourceTypeEvaluationTask,
	"autolearn":      aisConst.ResourceTypeAutomaticLearning,
	"codebase":       aisConst.ResourceTypeCodebase,
	"model":          aisConst.ResourceTypeModel,
}
