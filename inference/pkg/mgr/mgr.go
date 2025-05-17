package mgr

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/penglongli/gin-metrics/ginmetrics"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	modelhubapi "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/api"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/cvtmodels"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginapp"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	quotaGroup "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/quotagroup"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/snap"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

type Config struct {
	OssEndpoint            string                     `yaml:"ossEndpoint"`
	OssAccessKey           string                     `yaml:"ossAccessKey"`
	OssSecretKey           string                     `yaml:"ossSecretKey"`
	KserveResyncSeconds    int32                      `yaml:"kserveResyncSeconds"`
	LocalDebug             bool                       `yaml:"localDebug"`
	LocalKubeConfigPath    string                     `yaml:"localKubeConfigPath"`
	LogLevel               string                     `yaml:"logLevel"`
	ModelhubURL            string                     `yaml:"modelhubURL"`
	SolutionHub            string                     `yaml:"solutionHub"`
	QuotaGroupConf         *quotaGroup.Config         `yaml:"quotaGroupConf"`
	APILogRemainSeconds    int32                      `yaml:"apiRemainSeconds"`
	PodNamesRemainSeconds  int32                      `yaml:"podNamesRemainSeconds"`
	DefaultReplicaResource types.Resource             `yaml:"defaultReplicaResource"`
	DefaultAIGCResource    map[string]*types.Resource `yaml:"defaultAIGCResource"`
	ClusterDomain          string                     `json:"clusterDomain" yaml:"clusterDomain"`
	SnapXInferBaseImage    string                     `json:"snapXInferBaseImage" yaml:"snapXInferBaseImage"`
	ConvertDeviceConfig    ConvertDeviceConfig        `json:"convertDeviceConfig" yaml:"convertDeviceConfig"`
	Harbor                 aisTypes.Harbor            `json:"harbor" yaml:"harbor"`
	KorokConfig            *aisTypes.KorokConfig      `json:"korokConfig" yaml:"korokConfig"`
	SnapinfDeviceSpecs     map[string]string          `json:"snapinfDeviceSpecs" yaml:"snapinfDeviceSpecs"`
	MaxToken               int64                      `json:"maxToken" yaml:"maxToken"`
	StopCh                 <-chan struct{}
}

type ConvertDeviceConfig struct {
	Resource                    types.Resource `json:"resource" yaml:"resource"`
	EvalcabinServerCuda101Image string         `json:"evalcabinServerCuda101Image" yaml:"evalcabinServerCuda101Image"`
	EvalcabinServerCuda111Image string         `json:"evalcabinServerCuda111Image" yaml:"evalcabinServerCuda111Image"`
	EvalcabinServerCPUImage     string         `json:"evalcabinServerCPUImage" yaml:"evalcabinServerCPUImage"`
	EvalcabinServerAtlasImage   string         `json:"evalcabinServerAtlasImage" yaml:"evalcabinServerAtlasImage"`
	EvalcabinServerP4Image      string         `json:"evalcabinServerP4Image" yaml:"evalcabinServerP4Image"`
	InftoolsBaseImage           string         `json:"inftoolsBaseImage" yaml:"inftoolsBaseImage"`
}

type Mgr struct {
	app                 *ginapp.App
	config              *Config
	DynamicClient       dynamic.Interface
	DynamicFactory      dynamicinformer.DynamicSharedInformerFactory
	KubeInformerFactory informers.SharedInformerFactory
	kubeClient          *clientset.Clientset
	podsMap             sync.Map
	modelClient         modelhubapi.Client
	quotaClient         *quotaGroup.Client
	metrics             *ginmetrics.Monitor
	solutionImageConfig *snap.SolutionImageConfig
	ossClient           *oss.Client
}

