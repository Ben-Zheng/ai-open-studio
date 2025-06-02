package resource

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
)

type Client struct {
	NodeSelector labels.Selector
	Clientset    *kubernetes.Clientset
	NodeLister   listerv1.NodeLister
	PodLister    listerv1.PodLister
}

type Resource struct {
	UUID        string `bson:"uuid" json:"uuid"`
	UserID      string `bson:"userID" json:"userID"`
	UserName    string `bson:"userName" json:"userName"`
	TenantID    string `bson:"tenantID" json:"tenantID"`
	TenantName  string `bson:"tenantName" json:"tenantName"`
	TenantType  string `bson:"tenantType" json:"tenantType"`
	UsedQuota   Used   `bson:"usedQuota" json:"usedQuota"` // 这里考虑改一个名字 Used
	MemberCount int64  `bson:"memberCount" json:"memberCount"`
	IsDeleted   bool   `bson:"isDeleted" json:"isDeleted"`
	CreatedAt   int64  `bson:"createdAt" json:"createdAt"`
	UpdateAt    int64  `bson:"updateAt" json:"updateAt"`
}

type Used struct {
	CPU            float64 `bson:"cpu" json:"cpu"`
	Memory         float64 `bson:"memory" json:"memory"`
	GPU            float64 `bson:"gpu" json:"gpu"`
	NPU            float64 `bson:"npu" json:"npu"`
	APU            float64 `bson:"apu" json:"apu"`
	VirtGPU        float64 `bson:"virtgpu" json:"virtgpu"`
	SSD            float64 `bson:"ssd" json:"ssd"`
	HDD            float64 `bson:"hdd" json:"hdd"`
	Storage        float64 `bson:"storage" json:"storage"`
	DatasetStorage float64 `bson:"datasetStorage" json:"datasetStorage"`
}

func NilUsed() *Used {
	return &Used{}
}
