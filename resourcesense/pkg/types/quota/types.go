package quota

import (
	"go.mongodb.org/mongo-driver/bson/primitive"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/dataselect"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
)

type TenantQuotaAudit struct {
	ID                primitive.ObjectID `bson:"_id" json:"-"`
	TenantID          string             `bson:"tenantID" json:"tenantID"`
	TenantName        string             `bson:"tenantName" json:"tenantName"`
	TenantDisplayName string             `bson:"tenantDisplayName" json:"tenantDisplayName"`
	TotalQuota        Quota              `bson:"-" json:"-"`
	DiffQuota         Quota              `bson:"-" json:"-"`
	DTotalQuota       QuotaData          `bson:"totalQuota" json:"totalQuota"`
	DDiffQuota        QuotaData          `bson:"diffQuota" json:"diffQuota"`
	Creator           string             `bson:"creator" json:"creator"`
	CreatorID         string             `bson:"creatorID" json:"creatorID"`
	CreatedAt         int64              `bson:"createdAt" json:"createdAt"`
}

func (tqa *TenantQuotaAudit) ToDB() {
	tqa.DTotalQuota = tqa.TotalQuota.ToData().ToMongo()
	tqa.DDiffQuota = tqa.DiffQuota.ToData().ToMongo()
}

func (tqa *TenantQuotaAudit) FromDB() {
	tqa.DTotalQuota = tqa.DTotalQuota.FromMongo()
	tqa.DDiffQuota = tqa.DDiffQuota.FromMongo()
	tqa.TotalQuota = BuildQuotaFromData(tqa.DTotalQuota)
	tqa.DiffQuota = BuildQuotaFromData(tqa.DDiffQuota)
}

func (tqa *TenantQuotaAudit) ToData() {
	tqa.DTotalQuota = tqa.TotalQuota.ToData()
	tqa.DDiffQuota = tqa.DiffQuota.ToData()
}

// TenantDetail 所以对于前端来讲，有一个统一的接口和数据模型
// 这里作为 DB 里的基本模型 tenantdetail
type TenantDetail struct {
	ID                   primitive.ObjectID `bson:"_id" json:"-"`
	TenantType           string             `bson:"tenantType" json:"tenantType"` // 团队类型
	TenantID             string             `bson:"tenantID" json:"tenantID"`
	TenantName           string             `bson:"tenantName" json:"tenantName"`
	TenantDisplayName    string             `bson:"tenantDisplayName" json:"tenantDisplayName"`
	TotalQuota           Quota              `bson:"-" json:"-"` // 总配额/物理集群总资源， 通过配额分配的
	ChargedQuota         Quota              `bson:"-" json:"-"` // 已经分配配额, 通过同步统计的
	UsedQuota            Quota              `bson:"-" json:"-"` // 使用中的配额, 通过同步统计的
	Used                 Quota              `bson:"-" json:"-"` // 当前资源占用
	Accumulate           Quota              `bson:"-" json:"-"` // 累计
	RecentAccumulate     Quota              `bson:"-" json:"-"` // 最近累计
	MemberCount          int64              `bson:"memberCount" json:"memberCount"`
	IsDeleted            bool               `bson:"isDeleted" json:"isDeleted"`
	CreatedAt            int64              `bson:"createdAt" json:"createdAt"`
	UpdatedAt            int64              `bson:"updatedAt" json:"updatedAt"`
	DTotalQuota          QuotaData          `bson:"totalQuota" json:"totalQuota"`             // 总配额/物理集群总资源， 通过配额分配的
	DChargedQuota        QuotaData          `bson:"chargedQuota" json:"chargedQuota"`         // 已经分配配额, 通过同步统计的
	DUsedQuota           QuotaData          `bson:"usedQuota" json:"usedQuota"`               // 使用中的配额, 通过同步统计的
	DUsed                QuotaData          `bson:"used" json:"used"`                         // 当前资源占用
	DAccumulate          QuotaData          `bson:"accumulate" json:"accumulate"`             // 累计
	DRecentAccumulate    QuotaData          `bson:"recentAccumulate" json:"recentAccumulate"` // 最近累计
	AllocatedRareGPUType []string           `bson:"-" json:"allocatedRareGPUType"`            // 给前端展示使用，可使用的稀缺卡类型
}

