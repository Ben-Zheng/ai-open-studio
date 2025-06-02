package quota

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	corev1Informer "k8s.io/client-go/informers/core/v1"
	coreListers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/utils"
)

const (
	resyncPromethuesPeriod      int64 = 30 * 60 // 每隔 30min 同步一次 prometheus
	tenantEtcdLockPrefix              = "/ais-quota-%s"
	tenantEtcdLockTimeout             = 3    // 获取锁的超时时间 3s
	tooManyPodsWarningThreshold       = 1000 // 在 quota controller 同步的时候，Pod 过多的告警阈值
)

type ResourceKind = string

const (
	PodsResource                   ResourceKind = "pods"
	PersistentVolumeClaimsResource ResourceKind = "persistentvolumeclaims"
	DatasetStorageResource         ResourceKind = "datasetstorage"
)

var resourcesToBeCheck = map[corev1.ResourceName]bool{
	types.KubeResourceCPU:            true,
	types.KubeResourceMemory:         true,
	types.KubeResourceGPU:            true,
	types.KubeResourceVirtGPU:        true,
	types.KubeResourceStorage:        true,
	types.KubeResourceDatasetStorage: true,
}

type ResourcesGVRMap map[ResourceKind]metav1.GroupVersionResource

var ResourcesGVRMapInstance = ResourcesGVRMap{
	PodsResource: {
		Group:    "",
		Version:  "v1",
		Resource: PodsResource,
	},
	PersistentVolumeClaimsResource: {
		Group:    "",
		Version:  "v1",
		Resource: PersistentVolumeClaimsResource,
	},
	DatasetStorageResource: {
		Group:    "aiservice.brainpp.cn",
		Version:  "v1",
		Resource: DatasetStorageResource,
	},
}

type CheckQuotaFunc func(ctx context.Context,
	obj any, ar admissionv1.AdmissionReview, server *WebhookServer, dryRun bool) *admissionv1.AdmissionResponse

var CheckQuotaFuncMap = make(map[metav1.GroupVersionResource]CheckQuotaFunc)

func CheckQuota(gvr metav1.GroupVersionResource) CheckQuotaFunc {
	return CheckQuotaFuncMap[gvr]
}

type WebhookServer struct {
	sharedInformer informers.SharedInformerFactory
	// podsLister is used to store pods.
	PodsLister coreListers.PodLister
	// podsSynced returns true if the pod store has been synced at least once.
	PodsSynced cache.InformerSynced
	// namespacesLister is used to list namespaces.
	NamespacesLister coreListers.NamespaceLister
	// namespacesSynced returns true if the namespace store has been synced at least once.
	NamespacesSynced cache.InformerSynced

	PrometheusCollector *OnDemandCollector

	quotaBindAddr   string
	metricsBindAddr string
	tlsCert         string
	tlsKey          string

	// tenantLock acquires tenant required locks and returns a cleanup method to defer
	// current based on ectd lock
	lockFunc func(string) (func(), error)
}

func NewServer(quotaBindAddr, metricsBindAddr string,
	tlsCert, tlsKey string,
	nsInformer corev1Informer.NamespaceInformer,
	podInformer corev1Informer.PodInformer,
) *WebhookServer {
	server := &WebhookServer{
		quotaBindAddr:       quotaBindAddr,
		metricsBindAddr:     metricsBindAddr,
		tlsCert:             tlsCert,
		tlsKey:              tlsKey,
		PodsLister:          podInformer.Lister(),
		PodsSynced:          podInformer.Informer().HasSynced,
		NamespacesLister:    nsInformer.Lister(),
		NamespacesSynced:    nsInformer.Informer().HasSynced,
		PrometheusCollector: NewOnDemandCollector(),
		lockFunc:            etcdLocker,
	}

	return server
}

func (ws *WebhookServer) Run(stopCh <-chan struct{}) error {
	log.Info("[Quota] Waiting for quota server caches to be synced")
	defer utilruntime.HandleCrash()
	if ok := cache.WaitForCacheSync(stopCh, ws.PodsSynced, ws.NamespacesSynced); !ok {
		panic(fmt.Errorf("failed to wait for caches to synced"))
	}
	log.Info("[Quota] Quota server caches init successfully")

	log.Info("[Quota] Starting quota servers...")
	go ws.startQuotaServer()
	go ws.startPromethusServer()

	return nil
}

func (ws *WebhookServer) startQuotaServer() {
	pair, err := tls.LoadX509KeyPair(ws.tlsCert, ws.tlsKey)
	if err != nil {
		log.Errorf("[Quota] failed to load key pair: %v", err)
	}

	server := &http.Server{
		Addr:      ws.quotaBindAddr,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/quotacheck", ws.ServeQuota)
	server.Handler = mux
	log.Info("[Quota] start quota webhook server addr: ", ws.quotaBindAddr)
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Errorf("failed to listen and serve webhook server: %v", err)
	}
}

func (ws *WebhookServer) startPromethusServer() {
	bindAddr := ws.metricsBindAddr
	registry := RegisterHealthGaugeFunc()
	registry.MustRegister(ws.PrometheusCollector)
	metricsServer, err := RegisterMetricsServer(bindAddr, registry)
	if err != nil {
		log.WithError(err).Fatal("[Quota] RegisterMetricsServer")
	}
	log.Info("[Quota] start quota metrics server addr: ", ws.metricsBindAddr)
	if err := metricsServer.ListenAndServe(); err != nil {
		log.Errorf("[Quota] failed to listen and serve prometheus metrics server: %v", err)
	}
}

