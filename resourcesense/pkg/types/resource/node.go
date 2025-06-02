package resource

type Node struct {
	UUID              string     `bson:"uuid" json:"uuid"`
	UID               string     `bson:"uid" json:"uid"`
	Name              string     `bson:"name" json:"name"`
	IPAddr            string     `bson:"ipAddr" json:"ipAddr"`
	GPUType           string     `bson:"gpuType,omitempty" json:"gpuType"`
	NPUType           string     `bson:"npuType,omitempty" json:"npuType"`
	APUType           string     `bson:"apuType,omitempty" json:"apuType"`
	CPUArch           string     `bson:"cpuArch" json:"cpuArch"`
	Total             Used       `bson:"total" json:"total"`
	Pods              []Pod      `bson:"pods" json:"pods"`
	Status            string     `bson:"status" json:"status"`
	UnSchedulable     bool       `bson:"unSchedulable" json:"unSchedulable"`
	CPU               Category   `bson:"cpu" json:"cpu"`
	Memory            Category   `bson:"memory" json:"memory"`
	GPU               Category   `bson:"gpu" json:"gpu"`
	APU               Category   `bson:"apu" json:"apu"`
	NPU               Category   `bson:"npu" json:"npu"`
	VirtGPU           Category   `bson:"virtgpu" json:"virtgpu"`
	HDDs              []Category `bson:"hdds" json:"hdds"`
	SSDs              []Category `bson:"ssds" json:"ssds"`
	IsDeleted         bool       `bson:"isDeleted" json:"isDeleted"`
	StartTimestamp    int64      `bson:"startTimestamp" json:"startTimestamp"`
	CreationTimestamp int64      `bson:"creationTimestamp" json:"creationTimestamp"`
	UpdateAt          int64      `bson:"updateAt" json:"updateAt"`
}

type Category struct {
	Name      string  `bson:"name,omitempty" json:"name,omitempty"`
	Releasing float64 `bson:"releasing" json:"releasing"`
	System    float64 `bson:"system" json:"system"`
	Usable    float64 `bson:"usable" json:"usable"`
	Used      float64 `bson:"used" json:"used"`
	Total     float64 `bson:"total" json:"total"`
}

type Pod struct {
	UID               string `bson:"uid" json:"uid"`
	Name              string `bson:"name" json:"name"`
	Resourcetype      string `bson:"resourcetype" json:"resourcetype"`
	UserID            string `bson:"userID" json:"userID"`
	UserName          string `bson:"userName" json:"userName"`
	TenantID          string `bson:"tenantID" json:"tenantID"`
	UsedQuota         Used   `bson:"usedQuota" json:"usedQuota"`
	Phase             string `bson:"phase" json:"phase"`
	IsDeleting        bool   `bson:"isDeleting" json:"isDeleting"`
	CreationTimestamp int64  `bson:"creationTimestamp" json:"creationTimestamp"`
	DeletionTimestamp int64  `bson:"deletionTimestamp,omitempty" json:"deletionTimestamp"`
}
