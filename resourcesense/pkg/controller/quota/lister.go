package quota

import (
	"fmt"
	"strconv"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	coreListers "k8s.io/client-go/listers/core/v1"

	datahubv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/dataselect"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

type PodLister struct {
	lister     coreListers.PodLister
	nodeLister coreListers.NodeLister
}

type PVCLister struct {
	lister coreListers.PersistentVolumeClaimLister
}

type workloadsLister struct {
	podLister     *PodLister
	pvcLister     *PVCLister
	datasetClient datahubv1.Client
}

func (wl *workloadsLister) ListPodWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sorter *ginlib.Sort) (int64, []*quota.Workload, error) {
	pods, err := wl.podLister.ListPodsByTenant(tenantName)
	if err != nil {
		return 0, nil, err
	}
	rareGPUType, err := GetRareGPUTypeMapWithFeatureGate(ginlib.NewMockGinContext())
	if err != nil {
		return 0, nil, err
	}

	result := wl.podLister.MakeResults(pods, query, dataselect.PodSelector{
		Phases: []string{"Running"},
	}, pagination, rareGPUType)
	return result.Total, result.Data, nil
}

func (wl *workloadsLister) ListPVCWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sorter *ginlib.Sort) (int64, []*quota.Workload, error) {
	pvcs, err := wl.pvcLister.ListPVCsByTenant(tenantName)
	if err != nil {
		return 0, nil, err
	}
	result := wl.pvcLister.MakeResults(pvcs, query, dataselect.PVCSelector{
		Phases: []string{},
	}, pagination)
	return result.Total, result.Data, nil
}

func (wl *workloadsLister) ListAllWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sorter *ginlib.Sort) (int64, []*quota.Workload, error) {
	pods, err := wl.podLister.ListPodsByTenant(tenantName)
	if err != nil {
		return 0, nil, err
	}
	rareGPUType, err := GetRareGPUTypeMapWithFeatureGate(ginlib.NewMockGinContext())
	if err != nil {
		return 0, nil, err
	}

	podWorkloads := wl.podLister.MakeResults(pods, query, dataselect.PodSelector{
		Phases: []string{string(corev1.PodRunning), string(corev1.PodPending)},
	}, nil, rareGPUType)

	pvcs, err := wl.pvcLister.ListPVCsByTenant(tenantName)
	if err != nil {
		return 0, nil, err
	}
	pvcWorkloads := wl.pvcLister.MakeResults(pvcs, query, dataselect.PVCSelector{
		Phases: []string{},
	}, nil)

	datasetWorkloads := &dataselect.ListResult[*quota.Workload]{}
	if features.IsQuotaSupportDatasetStorageEnabled() {
		datasets, err := wl.datasetClient.ListDatasetForStorageQuota(tenantName, 0, 2047483647)
		if err != nil {
			log.WithError(err).Errorf("faild to ListDatasetForStorageQuota: %s", tenantName)
			return 0, nil, err
		}
		datasetWorkloads = MakeResultsForDataset(query, tenantName, datasets.Data)
	}

	allWorkloads := &dataselect.ListResult[*quota.Workload]{
		Total: podWorkloads.Total + pvcWorkloads.Total + datasetWorkloads.Total,
		Data:  append(podWorkloads.Data, pvcWorkloads.Data...),
	}
	allWorkloads.Data = append(allWorkloads.Data, datasetWorkloads.Data...)
	cmpFunc := func(i, j *quota.Workload) int {
		return -int(i.CreatedAt - j.CreatedAt)
	}
	allWorkloads = allWorkloads.Sort(cmpFunc).Paginate(pagination)
	return allWorkloads.Total, allWorkloads.Data, nil
}

func NewWorkloadsLister(podLister coreListers.PodLister, pvcLister coreListers.PersistentVolumeClaimLister,
	nodeLister coreListers.NodeLister, datasetClient datahubv1.Client) *workloadsLister {
	return &workloadsLister{
		podLister: &PodLister{
			lister:     podLister,
			nodeLister: nodeLister,
		},
		pvcLister: &PVCLister{
			lister: pvcLister,
		},
		datasetClient: datasetClient,
	}
}

func (pl *PodLister) ListPodsByTenant(tenantName string) ([]*corev1.Pod, error) {
	selectLabels := map[string]string{
		aisTypes.AISQuotaLabelKey: aisTypes.AISQuotaLabelValue,
		aisTypes.AISManaged:       aisTypes.AISManagedByAISIO,
		aisTypes.AISTenantName:    tenantName,
	}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: selectLabels,
	})
	if err != nil {
		log.WithError(err).Error("Failed to build pod selector")
		return nil, err
	}
	pods, err := pl.lister.List(selector)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = fmt.Errorf("list replica object failed")
		}
		log.WithError(err).Error("Failed to list pods")
		return nil, err
	}
	log.Infof("list found %d pods by tenant: %s", len(pods), tenantName)
	return pods, nil
}