func (td *TenantDetail) ToDB() {
	td.DTotalQuota = td.TotalQuota.ToData().ToMongo()
	td.DChargedQuota = td.ChargedQuota.ToData().ToMongo()
	td.DUsedQuota = td.UsedQuota.ToData().ToMongo()
	td.DUsed = td.Used.ToData().ToMongo()
	td.DAccumulate = td.Accumulate.ToData().ToMongo()
	td.DRecentAccumulate = td.RecentAccumulate.ToData().ToMongo()
}

func (td *TenantDetail) FromDB() {
	td.DTotalQuota = td.DTotalQuota.FromMongo()
	td.TotalQuota = BuildQuotaFromData(td.DTotalQuota)

	td.DChargedQuota = td.DChargedQuota.FromMongo()
	td.ChargedQuota = BuildQuotaFromData(td.DChargedQuota)

	td.DUsedQuota = td.DUsedQuota.FromMongo()
	td.UsedQuota = BuildQuotaFromData(td.DUsedQuota)

	td.DUsed = td.DUsed.FromMongo()
	td.Used = BuildQuotaFromData(td.DUsed.FromMongo())

	td.DAccumulate = td.DAccumulate.FromMongo()
	td.Accumulate = BuildQuotaFromData(td.DAccumulate)

	td.DRecentAccumulate = td.DRecentAccumulate.FromMongo()
	td.RecentAccumulate = BuildQuotaFromData(td.DRecentAccumulate)
}

func (td *TenantDetail) FromData() {
	td.TotalQuota = BuildQuotaFromData(td.DTotalQuota)
	td.ChargedQuota = BuildQuotaFromData(td.DChargedQuota)
	td.UsedQuota = BuildQuotaFromData(td.DUsedQuota)
	td.Used = BuildQuotaFromData(td.DUsed)
	td.Accumulate = BuildQuotaFromData(td.DAccumulate)
	td.RecentAccumulate = BuildQuotaFromData(td.DRecentAccumulate)
}

func (td *TenantDetail) ToData() {
	td.DTotalQuota = td.TotalQuota.ToData()
	td.DChargedQuota = td.ChargedQuota.ToData()
	td.DUsedQuota = td.UsedQuota.ToData()
	td.DUsed = td.Used.ToData()
	td.DAccumulate = td.Accumulate.ToData()
	td.DRecentAccumulate = td.RecentAccumulate.ToData()
}

// UserDetail 一个 user 可能在多个 tenant 中，这里会有多行记录
type UserDetail struct {
	ID                primitive.ObjectID `bson:"_id" json:"-"`
	UserID            string             `bson:"userID" json:"userID"`
	UserName          string             `bson:"userName" json:"userName"`
	TenantType        string             `bson:"tenantType" json:"tenantType"` // 团队类型
	TenantID          string             `bson:"tenantID" json:"tenantID"`
	TenantName        string             `bson:"tenantName" json:"tenantName"`
	ChargedQuota      Quota              `bson:"-" json:"-"`                               // 已经分配配额, 通过同步统计的
	UsedQuota         Quota              `bson:"-" json:"-"`                               // 使用中的配额, 通过同步统计的
	Used              Quota              `bson:"-" json:"-"`                               // 当前资源占用
	Accumulate        Quota              `bson:"-" json:"-"`                               // 累计资源占用
	RecentAccumulate  Quota              `bson:"-" json:"-"`                               // 最近累计资源占用
	DChargedQuota     QuotaData          `bson:"chargedQuota" json:"chargedQuota"`         // 已经分配配额, 通过同步统计的
	DUsedQuota        QuotaData          `bson:"usedQuota" json:"usedQuota"`               // 使用中的配额, 通过同步统计的
	DUsed             QuotaData          `bson:"used" json:"used"`                         // 当前资源占用
	DAccumulate       QuotaData          `bson:"accumulate" json:"accumulate"`             // 累计资源占用
	DRecentAccumulate QuotaData          `bson:"recentAccumulate" json:"recentAccumulate"` // 最近累计资源占用
	IsDeleted         bool               `bson:"isDeleted" json:"isDeleted"`
	CreatedAt         int64              `bson:"createdAt" json:"createdAt"`
	UpdatedAt         int64              `bson:"updatedAt" json:"updatedAt"`

	RoleName string `bson:"-" json:"roleName"`
}

func (ud *UserDetail) ToDB() {
	ud.DChargedQuota = ud.ChargedQuota.ToData().ToMongo()
	ud.DUsedQuota = ud.UsedQuota.ToData().ToMongo()
	ud.DUsed = ud.Used.ToData().ToMongo()
	ud.DAccumulate = ud.Accumulate.ToData().ToMongo()
	ud.DRecentAccumulate = ud.RecentAccumulate.ToData().ToMongo()
}

