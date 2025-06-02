package algoresource

import "go.mongodb.org/mongo-driver/bson/primitive"

type TenantAlgo struct {
	ID         primitive.ObjectID `bson:"_id" json:"-"`
	TenantID   string             `bson:"tenantID" json:"tenantID"`
	TenantName string             `bson:"tenantName" json:"tenantName"`
	AlgoCards  []*Content         `bson:"algoCards" json:"algoCards"` // 算法卡片
	Platforms  []*Content         `bson:"platforms" json:"platforms"` // 适用平台
	CreatedAt  int64              `bson:"createdAt" json:"createdAt"`
	UpdatedAt  int64              `bson:"updatedAt" json:"updatedAt"`
	IsDeleted  bool               `bson:"isDeleted" json:"-"`

	MemberCount int64 `bson:"-" json:"memberCount"`
}

type Content struct {
	ID   string `bson:"id" json:"id"`
	Name string `bson:"name" json:"name"`
}

// 分配表格
type TenantAlgoAudit struct {
	ID         primitive.ObjectID `bson:"_id" json:"-"`
	TenantID   string             `bson:"tenantID" json:"tenantID"`
	TenantName string             `bson:"tenantName" json:"tenantName"`
	AlgoType   AlgoType           `bson:"algoType" json:"algoType"`
	Records    []*Content         `bson:"records" json:"records"`     // 增加的算法卡片或适用平台
	CreatorID  string             `bson:"creatorID" json:"creatorID"` // 分配者ID
	Creator    string             `bson:"creator" json:"creator"`     // 分配者名
	CreatedAt  int64              `bson:"createdAt" json:"createdAt"` // 分配时间
}

type AlgoType string

const (
	AlgoCard AlgoType = "AlgoCard"
	Platform AlgoType = "Platform"
)

type UpdateRequest struct {
	AlgoType AlgoType   `json:"algoType"`
	Records  []*Content `json:"records"`
}

// 分配表格
type TenantProjectAlgo struct {
	ID          primitive.ObjectID `bson:"_id" json:"-"`
	TenantID    string             `bson:"tenantID" json:"tenantID"`
	ProjectID   string             `bson:"projectID" json:"projectID"`
	AlgoType    AlgoType           `bson:"algoType" json:"algoType"`
	RecordID    string             `bson:"recordID" json:"recordID"`       // 增加的算法卡片或适用平台
	NewRecordID string             `bson:"newRecordID" json:"newRecordID"` // 分配后的新ID codeID 或者 algocardID
	CreatorID   string             `bson:"creatorID" json:"creatorID"`     // 分配者ID
	Creator     string             `bson:"creator" json:"creator"`         // 分配者名
	CreatedAt   int64              `bson:"createdAt" json:"createdAt"`     // 分配时间
	IsDeleted   bool               `bson:"isDeleted" json:"-"`
}