func NewMgr(app *ginapp.App, config *Config) (*Mgr, error) {
	var err error
	mgr := &Mgr{
		app:    app,
		config: config,
	}

	config.StopCh = make(chan struct{})
	if config.DefaultReplicaResource.CPU == 0 {
		config.DefaultReplicaResource.CPU = DefaultResourceCPU
	}
	if config.DefaultReplicaResource.Memory == 0 {
		config.DefaultReplicaResource.Memory = DefaultResourceMemory
	}
	if config.DefaultReplicaResource.GPU == 0 {
		config.DefaultReplicaResource.GPU = DefaultResourceGPU
	}

	if config.ConvertDeviceConfig.Resource.CPU == 0 {
		config.ConvertDeviceConfig.Resource.CPU = ConvertResourceCPU
	}
	if config.ConvertDeviceConfig.Resource.Memory == 0 {
		config.ConvertDeviceConfig.Resource.Memory = ConvertResourceMemory
	}
	if config.ConvertDeviceConfig.Resource.GPU == 0 {
		config.ConvertDeviceConfig.Resource.GPU = ConvertResourceGPU
	}

	if config.DefaultAIGCResource == nil {
		config.DefaultAIGCResource = make(map[string]*types.Resource)
	}
	if config.DefaultAIGCResource["llama3_llama3_8b_full"] == nil {
		config.DefaultAIGCResource["llama3_llama3_8b_full"] = &types.Resource{
			GPU:    2,
			CPU:    2,
			Memory: 30,
		}
	}
	if config.DefaultAIGCResource["llama3_llama3_8b_freeze"] == nil {
		config.DefaultAIGCResource["llama3_llama3_8b_freeze"] = &types.Resource{
			GPU:    2,
			CPU:    2,
			Memory: 8,
		}
	}

	if config.DefaultAIGCResource["llama3_llama3_8b_lora"] == nil {
		config.DefaultAIGCResource["llama3_llama3_8b_lora"] = &types.Resource{
			GPU:    2,
			CPU:    2,
			Memory: 6,
		}
	}
	if config.DefaultAIGCResource["qwen2_qwen2_1_8b_lora"] == nil {
		config.DefaultAIGCResource["qwen2_qwen2_1_8b_lora"] = &types.Resource{
			GPU:    1,
			CPU:    2,
			Memory: 10,
		}
	}
	if config.DefaultAIGCResource["qwen2_qwen2_1_8b_freeze"] == nil {
		config.DefaultAIGCResource["qwen2_qwen2_1_8b_freeze"] = &types.Resource{
			GPU:    1,
			CPU:    2,
			Memory: 10,
		}
	}
	if config.DefaultAIGCResource["qwen2_qwen2_1_8b_full"] == nil {
		config.DefaultAIGCResource["qwen2_qwen2_1_8b_full"] = &types.Resource{
			GPU:    1,
			CPU:    2,
			Memory: 10,
		}
	}
	if config.DefaultAIGCResource["DEFAULT_lora"] == nil {
		config.DefaultAIGCResource["DEFAULT_lora"] = &types.Resource{
			GPU:    2,
			CPU:    5,
			Memory: 20,
		}
	}
	if config.DefaultAIGCResource["DEFAULT_freeze"] == nil {
		config.DefaultAIGCResource["DEFAULT_freeze"] = &types.Resource{
			GPU:    2,
			CPU:    5,
			Memory: 20,
		}
	}
	if config.DefaultAIGCResource["DEFAULT_full"] == nil {
		config.DefaultAIGCResource["DEFAULT_full"] = &types.Resource{
			GPU:    2,
			CPU:    5,
			Memory: 50,
		}
	}
	if config.KserveResyncSeconds == 0 {
		config.KserveResyncSeconds = 20
	}
	if config.ClusterDomain == "" {
		config.ClusterDomain = "kubebrain.local"
	}
	if config.SnapXInferBaseImage == "" {
		log.Error("snapXInferBaseImage is null")
		return nil, errors.New("snapXInferBaseImage is null")
	}
	if config.Harbor.AdminPullSecretName == "" {
		config.Harbor.AdminPullSecretName = "harbor-admin-pull-secret"
	}

	kubeConfig, err := clientcmd.BuildConfigFromFlags("", mgr.config.LocalKubeConfigPath)
	if err != nil {
		return nil, err
	}

	// 0. oss client
	mgr.ossClient, err = oss.NewClient(&oss.Config{
		AccessKey: config.OssAccessKey,
		SecretKey: config.OssSecretKey,
		Endpoint:  config.OssEndpoint,
	}, false)
	if err != nil {
		log.WithError(err).Error("failed to create oss client")
		return nil, errors.New("failed to create oss client")
	}

	if err := cvtmodels.InitKorokConfigS3Path(config.KorokConfig, mgr.ossClient); err != nil {
		log.WithError(err).Errorf("failed to init korok s3 path: %s", config.KorokConfig.S3Path)
		return nil, err
	}

	// 1. dynamic client and kserve informer factory
	mgr.DynamicClient, err = dynamic.NewForConfig(restclient.AddUserAgent(kubeConfig, "ais-inference-isvc"))
	if err != nil {
		log.WithError(err).Error("failed to connect to kfserving")
		return nil, err
	}
	mgr.DynamicFactory = dynamicinformer.NewDynamicSharedInformerFactory(mgr.DynamicClient, time.Duration(config.KserveResyncSeconds)*time.Second)

	// 2. kserve pod informer factory
	mgr.kubeClient, err = clientset.NewForConfig(restclient.AddUserAgent(kubeConfig, "ais-inference-pod"))
	if err != nil {
		return nil, err
	}

	// 3. only watch pods of kserve instances
	mgr.KubeInformerFactory = informers.NewSharedInformerFactoryWithOptions(
		mgr.kubeClient,
		time.Duration(config.KserveResyncSeconds)*time.Second,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labels.Set(map[string]string{KFServingPodsKey: KFServingPodsValue}).String()
		}),
	)

	mgr.modelClient = modelhubapi.NewClient(config.ModelhubURL)

	mgr.quotaClient, err = quotaGroup.NewClient(config.QuotaGroupConf)
	if err != nil {
		log.WithError(err).Error("failed to init quota client")
		return nil, err
	}

	mgr.metrics = mgr.RegisterMetricsService(app.Engine)

	mgr.solutionImageConfig, err = snap.NewSolutionImageConfig(config.SolutionHub)
	if err != nil {
		log.WithError(err).Error("failed to load snap config")
		return nil, err
	}

	return mgr, nil
}