func (pl *PodLister) MakeResults(pods []*corev1.Pod, query *dataselect.FilterQuery, selector dataselect.PodSelector, pagination *ginlib.Pagination, rareGPUType map[string]bool) *dataselect.ListResult[*quota.Workload] {
	r := &dataselect.ListResult[*corev1.Pod]{
		Total: int64(len(pods)),
		Data:  pods,
	}
	cmpFunc := func(i, j *corev1.Pod) int {
		return -int(i.CreationTimestamp.Unix() - j.CreationTimestamp.Unix())
	}
	r.Filter(selector.Filter).
		Filter(dataselect.BuildResourceNameFilter[*corev1.Pod](query.ResourceNameSelector)).
		Filter(dataselect.BuildAttrsFilter[*corev1.Pod](pl.getAttrs, query)).
		Sort(cmpFunc).
		Paginate(pagination)
	workloads := pl.TransformPodsToWorkloads(r.Data, rareGPUType)
	return &dataselect.ListResult[*quota.Workload]{
		Data:  workloads,
		Total: r.Total,
	}
}

func (pl *PodLister) getAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	w, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, nil, fmt.Errorf("not an pod")
	}
	return w.Labels, generic.ObjectMetaFieldsSet(&w.ObjectMeta, true), nil
}

func (pl *PodLister) TransformPodsToWorkloads(pods []*corev1.Pod, rareGPULevel map[string]bool) []*quota.Workload {
	workloads := make([]*quota.Workload, 0)
	for _, pod := range pods {
		lbs := pod.GetLabels()
		annotations := pod.GetAnnotations()
		chargedQuota := quota.NilQuota().Add(GetPodUsageToQuota(pod, rareGPULevel))
		workload := quota.Workload{
			ResourceType:       quota.ResourcePodResource,
			ResourceName:       pod.Name,
			CustomResourceType: annotations[aisTypes.AISResourceType],
			CustomResourceName: annotations[aisTypes.AISResourceName],
			CustomResourceID:   annotations[aisTypes.AISResourceID],
			DChargedQuota:      chargedQuota.ToData(),
			ChargedQuota:       chargedQuota,
			TenantName:         lbs[aisTypes.AISTenantName],
			ProjectName:        lbs[aisTypes.AISProjectName],
			Creator:            lbs[aisTypes.AISUserName],
			CreatorID:          lbs[aisTypes.AISUserID],
			CreatedAt:          pod.GetCreationTimestamp().Unix(),
		}

		if pod.Spec.NodeName != "" && CheckIfPodUseGPU(pod) {
			node, err := pl.nodeLister.Get(pod.Spec.NodeName)
			if err != nil {
				continue
			}
			workload.Node = &quota.WorkloadNode{
				Name:    node.Name,
				GPUType: types.GetNodeGPUTypeLabel(node),
			}
		}
		workloads = append(workloads, &workload)
	}
	return workloads
}

func (pl *PVCLister) ListPVCsByTenant(tenantName string) ([]*corev1.PersistentVolumeClaim, error) {
	selectLabels := map[string]string{
		aisTypes.AISQuotaLabelKey: aisTypes.AISQuotaLabelValue,
		aisTypes.AISManaged:       aisTypes.AISManagedByAISIO,
		aisTypes.AISTenantName:    tenantName,
	}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: selectLabels,
	})
	if err != nil {
		log.WithError(err).Error("Failed to build pvc selector")
		return nil, err
	}
	pods, err := pl.lister.List(selector)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = fmt.Errorf("list replica object failed")
		}
		log.WithError(err).Error("Failed to list pods")
		return nil, err
	}
	log.Infof("list found %d pvcs by tenant: %s", len(pods), tenantName)
	return pods, nil
}

