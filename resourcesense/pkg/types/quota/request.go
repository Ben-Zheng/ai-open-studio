package quota

import (
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
)

type GPUGroupUpdateReq struct {
	GPUGroups []*GPUGroup `json:"gpuGroups"`
}

type DatasetStorageQuota struct {
	DatasourceID         string `json:"datasourceID"`
	DatasourceName       string `json:"datasourceName"`
	RequestQuotaQuantity string `json:"requestQuotaQuantity"`
	RequestQuota         Quota  `json:"-"`
}

type TenantChargedQuotaUpdateReq struct {
	ResourceName             types.KubeResourceName `json:"resourceName"`
	ActionType               ActionType             `json:"actionType"`
	DatasetName              string                 `json:"datasetName"`
	RevisionName             string                 `json:"revisionName"`
	DatasetStorageQuota      []*DatasetStorageQuota `json:"datasetStorageQuota"`
	TotalDatasetStorageQuota Quota                  `json:"-"`
}
