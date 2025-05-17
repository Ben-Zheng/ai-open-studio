package mgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	isvcV1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	modeltypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/cvtmodels"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	modelUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/model"
	publicTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types"
)

var (
	deployTimeOut              int64 = 15 * 60 // 单位s
	scaleTarget                      = 80
	scaleMetricCPU                   = isvcV1beta1.MetricCPU
	InferenceServiceGVRV1beta1       = schema.GroupVersionResource{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Resource: "inferenceservices",
	}
)

func (m *Mgr) DeployKFService(gctx *ginlib.GinContext, col *mongo.Collection, inferService *types.InferenceService) (err error) {
	if inferService == nil || len(inferService.Revisions) == 0 {
		return errors.New("infer service is nil or revision is 0")
	}
	curRevision := inferService.Revisions[0]
	defer func() {
		if err != nil {
			log.WithError(err).Error("failed to deploy inferenceService, update stage to deployFailed")
			UpdateSpecificRevisionInfo(gctx, col, inferService.ID, curRevision.Revision, map[string]interface{}{
				"stage": types.InferenceServiceRevisionStageDeployFailed,
				"serviceInstances.0.status.notReadyReason": "internal error",
			})
		}
	}() //更新mongo中的inference元数据。

	if len(curRevision.ServiceInstances) == 0 || curRevision.ServiceInstances[0] == nil {
		return errors.New("service instance is nil")
	}

	// 设置 env 和 image
	curInstance := curRevision.ServiceInstances[0]
	if err := m.fillContainerParam(gctx, inferService.ID, curInstance); err != nil {
		return err
	}

	// 设置 label 和 annotation
	labels := map[string]string{
		KFServingPodsKey: KFServingPodsValue,
		publicTypes.AISAdmissionObjectSelectorLabelKey: publicTypes.AISAdmissionObjectSelectorLabelValue,
	}
	annotations := map[string]string{}
	commonLabels := &aisTypes.CommonLabels{
		UserID:       gctx.GetUserID(),
		UserName:     gctx.GetUserName(),
		TenantID:     gctx.GetAuthTenantID(),
		TenantName:   gctx.GetAuthTenantID(),
		ProjectID:    gctx.GetAuthProjectID(),
		ProjectName:  gctx.GetAuthProjectID(),
		ResourceType: aisTypes.ResourceTypeInference,
		ResourceID:   inferService.ID,
		ResourceName: fmt.Sprintf("%s/%s", inferService.Name, curRevision.Revision),
	}
	commonLabels.Inject(annotations, labels)

	// get kserveService
	kserveService := &isvcV1beta1.InferenceService{
		TypeMeta: metav1.TypeMeta{
			Kind:       InferenceServiceGVRV1beta1.Resource,
			APIVersion: InferenceServiceGVRV1beta1.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        kfServiceName(inferService),
			Namespace:   GetNamespace(gctx.GetAuthProjectID()),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: isvcV1beta1.InferenceServiceSpec{},
	}
	kserveService.Spec.Predictor = *createInferencePredictor(curInstance, m.config.SnapXInferBaseImage, m.config.Harbor.AdminPullSecretName)

	// get kserveService
	var createNew bool
	client := newKserveClient(m.DynamicClient, gctx.GetAuthProjectID())
	oldUnstructuredInstance, err := client.Get(context.TODO(), kserveService.Name, metav1.GetOptions{})
	if err != nil {
		if strings.Contains(err.Error(), types.InferenceServiceNotFound) {
			createNew = true
			err = nil
		} else {
			log.Error("failed to get inference instance")
			return err
		}
	} else {
		oldInstance, _ := unmarshalDynamic(oldUnstructuredInstance)
		kserveService.ResourceVersion = oldInstance.ResourceVersion
	}

	// create or update kserve service
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(kserveService)
	if err != nil {
		log.Error("runtime.DefaultUnstructuredConverter.ToUnstructured")
		return err
	}
	if createNew {
		if _, err = client.Create(context.TODO(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); err != nil {
			log.WithError(err).Error("failed to create kserve service")
			return inferenceErr.ErrKserve
		}
	} else {
		if _, err = client.Update(context.TODO(), &unstructured.Unstructured{Object: obj}, metav1.UpdateOptions{}); err != nil {
			log.WithError(err).Error("failed to update kserve service")
			return inferenceErr.ErrKserve
		}
	}

	// update db info
	if err = UpdateInferenceInstanceInfo(col, inferService.ID, gctx.GetAuthProjectID(), map[string]interface{}{
		FieldInstanceImageURI:          curInstance.Meta.ImageURI,
		FieldInstanceSnapSolutionName:  curInstance.Meta.SnapSolutionName,
		FieldInstanceSnapModelVersion:  curInstance.Meta.SnapModelVersion,
		FieldInstanceSnapModelProtocol: curInstance.Meta.SnapModelProtocol,
	}); err != nil {
		log.Error("failed to update instance imageURI")
		return err
	}

	return nil
}

func (m *Mgr) fillContainerParam(gc *ginlib.GinContext, resourceID string, instance *types.InferenceInstance) error {
	if err := m.fillInstanceMeta(gc, resourceID, instance); err != nil {
		return err
	}

	return nil
}

func (m *Mgr) fillInstanceMeta(gc *ginlib.GinContext, resourceID string, instance *types.InferenceInstance) error {
	// 设置oss配置
	logger := log.WithFields(log.Fields{
		"instanceID": instance.ID,
	})

	for i := range instance.Meta.GPUTypes {
		log.Infof("firstly gpu type: %s", instance.Meta.GPUTypes[i])
	}
	instance.Meta.Env = append(instance.Meta.Env, []v1.EnvVar{
		{Name: aisTypes.AISTenantEnvKey, Value: gc.GetAuthTenantID()},
		{Name: aisTypes.AISProjectEnvKey, Value: gc.GetAuthProjectID()},
		{Name: aisTypes.OSSEndpoint, Value: m.config.OssEndpoint},
		{Name: aisTypes.OSSAccessKeyID, Value: m.config.OssAccessKey},
		{Name: aisTypes.OSSAccessKeySecret, Value: m.config.OssSecretKey},
		{Name: aisTypes.AwsAccessKeyID, Value: m.config.OssAccessKey},
		{Name: aisTypes.AwsSecretAccessKey, Value: m.config.OssSecretKey},
		{Name: "GRPC_DNS_RESOLVER", Value: "native"},
		{Name: "REASON_UPDATE_URI", Value: fmt.Sprintf("http://inference-http.aiservice.svc:8080/v1/inferences/%s/reason", resourceID)},
		{Name: "RESULT_STORE_URI", Value: fmt.Sprintf("s3://%s/inference/%s", oss.GetOSSBucketName(gc.GetAuthProjectID(), oss.ComponentEvalHub), instance.ID)},
	}...)

	// 设置模型类型和格式，不再兼容其他类型的模型格式
	modelFormatType := string(instance.Meta.ModelFormatType)
	if instance.Meta.ModelParentRevision != "" {
		modelFormatType = string(modeltypes.ModelFormatTypeCM)
	} else if strings.Contains(string(instance.Meta.ModelFormatType), "TM") {
		modelFormatType = string(modeltypes.ModelFormatTypeTM)
	} else if instance.Meta.ModelFormatType == modeltypes.ModelFormatTypeONNX {
		// OCR 的 原始模型格式是 ONNX 格式，改成 TM 传递
		modelFormatType = string(modeltypes.ModelFormatTypeTM)
	}

	modelEnv := []v1.EnvVar{
		{Name: types.ModelStoreURI, Value: instance.Meta.ModelURI},
		{Name: types.ModelAppType, Value: instance.Meta.ModelAppType},
		{Name: types.ModelFormatType, Value: modelFormatType},
	}
	instance.Meta.Env = append(instance.Meta.Env, modelEnv...)

	// 设置推理服务镜像
	if instance.Meta.From == types.InferenceInstanceFromModel {
		if instance.Meta.ImageURI == "" {
			uri, gpuTypes, err := m.chooseModelServingImage(gc, instance.Meta)
			if err != nil {
				log.Error("failed to chooseModelServingImage")
				return err
			}
			instance.Meta.ImageURI = uri
			for i := range gpuTypes {
				log.Infof("model zip gpu type: %s", gpuTypes[i])
			}
		}
	}
	// SnapModelVersionV1 含义是啥
	if instance.Meta.SnapModelVersion == "" {
		instance.Meta.SnapModelVersion = types.SnapModelVersionV1
	}

	snapEnv := []v1.EnvVar{
		{Name: types.SnapSolutionName, Value: instance.Meta.SnapSolutionName},
		{Name: types.SnapModelVersion, Value: instance.Meta.SnapModelVersion},
		{Name: types.SnapModelProtocol, Value: instance.Meta.SnapModelProtocol},
		{Name: aisTypes.AISUserNameEnvKey, Value: gc.GetUserName()},
	}
	instance.Meta.Env = append(instance.Meta.Env, snapEnv...)

	if instance.Meta.ModelParentRevision != "" { // 目前通过他判断转换后模型， 等后续重构模型这里的判断
		instance.Meta.Env = append(instance.Meta.Env, cvtmodels.BuildSnapinfKorokEnv(m.config.KorokConfig)...)
		instance.Meta.Env = append(instance.Meta.Env, v1.EnvVar{
			Name:  "SNAPINF_REPORT",
			Value: "false",
		})
		dh := aisTypes.NewDeviceHelper(instance.Meta.Device)
		switch dh.Device.Identity {
		case aisTypes.DeviceEmbedded377.Identity, aisTypes.DeviceEmbeddedRV1109.Identity:
			instance.Meta.Env = append(instance.Meta.Env, v1.EnvVar{
				Name:  "SNAPINF_KOROK_CHARGED_GROUP",
				Value: "ads",
			})
		}
		cvtModel, err := cvtmodels.NewConvetedModel(instance.Meta.ModelURI, m.ossClient.GetClientSession(), logger)
		if err != nil {
			log.WithError(err).Errorf("failed new converted models: %s", instance.Meta.ModelURI)
		} else {
			instance.Meta.Env = append(instance.Meta.Env, cvtmodels.BuildSnapinfDeviceSpecEnv(m.config.SnapinfDeviceSpecs, cvtModel.Device())...)
		}
	}

	logger.WithFields(log.Fields{
		types.ModelStoreURI:   instance.Meta.ModelURI,
		types.ModelAppType:    instance.Meta.ModelAppType,
		types.ModelFormatType: modelFormatType,
		"ImageURI":            instance.Meta.ImageURI,
	}).Info("fill instance meta")

	return nil
}

func DeleteKFService(client dynamic.ResourceInterface, service *types.InferenceService) error {
	return client.Delete(context.TODO(), kfServiceName(service), metav1.DeleteOptions{})
}

func unmarshalDynamic(obj interface{}) (*isvcV1beta1.InferenceService, error) {
	res := &isvcV1beta1.InferenceService{}
	objJSON, err := obj.(*unstructured.Unstructured).MarshalJSON()
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(objJSON, res)
	return res, err
}

func (m *Mgr) updateKserveHandler(old, updated interface{}) {
	oldInstance, err := unmarshalDynamic(old)
	if err != nil {
		log.WithError(err).Fatal("updatekserveHandler for old")
	}
	newInstance, err := unmarshalDynamic(updated)
	if err != nil {
		log.WithError(err).Fatal("updatekserveHandler for updated")
	}
	l := log.WithFields(log.Fields{"id": newInstance.GetAnnotations()[aisTypes.AISResourceID], "ns": newInstance.Namespace,
		"newRvID": newInstance.ResourceVersion, "oldRvID": oldInstance.ResourceVersion,
		"newGen": newInstance.Generation, "oldGen": newInstance.Status.ObservedGeneration,
	})

	tenant, ok := newInstance.ObjectMeta.Labels[aisTypes.AISTenantID]
	if !ok {
		l.Error("Service Tenant is not found")
		return
	}
	project, ok := newInstance.ObjectMeta.Labels[aisTypes.AISProject]
	if !ok {
		l.Error("Service Project is not found")
		return
	}

	// get inferenceservice from db
	col := getInferenceCollectionByTenant(m.app.MgoClient, tenant)
	curService, err := FindInferenceService(col, project, newInstance.Annotations[aisTypes.AISResourceID])
	if err != nil {
		l.WithError(err).Error("failed to get inference service")
		return
	}
	if len(curService.Revisions) != 1 || curService.Revisions[0] == nil {
		l.Error("current revision is nil")
		return
	}
	curServiceRevision := curService.Revisions[0]
	if len(curServiceRevision.ServiceInstances) != 1 || curServiceRevision.ServiceInstances[0] == nil {
		l.Error("current infer instance is nil")
		return
	}
	if curServiceRevision.Stage != types.InferenceServiceRevisionStageServing &&
		curServiceRevision.Stage != types.InferenceServiceRevisionStageDeploying {
		l.Infof("skip it, only update state in Serving or Deploying")
		return
	}

	l = l.WithFields(log.Fields{"revision": curServiceRevision.Revision})
	if curServiceRevision.Active && curServiceRevision.Stage == types.InferenceServiceRevisionStageDeploying &&
		time.Now().Unix()-curServiceRevision.CreatedAt >= deployTimeOut {
		l.WithFields(log.Fields{"createdAt": curServiceRevision.CreatedAt}).Warnf("deploy timeout, delete inference service")
		if err := UpdateInferenceRevisionReadyAndStage(col, newInstance.GetAnnotations()[aisTypes.AISResourceID],
			project, "deploy timeout", false, types.InferenceServiceRevisionStageDeployFailed); err != nil {
			l.WithError(err).Error("failed to UpdateInferenceRevisionStage to deployFailed")
			return
		}

		if err := DeleteKFService(newKserveClient(m.DynamicClient, curService.ProjectID), curService); err != nil {
			l.WithError(err).Errorf("failed to delete inference service, id: %s", kfServiceName(curService))
		}

		return
	}
	//newInstance来自updated，curService来自数据库
	// update inference service readiness
	newServiceReady := newInstance.Status.IsReady()
	newServiceStage := curServiceRevision.Stage
	notReadyReason := ""
	if newInstance.Status.ObservedGeneration != newInstance.Generation && newServiceReady {
		l.Infof("new revision created, revision is not ready now, oldGen: %d, newGen: %d", newInstance.Status.ObservedGeneration, newInstance.Generation)
		newServiceReady = false
		newServiceStage = types.InferenceServiceRevisionStageDeploying
	}
	for _, env := range oldInstance.Spec.Predictor.PodSpec.Containers[0].Env {
		if env.Name == types.ModelStoreURI && env.Value != curServiceRevision.ServiceInstances[0].Meta.ModelURI {
			l.Infof("model changed, revision is not ready now, old URI: %s, newURI: %s", env.Value, curServiceRevision.ServiceInstances[0].Meta.ModelURI)
			newServiceReady = false
			newServiceStage = types.InferenceServiceRevisionStageDeploying
			break
		}
	}
	if !curServiceRevision.Ready && newServiceReady {
		serviceDeploy, err := m.kubeClient.AppsV1().Deployments(newInstance.Namespace).Get(context.TODO(), newInstance.Name+types.InferencePredictorDeploySuffix, metav1.GetOptions{})
		if err != nil {
			l.Infof("failed to get inference service deploy")
			return
		}
		if serviceDeploy.Status.Replicas > *(serviceDeploy.Spec.Replicas) {
			l.Infof("deploy is rollout restart, revision is not ready now, status rs: %d, spec rs: %d", serviceDeploy.Status.Replicas, *(serviceDeploy.Spec.Replicas))
			newServiceReady = false
			newServiceStage = types.InferenceServiceRevisionStageDeploying
		}
	}

	if newServiceReady {
		newServiceStage = types.InferenceServiceRevisionStageServing
	}
	if !newServiceReady {
		notReadyReason = "internal error"
	}
	if curServiceRevision.Ready != newServiceReady {
		l.Infof("revision ready filed change: %v -> %v", curServiceRevision.Ready, newServiceReady)
		if err := UpdateInferenceRevisionReadyAndStage(
			col, newInstance.GetAnnotations()[aisTypes.AISResourceID], project, notReadyReason, newServiceReady, newServiceStage); err != nil {
			l.WithError(err).Errorf("failed to update service revision ready filed to %v", newServiceReady)
			return
		}
	}

	// update service URL
	if curService.ServiceURI != newInstance.Status.URL.String() {
		labelSelector := fmt.Sprintf("%s=%s,%s=%s", KserveWebhookLabelKeyIsvc, newInstance.Name, KserveWebhookLabelKeyComponent, isvcV1beta1.PredictorComponent)
		podSvc, err := m.kubeClient.CoreV1().Services(newInstance.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			l.WithError(err).Errorf("failed to list service by %s", labelSelector)
			return
		}
		if len(podSvc.Items) != 1 {
			l.WithError(err).Errorf("service list is not one by %s", labelSelector)
			return
		}

		internalURL := fmt.Sprintf("http://%s.%s.svc.%s", podSvc.Items[0].Name, newInstance.Namespace, m.config.ClusterDomain)
		l.Infof("internalURI: %s, externalURI: %s", internalURL, newInstance.Status.URL.String())
		if err := UpdateInferenceServiceURL(col, newInstance.GetAnnotations()[aisTypes.AISResourceID], project, newInstance.Status.URL.String(), internalURL); err != nil {
			l.WithError(err).Errorf("failed to update service url to %s(%s)", newInstance.Status.URL, internalURL)
			return
		}
	}

	// update default instance
	if err := m.updateInferenceInstanceStatusIfChanged(col, newInstance, curServiceRevision.ServiceInstances[0].Status, l); err != nil {
		l.WithError(err).Error("failed to updateInferenceInstanceStatusIfChanged")
		return
	}
}

func (m *Mgr) updateInferenceInstanceStatusIfChanged(col *mongo.Collection, newService *isvcV1beta1.InferenceService,
	curInstanceStatus *types.InferenceInstanceStatus, l *log.Entry) error {
	// 1. get current instance status
	if curInstanceStatus == nil {
		curInstanceStatus = &types.InferenceInstanceStatus{}
	}

	// 2. get new instance status
	var newInstanceReady bool
	var notReadyReason string
	var instanceURI string
	conditions := newService.Status.GetConditions()
	for i := range conditions {
		if conditions[i].Type != isvcV1beta1.PredictorReady {
			continue
		}
		if conditions[i].Status == v1.ConditionTrue {
			newInstanceReady = true
			notReadyReason = ""
			instanceURI = newService.Status.Components[isvcV1beta1.PredictorComponent].URL.String()
		} else {
			newInstanceReady = false
			notReadyReason = conditions[i].Reason
		}
	}

	project, ok := newService.ObjectMeta.Labels[aisTypes.AISProject]
	if !ok {
		return fmt.Errorf("failed to get project of kfserving: %s", newService.Name)
	}

	// 3. update instance status if changed
	if newInstanceReady != curInstanceStatus.Ready {
		if err := UpdateInferenceInstanceInfo(col, newService.ObjectMeta.Name, project, map[string]interface{}{
			FieldInstanceReady:          newInstanceReady,
			FieldInstanceNotReadyReason: notReadyReason,
			FieldInstanceInstanceURI:    instanceURI,
		}); err != nil {
			l.Errorf("failed to update instance filed ready to %t, notReadyReason to %s", newInstanceReady, notReadyReason)
			return err
		}
	}

	return nil
}

func (m *Mgr) deleteKserveHandler(obj interface{}) {
	instance, err := unmarshalDynamic(obj)
	if err != nil {
		log.WithError(err).Fatal("updatekserveHandler for old")
	}
	l := log.WithFields(log.Fields{"id": instance.GetAnnotations()[aisTypes.AISResourceID], "ns": instance.Namespace})
	l.Debugf("received service delete event")

	tenant, ok := instance.ObjectMeta.Labels[aisTypes.AISTenantID]
	if !ok {
		l.Error("Service Tenant is not found")
		return
	}
	project, ok := instance.ObjectMeta.Labels[aisTypes.AISProject]
	if !ok {
		l.Error("Service Project is not found")
		return
	}

	if err := UpdateInferenceInstanceInfo(getInferenceCollectionByTenant(m.app.MgoClient, tenant),
		instance.ObjectMeta.Name, project, map[string]interface{}{
			FieldInstanceReady:          false,
			FieldInstanceInstanceURI:    "",
			FieldInstanceUpdatedAt:      time.Now().Unix(),
			FieldInstanceNotReadyReason: "instance deleted",
		}); err != nil {
		l.WithError(err).Error("failed to UpdateInferenceInstanceInfo")
		return
	}
}

func kfServiceName(service *types.InferenceService) string {
	return strings.ToLower(service.ID)
}

func newKserveClient(client dynamic.Interface, projectName string) dynamic.ResourceInterface {
	return client.Resource(InferenceServiceGVRV1beta1).Namespace(GetNamespace(projectName))
}

func (m *Mgr) newKFServingRestClient() (dynamic.Interface, error) {
	var config *restclient.Config
	var err error
	config, err = restclient.InClusterConfig()
	if err != nil {
		log.WithError(err).Error("failed to load inCluster config")
		if !m.config.LocalDebug {
			return nil, err
		}

		// Only for debug usage: try to read ~/.kube/config
		config, err = clientcmd.BuildConfigFromFlags("", m.config.LocalKubeConfigPath)
		if err != nil {
			log.WithError(err).Error("failed to load kubeConfig")
			return nil, err
		}
	}
	return dynamic.NewForConfig(restclient.AddUserAgent(config, "ais-inference"))
}

// createInferencePredictor
func createInferencePredictor(instance *types.InferenceInstance, inferBaseImage string, harborSecret string) *isvcV1beta1.PredictorSpec {
	if instance.Spec.Resource.GPU != 0 {
		if instance.Meta.Env == nil {
			instance.Meta.Env = []v1.EnvVar{}
		}
		// env required for the nvidia-container
		instance.Meta.Env = append(instance.Meta.Env, []v1.EnvVar{
			{
				Name:  "NVIDIA_DRIVER_CAPABILITIES",
				Value: "compute,utility",
			},
			{
				Name:  "NVIDIA_REQUIRE_CUDA",
				Value: "cuda>=10.1",
			},
		}...)
	}

	// set env
	envKV := make(map[string]string)
	for i := range instance.Meta.Env {
		envKV[instance.Meta.Env[i].Name] = instance.Meta.Env[i].Value
	}
	envKV["ADD_RESPONSE_HEADERS"] = InstanceIDName
	instance.Meta.Env = []v1.EnvVar{}
	for k, v := range envKV {
		instance.Meta.Env = append(instance.Meta.Env, v1.EnvVar{Name: k, Value: v})
	}

	// set command
	command := instance.Meta.EntryPoint
	if len(command) == 0 {
		command = []string{"/bin/bash", "-c", "cd /mnt/models/inferencebackends && bash run.sh"}
	}

	var gpuTypes []string
	var gpuResourceName = aisTypes.ResourceNvidiaGPU
	var gpuTypeKey = aisTypes.GPUTypeKey
	modelFormatType := instance.Meta.ModelFormatType
	if instance.Meta.Device != "RAW" {
		modelFormatType = modeltypes.ModelFormatTypeCM
	}

	if len(instance.Meta.GPUTypes) > 0 {
		gpuTypes = instance.Meta.GPUTypes
	} else if modelFormatType == modeltypes.ModelFormatTypeCM { // 转换后模型推理
		dh := aisTypes.NewDeviceHelper(instance.Meta.Device)
		if dh.BelongsTo(aisTypes.Device2080TI, aisTypes.DeviceP4, aisTypes.DeviceA2, aisTypes.DeviceA100, aisTypes.DeviceATLAS300VPRO) {
			gpuTypes = append(gpuTypes, dh.Device.Labels...)
			gpuResourceName = dh.KubeResourceName()
			if gpuResourceName == aisTypes.HuaweiAscend310PResourceName {
				gpuTypeKey = aisTypes.NPUTypeKey
			}
		}
	}

	for i := range gpuTypes {
		log.Infof("last gpu type: %s", gpuTypes[i])
	}
	var affinity *v1.Affinity
	if len(gpuTypes) != 0 {
		nodeselector := v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      gpuTypeKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   gpuTypes,
						},
					},
				},
			},
		}
		affinity = &v1.Affinity{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &nodeselector,
			},
		}
	}

	// set others
	predictor := isvcV1beta1.PredictorSpec{
		PodSpec: isvcV1beta1.PodSpec{
			Affinity: affinity,
			Containers: []v1.Container{
				{
					Name:    "predictor",
					Image:   instance.Meta.ImageURI,
					Command: command,
					Env:     instance.Meta.Env,
					Resources: v1.ResourceRequirements{
						Limits: convertResourceLimits(
							instance.Spec.Resource.CPU,
							instance.Spec.Resource.GPU,
							instance.Spec.Resource.Memory,
							gpuResourceName,
						),
						Requests: convertResourceLimits(
							instance.Spec.Resource.CPU,
							instance.Spec.Resource.GPU,
							instance.Spec.Resource.Memory,
							gpuResourceName,
						),
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      types.InitContainerShareVolumeName,
							MountPath: types.InitContainerShareVolumeMountPath,
						},
					},
				},
			},
			InitContainers: []v1.Container{
				{
					Name:  "model-server",
					Image: inferBaseImage,
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      types.InitContainerShareVolumeName,
							MountPath: types.InitContainerShareVolumeMountPath,
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: types.InitContainerShareVolumeName,
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
			ImagePullSecrets: []v1.LocalObjectReference{{Name: harborSecret}},
		},
		ComponentExtensionSpec: isvcV1beta1.ComponentExtensionSpec{
			MinReplicas: &instance.Spec.MinReplicas,
			MaxReplicas: instance.Spec.MaxReplicas,
		},
	}

	return &predictor
}

