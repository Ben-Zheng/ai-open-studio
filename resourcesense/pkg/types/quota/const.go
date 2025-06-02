package quota

import "time"

type ResourceEventType string

const (
	ResourceEventCreate ResourceEventType = "Create"
	ResourceEventDelete ResourceEventType = "Delete"
	ResourceEventStart  ResourceEventType = "Start"
	ResourceEventFinish ResourceEventType = "Finish"
)

type KubeResourceType string

const (
	ResourcePodResource    KubeResourceType = "Pod"
	ResourcePVCResource    KubeResourceType = "PVC"
	ResourceDatasetStorage KubeResourceType = "DatasetStorage"
)

type GPUTypeLevel = string

const (
	GPUTypeLevelRare   = "Rare"
	GPUTypeLevelNormal = "Normal"
)

const (
	DatasetStorageLock = "/ais-quota-dataset_storage-%s"
	LockTimeout        = 3 * time.Second
)

type ActionType string

const (
	ActionTypeCharge = "Charge"
	ActionTypeFree   = "Free"
	ActionTypeReSync = "ReSync"
)
