package resource

type Bill struct {
	UserID            string `bson:"userID" json:"userID"`
	TenantID          string `bson:"tenantID" json:"tenantID"`
	Accumulate        Used   `bson:"accumulate" json:"accumulate"`
	AccumulateAdvance Used   `bson:"accumulateAdvance" json:"accumulateAdvance"`
	CreatedAt         int64  `bson:"createdAt" json:"createdAt"`
	UpdateAt          int64  `bson:"updateAt" json:"updateAt"`
	IsCorrected       bool   `bson:"isCorrected" json:"isCorrected"` // 校正错误数据
}

type Charge struct {
	UserID    string `bson:"userID" json:"userID"`
	TenantID  string `bson:"tenantID" json:"tenantID"`
	UsedQuota Used   `json:"usedQuota" bson:"usedQuota"`
	RunTime   int64  `json:"runTime" bson:"runTime"`               // 使用时长：秒
	Timestamp int64  `json:"timestamp" bson:"timestamp,omitempty"` // 记录时时间戳
}
