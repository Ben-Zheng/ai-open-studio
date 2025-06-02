package types

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const (
	AISAdmissionObjectSelectorLabelKey   = aisTypes.AISManaged
	AISAdmissionObjectSelectorLabelValue = aisTypes.AISManagedByAISIO
)

type KubeResourceName = corev1.ResourceName

const (
	// 业务侧的映射后名称
	ResourceCPU            = "cpu"
	ResourceGPU            = "gpu"
	ResourceNPU            = "npu"
	ResourceAPU            = "apu"
	ResourceVirtGPU        = "virtgpu"
	ResourceMemory         = "memory"
	ResourceSSD            = "ssd"
	ResourceHDD            = "hdd"
	ResourceStorage        = "storage" // 配额引入的固态存储
	ResourceDatasetStorage = "dataset-storage"
	ResourceAPUWs          = "apu_ws"    // ws 专用，等同于 virtgpu
	ResourceAPUOther       = "apu_other" // 除去 ws 专用，包括 gpu、npu

	// k8s 的 resourceName
	KubeResourceCPU            = corev1.ResourceCPU
	KubeResourceMemory         = corev1.ResourceMemory
	KubeResourceGPU            = corev1.ResourceName(aisTypes.NVIDIAGPUResourceName)
	KubeResourceVirtGPU        = corev1.ResourceName(aisTypes.NVIDIAVirtGPUResourceName)
	KubeResourceStorage        = corev1.ResourceName("storage")
	KubeResourceDatasetStorage = corev1.ResourceName(aisTypes.DatasetStorageResourceName)
	KubeResourceNPU            = corev1.ResourceName(aisTypes.HUAWEINPUResourceName)
)

var (
	KubeResourceGPUPrefix     = fmt.Sprintf("%s.", aisTypes.NVIDIAGPUResourceName)
	KubeResourceVirtGPUPrefix = fmt.Sprintf("%s.", aisTypes.NVIDIAVirtGPUResourceName)
	KubeResourceNPUPrefix     = fmt.Sprintf("%s.", aisTypes.HUAWEINPUResourceName)

	K8sResourceNameMap = map[KubeResourceName]bool{
		KubeResourceCPU:            true,
		KubeResourceGPU:            true,
		KubeResourceVirtGPU:        true,
		KubeResourceMemory:         true,
		KubeResourceDatasetStorage: true,
		KubeResourceStorage:        true,
	}

	PodSortMap = map[string]bool{
		ResourceCPU:     true,
		ResourceMemory:  true,
		ResourceGPU:     true,
		ResourceVirtGPU: true,
		ResourceNPU:     true,
		ResourceAPU:     true,
	}
)

type User struct {
	UserID   string `bson:"userID" json:"userID"`
	UserName string `bson:"userName" json:"userName"`
}
