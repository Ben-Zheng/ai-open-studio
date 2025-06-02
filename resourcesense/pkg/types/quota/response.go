package quota

type ListEventsResponse struct {
	Items []*ResourceEvent `json:"items"`
	Total int64            `json:"total"`
}

type ListGPUGroupResp struct {
	GPUTypeLevelGroup []string `json:"gpuTypeLevelGroup"`
}

type ListTenantDetailsResponse struct {
	Total int64           `json:"total"`
	Items []*TenantDetail `json:"items"`
}

type ListTenantUsersResponse struct {
	Total int64         `json:"total"`
	Items []*UserDetail `json:"items"`
}

type ListTenantWorkloadsResponse struct {
	Total int64       `json:"total"`
	Items []*Workload `json:"items"`
}
