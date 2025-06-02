package resource

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metascheme "k8s.io/apimachinery/pkg/apis/meta/internalversion/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/apis/common"
)

type PropertyName string

// List of all property names supported by the UI.
const (
	NameProperty              PropertyName = "name"
	CreationTimestampProperty PropertyName = "creationTimestamp"
	NamespaceProperty         PropertyName = "namespace"
	CPUProperty               PropertyName = "cpu"
	GPUProperty               PropertyName = "gpu"
	VirtGPUProperty           PropertyName = "virtgpu"
	MemoryProperty            PropertyName = "memory"
	StorageProperty           PropertyName = "storage"
	MaintenanceProperty       PropertyName = "maintenance"
)

type ResourcesSelectQuery struct {
	PaginationQuery *ginlib.Pagination
	SortQuery       *ginlib.Sort
	FilterQuery     *FilterQuery
}

type FilterQuery struct {
	// A selector based on labels
	LabelSelector labels.Selector
	// A selector based on fields
	FieldSelector fields.Selector
	// A selector based on annotation.
	AnnotationSelector *AnnotationSelector
	// A selector based on resource name
	ResourceNameSelector string
	ResourceType         []string
}

type AnnotationSelector struct {
	Annotations map[string]string
}

var NoAnnotationSelector = &AnnotationSelector{
	Annotations: map[string]string{},
}

// ListResult holds a list of objects
type ListResult struct {
	Total int      `json:"total" description:"total count"`
	Data  []Object `json:"data" description:"paging data"`
}

type Object interface {
	runtime.Object
	metav1.Object
}

func HandleListResult(r *ListResult, getAttrs GetAttrs, dataSelect *ResourcesSelectQuery, resourceType string) {
	r.resourceNameFilter(dataSelect.FilterQuery.ResourceNameSelector, resourceType)
	r.Filter(getAttrs, dataSelect.FilterQuery)
	r.Sort(dataSelect.SortQuery)
	r.Paginate(dataSelect.PaginationQuery)
}

type GetAttrs func(obj runtime.Object) (labels.Set, fields.Set, error)

func (r *ListResult) podFilter(resoursetypes, podStatus []string) {
	var filtered []Object
	var dec []Object
	for _, item := range r.Data {
		p, ok := item.(*corev1.Pod)
		if !ok || len(resoursetypes) == 0 {
			filtered = r.Data
			break
		}
		for _, resoursetype := range resoursetypes {
			if resoursetype == aisTypes.AJobCRDResourceType {
				if value, exist := p.Labels[aisTypes.ResourceTypeLabel]; exist && value == "ajob" {
					filtered = append(filtered, item)
				}
			}
		}
	}
	if len(filtered) > 0 {
		dec = filtered
	} else {
		dec = r.Data
	}
	filtered = []Object{}
	for _, item := range dec {
		p, ok := item.(*corev1.Pod)
		if !ok || len(podStatus) == 0 {
			filtered = dec
			break
		}
		for _, status := range podStatus {
			if string(p.Status.Phase) == status {
				filtered = append(filtered, item)
				break
			}
		}
	}
	r.Data = filtered
	r.Total = len(r.Data)
}

func (r *ListResult) resourceNameFilter(resourceName string, resourceType string) {
	if resourceName == "" {
		return
	}
	var filtered []Object
	for _, item := range r.Data {
		if strings.Contains(item.GetName(), resourceName) {
			filtered = append(filtered, item)
		}
	}
	r.Data = filtered
	r.Total = len(r.Data)
}

func (r *ListResult) Filter(getAttrs GetAttrs, query *FilterQuery) {
	if getAttrs == nil {
		return
	}
	var filtered []Object
	for _, item := range r.Data {
		labelSet, fieldSet, err := getAttrs(item)
		if err != nil {
			continue
		}
		if query.AnnotationSelector.Matches(item.GetAnnotations()) &&
			(labelSet == nil || query.LabelSelector.Matches(labelSet)) &&
			(fieldSet == nil || query.FieldSelector.Matches(fieldSet)) {
			filtered = append(filtered, item)
		}
	}
	r.Data = filtered
	r.Total = len(r.Data)
}