func (m *Mgr) Run() {
	go m.WatchInferenceServiceReplicaSetsAndPods()
	go m.WatchInferenceService()
	go m.StartMonitorInferenceServiceExpiredTime(context.TODO())
}

func (m *Mgr) WatchInferenceServiceReplicaSetsAndPods() {
	podsInformer := m.KubeInformerFactory.Core().V1().Pods()
	podsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    m.addKFServicePodsHandler,
		UpdateFunc: m.updateKFServicePodsHandler,
		DeleteFunc: m.deleteKFServicePodsHandler,
	})

	m.KubeInformerFactory.Start(m.config.StopCh)
}

func (m *Mgr) WatchInferenceService() {
	kfInformer := m.DynamicFactory.ForResource(InferenceServiceGVRV1beta1)
	kfInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: m.updateKserveHandler,
		DeleteFunc: m.deleteKserveHandler,
	})
	m.DynamicFactory.Start(m.config.StopCh)
}

func (m *Mgr) StartMonitorInferenceServiceExpiredTime(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			log.Info("stop inference service monitor")
		case <-ticker.C:
			m.checkKeepAliveDuration()
		}
	}
}

func (m *Mgr) checkKeepAliveDuration() {
	// 获取所有租户
	tenants, err := GetAllTenantName(m.app.MgoClient)
	if err != nil {
		log.WithError(err).Error("failed to GetAllTenantName")
		return
	}
	// 查看所有inferecen
	for i := range tenants {
		scanOneTenantInferenceService(m.app.MgoClient, m.DynamicClient, tenants[i])
	}
}

func scanOneTenantInferenceService(mgoCli *mgolib.MgoClient, dynamicCli dynamic.Interface, tenant string) error {
	logger := log.WithField("tenant", tenant)
	col := getInferenceCollectionByTenant(mgoCli, tenant)
	isvcs, err := FindInferenceServiceKANotZero(col)
	if err != nil {
		logger.Error("failed to FindInferenceServiceKANotZero")
		return err
	}

	if len(isvcs) == 0 {
		logger.Info("all inference services are not in serving")
		return nil
	}

	for _, isvc := range isvcs {
		logger = log.WithField("id", isvc.ID)
		lastRevision := isvc.Revisions[len(isvc.Revisions)-1]
		if !lastRevision.ServiceInstances[0].Spec.AutoShutdown {
			logger.Infof("infer service is forever")
			continue
		}
		if time.Now().Unix()-lastRevision.CreatedAt < int64(lastRevision.ServiceInstances[0].Spec.KeepAliveDuration) {
			logger.Info("infer service is not expired")
			continue
		}
		logger.Info("infer service is expired, auto shutdown inference now")
		if err := UpdateInferenceRevisionReadyAndStage(col, isvc.ID, isvc.ProjectID, "", false, types.InferenceServiceRevisionStageShutdown); err != nil {
			logger.WithError(err).Error("failed to UpdateInferenceRevisionStage")
			continue
		}
		// remove deployed service
		if err := DeleteKFService(newKserveClient(dynamicCli, isvc.ProjectID), isvc); err != nil {
			log.WithError(err).Error("failed to remove deployed service")
			continue
		}
	}
	return nil
}

func GetAllTenantName(client *mgolib.MgoClient) ([]string, error) {
	colNames, err := client.GetCollectionNames()
	if err != nil {
		return nil, err
	}

	tenantNames := make([]string, 0)
	for i := range colNames {
		if strings.HasSuffix(colNames[i], CollInference) {
			tenantName := strings.TrimSuffix(colNames[i], "-"+CollInference)
			tenantNames = append(tenantNames, tenantName)
		}
	}

	return tenantNames, err
}
