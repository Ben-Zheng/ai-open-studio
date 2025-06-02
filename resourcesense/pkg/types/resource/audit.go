package resource

type Audit struct {
	TenantID     string         `json:"tenantID" bson:"tenantID"`
	TenantName   string         `json:"tenantName" bson:"-"`
	UserID       string         `json:"userID" bson:"userID"`
	ProjectID    string         `json:"projectID" bson:"projectID,omitempty"`
	Resource     string         `json:"resource" bson:"resource,omitempty"`
	ResourceName string         `json:"resourceName" bson:"resourceName,omitempty"`
	Operation    string         `json:"operation" bson:"operation,omitempty"`
	Result       string         `json:"result" bson:"result,omitempty"`
	Timestamp    int64          `json:"timestamp" bson:"timestamp,omitempty"`
	Datasets     []*DatasetMeta `json:"datasets" bson:"datasets"` // 资源使用的数据集

	UserName string `json:"userName" bson:"-"`
}

type DatasetMeta struct {
	Name  string `json:"name" bson:"-"`
	ID    string `json:"id" bson:"id"`
	RvID  string `json:"rvID" bson:"rvID"`
	Level string `json:"level" bson:"level"`
}