func (ud *UserDetail) FromDB() {
	ud.DChargedQuota = ud.DChargedQuota.FromMongo()
	ud.ChargedQuota = BuildQuotaFromData(ud.DChargedQuota)
	ud.DUsedQuota = ud.DUsedQuota.FromMongo()
	ud.UsedQuota = BuildQuotaFromData(ud.DUsedQuota)
	ud.DUsed = ud.DUsed.FromMongo()
	ud.Used = BuildQuotaFromData(ud.DUsed)
	ud.DAccumulate = ud.DAccumulate.FromMongo()
	ud.Accumulate = BuildQuotaFromData(ud.DAccumulate)
	ud.DRecentAccumulate = ud.DRecentAccumulate.FromMongo()
	ud.RecentAccumulate = BuildQuotaFromData(ud.DRecentAccumulate)
}

func (ud *UserDetail) ToData() {
	ud.DChargedQuota = ud.ChargedQuota.ToData()
	ud.DUsedQuota = ud.UsedQuota.ToData()
	ud.DUsed = ud.Used.ToData()
	ud.DAccumulate = ud.Accumulate.ToData()
	ud.DRecentAccumulate = ud.RecentAccumulate.ToData()
}

type ResourceEvent struct {
	EventType          ResourceEventType `bson:"eventType" json:"eventType"`
	ResourceType       KubeResourceType  `bson:"resourceType" json:"resourceType"`
	ResourceName       string            `bson:"resourceName" json:"resourceName"`
	CustomResourceType string            `bson:"customResourceType" json:"customResourceType"`
	CustomResourceID   string            `bson:"customResourceID" json:"customResourceID"`
	CustomResourceName string            `bson:"customResourceName" json:"customResourceName"`
	ChargedQuota       Quota             `bson:"-" json:"-"`
	DChargedQuota      QuotaData         `bson:"chargedQuota" json:"chargedQuota"`
	TenantName         string            `bson:"tenantName" json:"tenantName"`
	ProjectName        string            `bson:"projectName" json:"projectName"`
	Creator            string            `bson:"creator" json:"creator"`
	CreatorID          string            `bson:"creatorID" json:"creatorID"`
	CreatedAt          int64             `bson:"createdAt" json:"createdAt"`
}

func (re *ResourceEvent) ToDB() {
	re.DChargedQuota = re.ChargedQuota.ToData().ToMongo()
}

func (re *ResourceEvent) FromDB() {
	re.DChargedQuota = re.DChargedQuota.FromMongo()
	re.ChargedQuota = BuildQuotaFromData(re.DChargedQuota)
}

type Workload struct {
	ResourceType       KubeResourceType `bson:"resourceType" json:"resourceType"`
	ResourceName       string           `bson:"resourceName" json:"resourceName"`
	CustomResourceType string           `bson:"customResourceType" json:"customResourceType"`
	CustomResourceID   string           `bson:"customResourceID" json:"customResourceID"`
	CustomResourceName string           `bson:"customResourceName" json:"customResourceName"`
	ChargedQuota       Quota            `bson:"-" json:"-"`
	DChargedQuota      QuotaData        `bson:"chargedQuota" json:"chargedQuota"`
	TenantName         string           `bson:"tenantName" json:"tenantName"`
	ProjectName        string           `bson:"projectName" json:"projectName"`
	Node               *WorkloadNode    `bson:"-" json:"node"`
	Creator            string           `bson:"creator" json:"creator"`
	CreatorID          string           `bson:"creatorID" json:"creatorID"`
	CreatedAt          int64            `bson:"createdAt" json:"createdAt"`
}

type WorkloadNode struct {
	Name    string `json:"name"`
	GPUType string `json:"gpuType"`
}

func (w *Workload) ToDB() {
	w.DChargedQuota = w.ChargedQuota.ToData().ToMongo()
}

func (w *Workload) FromDB() {
	w.DChargedQuota = w.DChargedQuota.FromMongo()
	w.ChargedQuota = BuildQuotaFromData(w.DChargedQuota)
}

type WorkloadsLister interface {
	ListPodWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sort *ginlib.Sort) (int64, []*Workload, error)
	ListPVCWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sort *ginlib.Sort) (int64, []*Workload, error)
	ListDatasetWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sort *ginlib.Sort) (int64, []*Workload, error)
	ListAllWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sort *ginlib.Sort) (int64, []*Workload, error)
}