func (r *ListResult) Paginate(p *ginlib.Pagination) {
	total := int64(r.Total)
	if p.Skip > total-1 {
		r.Data = []Object{}
		return
	}
	limit := p.Limit
	if p.Skip+p.Limit > total {
		limit = total - p.Skip
	}
	r.Data = r.Data[p.Skip : p.Skip+limit]
}

func (r *ListResult) nodeMaintenanceStatus(nodeLabels map[string]string) common.NodeMaintenanceStatus {
	if nodeLabels == nil {
		return common.NodeMaintenanceNormalStatus
	}
	value, maintainExists := nodeLabels[common.MachineMaintenanceLabel]
	if maintainExists && value == common.NodeMaintenanceLabelValueTrue {
		return common.NodeMaintenanceProcessingStatus
	}
	if maintainExists && value == common.NodeMaintenanceLabelValueBooked {
		return common.NodeMaintenanceBookedStatus
	}
	return common.NodeMaintenanceNormalStatus
}

func (r *ListResult) Sort(sortBy *ginlib.Sort) {
	if sortBy == nil {
		return
	}

	sort.Slice(r.Data, func(i, j int) bool {
		var cmp int
		switch PropertyName(sortBy.By) {
		case MaintenanceProperty:
			SortDict := map[common.NodeMaintenanceStatus]int{
				common.NodeMaintenanceNormalStatus:     1,
				common.NodeMaintenanceProcessingStatus: 2,
				common.NodeMaintenanceBookedStatus:     3,
			}
			INodeMaintenanceStatus := r.nodeMaintenanceStatus(r.Data[i].GetLabels())
			JNodeMaintenanceStatus := r.nodeMaintenanceStatus(r.Data[j].GetLabels())
			cmp = SortDict[INodeMaintenanceStatus] - SortDict[JNodeMaintenanceStatus]
		case NameProperty:
			cmp = strings.Compare(r.Data[i].GetName(), r.Data[j].GetName())
		case CreationTimestampProperty:
			cmp = int(r.Data[i].GetCreationTimestamp().Sub(r.Data[j].GetCreationTimestamp().Time).Seconds())
		case NamespaceProperty:
			cmp = strings.Compare(r.Data[i].GetNamespace(), r.Data[j].GetNamespace())
		case "GPU":
			if pod, ok := r.Data[i].(*corev1.Pod); ok {
				firstGPU := resource.NewQuantity(0, resource.BinarySI)
				secondGPU := resource.NewQuantity(0, resource.BinarySI)
				for index := range pod.Spec.Containers {
					if pod.Spec.Containers[index].Resources.Requests == nil {
						continue
					}
					if value, exist := pod.Spec.Containers[index].Resources.Requests[aisTypes.NVIDIAGPUResourceName]; exist {
						firstGPU.Add(value)
					}
				}
				for index := range r.Data[j].(*corev1.Pod).Spec.Containers {
					container := r.Data[j].(*corev1.Pod).Spec.Containers[index]
					if container.Resources.Requests == nil {
						continue
					}
					if value, exist := container.Resources.Requests[aisTypes.NVIDIAGPUResourceName]; exist {
						secondGPU.Add(value)
					}
				}
				if firstGPU.Value() > secondGPU.Value() {
					cmp = 1
				} else if firstGPU.Value() == secondGPU.Value() {
					cmp = 0
				} else {
					cmp = -1
				}
			}
		case "CPU":
			if pod, ok := r.Data[i].(*corev1.Pod); ok {
				firstCPU := resource.NewQuantity(0, resource.BinarySI)
				secondCPU := resource.NewQuantity(0, resource.BinarySI)
				for index := range pod.Spec.Containers {
					if pod.Spec.Containers[index].Resources.Requests == nil {
						continue
					}
					if value := pod.Spec.Containers[index].Resources.Requests.Cpu(); value != nil {
						firstCPU.Add(*value)
					}
				}
				for index := range r.Data[j].(*corev1.Pod).Spec.Containers {
					container := r.Data[j].(*corev1.Pod).Spec.Containers[index]
					if container.Resources.Requests == nil {
						continue
					}
					if value := container.Resources.Requests.Cpu(); value != nil {
						secondCPU.Add(*value)
					}
				}
				firstCPU.Sub(*secondCPU)
				if firstCPU.Value() > 0 {
					cmp = 1
				} else if firstCPU.Value() < 0 {
					cmp = -1
				} else {
					cmp = 0
				}
			}
		case "Memory":
			if pod, ok := r.Data[i].(*corev1.Pod); ok {
				firstMemory := resource.NewQuantity(0, resource.BinarySI)
				secondMemory := resource.NewQuantity(0, resource.BinarySI)
				for index := range pod.Spec.Containers {
					if pod.Spec.Containers[index].Resources.Requests == nil {
						continue
					}
					if value := pod.Spec.Containers[index].Resources.Requests.Memory(); value != nil {
						firstMemory.Add(*value)
					}
				}
				for index := range r.Data[j].(*corev1.Pod).Spec.Containers {
					container := r.Data[j].(*corev1.Pod).Spec.Containers[index]
					if container.Resources.Requests == nil {
						continue
					}
					if value := container.Resources.Requests.Memory(); value != nil {
						secondMemory.Add(*value)
					}
				}
				if firstMemory.Value() > secondMemory.Value() {
					cmp = 1
				} else if firstMemory.Value() == secondMemory.Value() {
					cmp = 0
				} else {
					cmp = -1
				}
			}
		}

		return (cmp <= 0) == (sortBy.Order == ginlib.SortOrderAscInt)
	})
}