func (m *Mgr) chooseModelServingImage(gc *ginlib.GinContext, meta *types.InferenceInstanceMeta) (string, []string, error) {
	// 0. 如果是转换后模型，使用配置文件中对应镜像 (FIXME: 这里的判断不太合适)
	if meta.ModelParentRevision != "" {
		image := m.config.ConvertDeviceConfig.InftoolsBaseImage

		dh := aisTypes.NewDeviceHelper(meta.Device)
		switch dh.Device.Identity {
		case aisTypes.DeviceCPU.Identity:
			image = m.config.ConvertDeviceConfig.EvalcabinServerCPUImage
		case aisTypes.Device2080TI.Identity:
			image = m.config.ConvertDeviceConfig.EvalcabinServerCuda101Image
		case aisTypes.DeviceP4.Identity:
			image = m.config.ConvertDeviceConfig.EvalcabinServerP4Image
		case aisTypes.DeviceA2.Identity, aisTypes.DeviceA100.Identity:
			image = m.config.ConvertDeviceConfig.EvalcabinServerCuda111Image
		case aisTypes.DeviceATLAS300VPRO.Identity:
			image = m.config.ConvertDeviceConfig.EvalcabinServerAtlasImage
		}
		return image, nil, nil
	}

	// 1. 请求modelhub，获取model revision framework version
	modelRevision, err := m.modelClient.GetRevision(gc, meta.ModelID, meta.ModelRevision)
	if err != nil {
		log.Error("failed to get model revision")
		return "", nil, inferenceErr.ErrModelHub
	}

	// 2. TM 模型镜像
	if modelRevision.SolutionImageURI() != "" {
		logger := log.WithFields(log.Fields{
			"MODEL_STORE_URI": meta.ModelURI,
			"MODEL_ID":        meta.ModelID,
			"MODEL_REVISION":  meta.ModelRevision,
		})
		// 2.1 第一次获取
		snapXImageURI := modelRevision.SolutionImageURI()

		// 2.2 第二次获取
		snapModel := modelUtils.NewSnapModel(modelRevision.Meta.FileURI, m.ossClient.GetClientSession(), logger)
		if err := snapModel.DownloadAndParse(); err != nil {
			logger.WithError(err).Warn("failed to parse snap solution image url from model zip file, use image from meta revision")
			return snapXImageURI, nil, nil
		}
		if snapModel.SolutionImageURI() != "" {
			snapXImageURI = snapModel.SolutionImageURI()
		}

		// 2.3 第三次获取
		if snapModel.Meta != nil {
			meta.SnapSolutionName = snapModel.Meta.SolutionName
			meta.SnapModelVersion = snapModel.Meta.ModelVersion
			meta.SnapModelProtocol = snapModel.Protocol()
			logger = log.WithFields(log.Fields{
				"MODEL_SOLUTION_NAME": snapModel.Meta.SolutionName,
				"MODEL_PROTOCOL":      snapModel.Protocol(),
			})
		}
		if meta.SnapModelVersion == "" {
			meta.SnapModelVersion = types.SnapModelVersionV1
		}
		if meta.SnapSolutionName != "" {
			_, snapXImageURI = m.solutionImageConfig.ChooseSolutionImageURI(meta.SnapSolutionName)
		}

		logger.Infof("choose image uri: %s", snapXImageURI)
		return snapXImageURI, snapModel.GPUTypes(), nil
	}

	return "", nil, errors.New("not found available serving image")
}