type GPUGroup struct {
	GPUType      string       `bson:"gpuType" json:"gpuType"`                                // GPU型号
	GPUTypeLevel GPUTypeLevel `bson:"gpuTypeLevel" json:"gpuTypeLevel" enums:"rare, normal"` // 按GPU型号划分等级，稀有类型、普通类型
}

type ClusterQuota struct {
	TotalQuota      Quota     `json:"-"`              // 集群总配额，可以通过配置指定，若未指定则为所有节点的资源总和
	AssignedQuota   Quota     `json:"-"`              // 分配出去的总配额, 团队分配的配额之和, 实时从 tenantQuota 配额分配表累计的
	ChargedQuota    Quota     `json:"-"`              // 统计的配额占用, 实时从 tenantQuota 配额分配表累积的
	UsedQuota       Quota     `json:"-"`              // 统计的配额使用 (可能大于 chargedQuota(因为包含非quota管理的占用), 也可能小于 chargedQuota(因为需要是实际调度上的))
	AvailableQuota  Quota     `json:"-"`              // 计算超发率之后的 = (totalQuota - chargedQuota) * 超发率
	Total           Quota     `json:"-"`              // 集群总资源: 通过 node 获取的，不包含不可调度节点资源
	Used            Quota     `json:"-"`              // 通过 resources 统计的
	DTotalQuota     QuotaData `json:"totalQuota"`     // 集群总配额，可以通过配置指定，若未指定则为所有节点的资源总和
	DAssignedQuota  QuotaData `json:"assignedQuota"`  // 分配出去的总配额, 团队分配的配额之和, 实时从 tenantQuota 配额分配表累计的
	DChargedQuota   QuotaData `json:"chargedQuota"`   // 统计的配额占用, 实时从 tenantQuota 配额分配表累积的
	DUsedQuota      QuotaData `json:"usedQuota"`      // 统计的配额使用 (可能大于 chargedQuota(因为包含非quota管理的占用), 也可能小于 chargedQuota(因为需要是实际调度上的))
	DAvailableQuota QuotaData `json:"availableQuota"` // 计算超发率之后的 = (totalQuota - chargedQuota) * 超发率
	DTotal          QuotaData `json:"total"`          // 集群总资源: 通过 node 获取的，不包含不可调度节点资源
	DUsed           QuotaData `json:"used"`           // 通过 resources 统计的
}

func (cd *ClusterQuota) ToData() {
	cd.DTotalQuota = cd.TotalQuota.ToData()
	cd.DAssignedQuota = cd.AssignedQuota.ToData()
	cd.DChargedQuota = cd.ChargedQuota.ToData()
	cd.DUsedQuota = cd.UsedQuota.ToData()
	cd.DAvailableQuota = cd.AvailableQuota.ToData()
	cd.DTotal = cd.Total.ToData()
	cd.DUsed = cd.Used.ToData()
}

func (cd *ClusterQuota) FromData() {
	cd.TotalQuota = BuildQuotaFromData(cd.DTotalQuota)
	cd.AssignedQuota = BuildQuotaFromData(cd.DAssignedQuota)
	cd.ChargedQuota = BuildQuotaFromData(cd.DChargedQuota)
	cd.UsedQuota = BuildQuotaFromData(cd.DUsedQuota)
	cd.AvailableQuota = BuildQuotaFromData(cd.DAvailableQuota)
	cd.Total = BuildQuotaFromData(cd.DTotal)
	cd.Used = BuildQuotaFromData(cd.DUsed)
}

func (cd *ClusterQuota) Add(other *ClusterQuota) *ClusterQuota {
	ret := ClusterQuota{}
	if other == nil {
		other = &ClusterQuota{}
	}
	cd.TotalQuota = cd.TotalQuota.Add(other.TotalQuota)
	cd.AssignedQuota = cd.AssignedQuota.Add(other.AssignedQuota)
	cd.ChargedQuota = cd.ChargedQuota.Add(other.ChargedQuota)
	cd.UsedQuota = cd.UsedQuota.Add(other.UsedQuota)
	cd.AvailableQuota = cd.AvailableQuota.Add(other.AvailableQuota)
	cd.Total = cd.Total.Add(other.Total)
	cd.Used = cd.Used.Add(other.Used)
	return &ret
}