func (s *AnnotationSelector) Matches(annotations map[string]string) bool {
	if len(annotations) == 0 && len(s.Annotations) != 0 {
		return false
	}
	for k, v := range s.Annotations {
		if sv, exists := annotations[k]; !exists || v != sv {
			return false
		}
	}
	return true
}

func NewAnnotationSelector(annotationListRaw []string) *AnnotationSelector {
	if len(annotationListRaw) == 0 {
		return NoAnnotationSelector
	}
	s := &AnnotationSelector{Annotations: map[string]string{}}
	for _, annotationStr := range annotationListRaw {
		parts := strings.SplitN(annotationStr, "=", 2)
		if len(parts) != 2 {
			continue
		}
		s.Annotations[parts[0]] = parts[1]
	}
	return s
}

func ParseResourceSelectQuery(gc *ginlib.GinContext) (d *ResourcesSelectQuery, err error) {
	// parse k8s ListOptions
	metaOpts := metainternalversion.ListOptions{}
	if err := metascheme.ParameterCodec.DecodeParameters(gc.Request.URL.Query(), metav1.SchemeGroupVersion, &metaOpts); err != nil {
		return nil, err
	}

	if metaOpts.LabelSelector == nil {
		metaOpts.LabelSelector = labels.Everything()
	}
	if metaOpts.FieldSelector == nil {
		metaOpts.FieldSelector = fields.Everything()
	}

	return &ResourcesSelectQuery{
		PaginationQuery: gc.GetPagination(),
		SortQuery:       gc.GetSort(),
		FilterQuery: &FilterQuery{
			LabelSelector:        metaOpts.LabelSelector,
			FieldSelector:        metaOpts.FieldSelector,
			AnnotationSelector:   NewAnnotationSelector(strings.Split(gc.Query("annotationSelector"), ",")),
			ResourceType:         GetResourceType(gc),
			ResourceNameSelector: gc.Query("resourceNameSelector"),
		},
	}, nil
}

func GetResourceType(gc *ginlib.GinContext) []string {
	types := gc.QueryArray("resourcetype")
	return types
}
