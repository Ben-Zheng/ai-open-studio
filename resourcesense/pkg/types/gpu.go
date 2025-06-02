package types

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const (
	AisNormalGPUType = "AisNormal"
)

func ResourceNameWithGPUType(gpuType string) corev1.ResourceName {
	return corev1.ResourceName(fmt.Sprintf("%s.%s", aisTypes.NVIDIAGPUResourceName, gpuType))
}

func ResourceNameWithVirtGPUType(gpuType string) corev1.ResourceName {
	return corev1.ResourceName(fmt.Sprintf("%s.%s", aisTypes.NVIDIAVirtGPUResourceName, gpuType))
}

func ResourceNameWithNPUType(gpuType string) corev1.ResourceName {
	return corev1.ResourceName(fmt.Sprintf("%s.%s", aisTypes.HUAWEINPUResourceName, gpuType))
}

func IsGPUResourceName(resourceName corev1.ResourceName) bool {
	return strings.HasPrefix(resourceName.String(), KubeResourceGPUPrefix)
}

func IsVirtGPUResourceName(resourceName corev1.ResourceName) bool {
	return strings.HasPrefix(resourceName.String(), KubeResourceVirtGPUPrefix)
}

func IsNPUResourceName(resourceName corev1.ResourceName) bool {
	return strings.HasPrefix(resourceName.String(), KubeResourceNPUPrefix)
}

func GPUTypeFromResourceName(resourceName corev1.ResourceName) string {
	if !IsGPUResourceName(resourceName) {
		return ""
	}
	return strings.TrimPrefix(resourceName.String(), KubeResourceGPUPrefix)
}

func VirtGPUTypeFromResourceName(resourceName corev1.ResourceName) string {
	if !IsVirtGPUResourceName(resourceName) {
		return ""
	}
	return strings.TrimPrefix(resourceName.String(), KubeResourceVirtGPUPrefix)
}

func NPUTypeFromResourceName(resourceName corev1.ResourceName) string {
	if !IsNPUResourceName(resourceName) {
		return ""
	}
	return strings.TrimPrefix(resourceName.String(), KubeResourceNPUPrefix)
}

func GetNodeGPUTypeLabel(node *corev1.Node) string {
	return node.Labels[aisTypes.GPUTypeKey]
}