func (pl *PVCLister) MakeResults(pvcs []*corev1.PersistentVolumeClaim, query *dataselect.FilterQuery, selector dataselect.PVCSelector, pagination *ginlib.Pagination) *dataselect.ListResult[*quota.Workload] {
	r := &dataselect.ListResult[*corev1.PersistentVolumeClaim]{
		Total: int64(len(pvcs)),
		Data:  pvcs,
	}
	cmpFunc := func(i, j *corev1.PersistentVolumeClaim) int {
		return -int(i.CreationTimestamp.Unix() - j.CreationTimestamp.Unix())
	}
	r.Filter(selector.Filter).
		Filter(dataselect.BuildResourceNameFilter[*corev1.PersistentVolumeClaim](query.ResourceNameSelector)).
		Filter(dataselect.BuildAttrsFilter[*corev1.PersistentVolumeClaim](pl.getAttrs, query)).
		Filter(dataselect.BuildResourceTypeFilter[*corev1.PersistentVolumeClaim]()).
		Sort(cmpFunc).
		Paginate(pagination)
	workloads := pl.TransformPVCsToWorkloads(r.Data)
	return &dataselect.ListResult[*quota.Workload]{
		Data:  workloads,
		Total: r.Total,
	}
}

func (pl *PVCLister) getAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	w, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil, nil, fmt.Errorf("not an pvc")
	}
	return w.Labels, generic.ObjectMetaFieldsSet(&w.ObjectMeta, true), nil
}

func (pl *PVCLister) TransformPVCsToWorkloads(pvcs []*corev1.PersistentVolumeClaim) []*quota.Workload {
	workloads := make([]*quota.Workload, 0)
	for _, pvc := range pvcs {
		lbs := pvc.GetLabels()
		annotations := pvc.GetAnnotations()
		chargedQuota := quota.NilQuota().Add(GetPVCUsageToQuota(pvc))
		workload := quota.Workload{
			ResourceType:       quota.ResourcePVCResource,
			ResourceName:       pvc.Name,
			CustomResourceType: annotations[aisTypes.AISResourceType],
			CustomResourceName: annotations[aisTypes.AISResourceName],
			CustomResourceID:   annotations[aisTypes.AISResourceID],
			DChargedQuota:      chargedQuota.ToData(),
			ChargedQuota:       chargedQuota,
			TenantName:         lbs[aisTypes.AISTenantName],
			ProjectName:        lbs[aisTypes.AISProjectName],
			Creator:            lbs[aisTypes.AISUserName],
			CreatorID:          lbs[aisTypes.AISUserID],
			CreatedAt:          pvc.GetCreationTimestamp().Unix(),
		}
		workloads = append(workloads, &workload)
	}
	return workloads
}

func (wl *workloadsLister) ListDatasetWorkloads(tenantName string, query *dataselect.FilterQuery, pagination *ginlib.Pagination, sorter *ginlib.Sort) (int64, []*quota.Workload, error) {
	datasets, err := wl.datasetClient.ListDatasetForStorageQuota(tenantName, pagination.Skip, pagination.Limit)
	if err != nil {
		log.WithError(err).Errorf("faild to ListDatasetForStorageQuota: %s", tenantName)
	}

	res := MakeResultsForDataset(query, tenantName, datasets.Data)
	return res.Total, res.Data, nil
}

func MakeResultsForDataset(query *dataselect.FilterQuery, tenantName string, datasets []*ctx.SimpleDatasetForQuotaStorage) *dataselect.ListResult[*quota.Workload] {
	var total int64
	workloads := make([]*quota.Workload, 0, len(datasets))

	for i := range datasets {
		for j := range datasets[i].Revisions {
			if query.AnnotationSelector.Annotations[aisTypes.AISResourceType] != "" && query.AnnotationSelector.Annotations[aisTypes.AISResourceType] != string(aisTypes.ResourceTypeDataset) {
				continue
			}

			total++
			workloads = append(workloads, &quota.Workload{
				ResourceType:       quota.ResourceDatasetStorage,
				ResourceName:       datasets[i].Name + "/" + datasets[i].Revisions[j].RevisionName,
				CustomResourceType: string(aisTypes.ResourceTypeDataset),
				CustomResourceID:   datasets[i].Revisions[j].RevisionID.Hex(),
				CustomResourceName: datasets[i].Name + "/" + datasets[i].Revisions[j].RevisionName,
				ChargedQuota:       quota.Quota{types.KubeResourceDatasetStorage: k8sresource.MustParse(strconv.FormatInt(datasets[i].Revisions[j].MetaStat.StorageQuota, 10))},
				DChargedQuota:      quota.QuotaData{types.KubeResourceDatasetStorage: float64(datasets[i].Revisions[j].MetaStat.StorageQuota)},
				TenantName:         tenantName,
				ProjectName:        datasets[i].ProjectID,
				Creator:            datasets[i].Revisions[j].CreatedBy,
				CreatedAt:          datasets[i].Revisions[j].CreatedAt / 1000,
			})
		}
	}
	return &dataselect.ListResult[*quota.Workload]{
		Data:  workloads,
		Total: total,
	}
}