// ServeQuota Serve is used to solve the request from podQuotaCheck.
func (ws *WebhookServer) ServeQuota(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	obj, gvk, err := deserializer.Decode(body, nil, nil)
	if err != nil {
		log.Errorf("[Quota] Request could not be decoded: %s", err.Error())
		http.Error(w, "request could not be decoded", http.StatusBadRequest)
		return
	}

	responseAdmissionReview := admissionv1.AdmissionReview{}
	switch *gvk {
	case admissionv1.SchemeGroupVersion.WithKind("AdmissionReview"):
		requestedAdmissionReview, ok := obj.(*admissionv1.AdmissionReview)
		if !ok {
			log.Errorf("[Quota] expected admissionv1.AdmissionReview but got: %T", obj)
			http.Error(w, "request body is not AdmissionReview", http.StatusBadRequest)
			return
		}
		responseAdmissionReview.SetGroupVersionKind(*gvk)
		responseAdmissionReview.Response = AdmitQuotaCheck(r.Context(), *requestedAdmissionReview, ws)
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
	default:
		msg := fmt.Sprintf("[Quota] unsupported group version kind: %v", gvk)
		log.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		log.WithError(err).Error("failed to marshal responseAdmissionReview")
		http.Error(w, "could not encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(respBytes); err != nil {
		log.WithError(err).Error("failed to write response bytes")
		http.Error(w, "could not write response", http.StatusInternalServerError)
		return
	}
}

func AdmitQuotaCheck(ctx context.Context, ar admissionv1.AdmissionReview, server *WebhookServer) *admissionv1.AdmissionResponse {
	reviewResponse := &admissionv1.AdmissionResponse{}
	namespace := ar.Request.Namespace
	namespaceInstance, err := server.NamespacesLister.Get(namespace)
	if err != nil {
		reviewResponse.Allowed = true
		log.Infof("[Quota] fail to get namespaceInstance[%+v]", namespace)
		return reviewResponse
	}

	// to check whether this pod needs quota check
	if namespaceInstance.Status.Phase == corev1.NamespaceTerminating {
		reviewResponse.Allowed = true
		log.Infof("[Quota] pods in this namespace[name: %+v, phase: %+v] do not have to check quota", namespace, corev1.NamespaceTerminating)
		return reviewResponse
	}

	passthrough, objIn, gvr, err := CheckAndGetObj(ar)
	if err != nil {
		return toV1beta1AdmissionDenied(err)
	}
	if passthrough {
		return v1beta1AdmissionAllowed()
	}
	return CheckQuota(gvr)(ctx, objIn, ar, server, ar.Request.DryRun != nil && *ar.Request.DryRun)
}

func CheckAndGetObj(ar admissionv1.AdmissionReview) (bool, any, metav1.GroupVersionResource, error) {
	raw := ar.Request.Object.Raw
	var obj metav1.Object
	var gvr metav1.GroupVersionResource
	switch ar.Request.Resource {
	case ResourcesGVRMapInstance[PodsResource]:
		gvr = ResourcesGVRMapInstance[PodsResource]
		obj = &corev1.Pod{}
		if _, _, err := deserializer.Decode(raw, nil, obj.(runtime.Object)); err != nil {
			log.WithError(err).Error("[Quota] fail to decode raw into pod")
			return false, nil, metav1.GroupVersionResource{}, err
		}
	case ResourcesGVRMapInstance[PersistentVolumeClaimsResource]:
		gvr = ResourcesGVRMapInstance[PersistentVolumeClaimsResource]
		obj = &corev1.PersistentVolumeClaim{}
		if _, _, err := deserializer.Decode(raw, nil, obj.(runtime.Object)); err != nil {
			log.WithError(err).Error("[Quota] fail to decode raw into pvc")
			return false, nil, metav1.GroupVersionResource{}, err
		}
		if obj.GetAnnotations()[aisTypes.AISResourceType] == string(aisTypes.AIServiceTypeWorkspace) {
			return true, nil, gvr, nil
		}
	default:
		err := fmt.Errorf("[Quota] expect gvr:%+v", ResourcesGVRMapInstance)
		log.WithError(err).Error("[Quota] invalid gvr")
		return false, nil, metav1.GroupVersionResource{}, err
	}
	passthrough, err := isValidQuotaLabels(obj)
	return passthrough, obj, gvr, err
}

func isValidQuotaLabels(obj metav1.Object) (bool, error) {
	labels := obj.GetLabels()
	if labels[aisTypes.AISQuotaLabelKey] != aisTypes.AISQuotaLabelValue {
		return true, nil
	}
	if labels[aisTypes.AISTenantName] == "" {
		return false, fmt.Errorf("[Quota] must have non empty tenantName")
	}
	if labels[aisTypes.AISUserName] == "" {
		return false, fmt.Errorf("[Quota] must have non empty userName")
	}
	return false, nil
}

func etcdLocker(tenant string) (func(), error) {
	return utils.EtcdLocker(consts.EtcdClient, fmt.Sprintf(tenantEtcdLockPrefix, tenant), tenantEtcdLockTimeout*time.Second)
}

func init() {
	CheckQuotaFuncMap = map[metav1.GroupVersionResource]CheckQuotaFunc{
		ResourcesGVRMapInstance[PodsResource]:                   CheckPodQuota,
		ResourcesGVRMapInstance[PersistentVolumeClaimsResource]: CheckPVCQuota,
	}
}
