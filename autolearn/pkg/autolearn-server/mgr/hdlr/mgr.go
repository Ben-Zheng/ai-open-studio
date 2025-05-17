package hdlr

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/autolearn"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/watcher"
	aisclient "go.megvii-inc.com/brain/brainpp/projects/aiservice/components/pkg/client/clientset/versioned"
	datahubV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginapp"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	noriUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/nori"
	publicService "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg"
	schedulerv1alpha1 "go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/client/clientset/versioned/typed/gang/v1alpha1"
)

type Mgr struct {
	App                  *ginapp.App
	KubeClient           *kubernetes.Clientset
	AisClient            *aisclient.Clientset
	OssClient            *oss.Client
	OssProxyClient       *oss.Client
	SchedulerClient      *schedulerv1alpha1.SchedulerV1alpha1Client
	DataHubClient        datahubV1.Client
	NoriControllerClient noriUtils.Client
	PsClient             publicService.Client
	Controller           *autolearn.Controller
}

func NewMgr(app *ginapp.App, conf *config.Conf) (*Mgr, error) {
	config.Profile = conf
	config.Profile.NoriServer = SetNoriStateQueryTime(conf.NoriServer)
	config.Profile.DetEvalCodebase = SetEvalJobResource(conf.DetEvalCodebase)
	config.Profile.ClfEvalCodebase = SetEvalJobResource(conf.ClfEvalCodebase)

	ossClient, err := oss.NewClient(conf.OssConfig, false)
	if err != nil {
		log.WithError(err).Error("failed to init OSS client")
		return nil, err
	}

	ossProxyClient, err := oss.NewClient(conf.OssConfig, true)
	if err != nil {
		log.WithError(err).Error("failed to init OSS proxy clint")
		return nil, err
	}

	if app.Conf.AuthConf.Debug {
		return &Mgr{
			App:            app,
			OssClient:      ossClient,
			OssProxyClient: ossProxyClient,
			DataHubClient:  datahubV1.NewClient(conf.AisEndPoint.DatahubAPIServer),
			PsClient:       publicService.NewClient(conf.AisEndPoint.PublicServer),
		}, nil
	}

	ctx := context.TODO()
	kubeAndAutoLearnConfig, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.WithError(err).Error("failed to get kubernetes and autoLearnClient Config")
		return nil, err
	}

	kubeClient := kubernetes.NewForConfigOrDie(restclient.AddUserAgent(kubeAndAutoLearnConfig, "kube"))
	schedulerClient := schedulerv1alpha1.NewForConfigOrDie(restclient.AddUserAgent(kubeAndAutoLearnConfig, "kubebrain"))

	aisClient, err := aisclient.NewForConfig(kubeAndAutoLearnConfig)
	if err != nil {
		log.WithError(err).Error("failed to init ais kube client")
		return nil, err
	}

	k8sInformerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)
	go k8sInformerFactory.Start(ctx.Done())

	if err := noriUtils.ReloadConfig(&noriUtils.OSSConfig{
		AccessKey: config.Profile.OssConfig.AccessKey,
		SecretKey: config.Profile.OssConfig.SecretKey,
		Endpoint:  config.Profile.OssConfig.Endpoint,
	}); err != nil {
		log.WithError(err).Error("failed to reloadConfig")
		return nil, err
	}

	noriControllerClient := noriUtils.NewNoriClient(&noriUtils.Server{
		ControllerAddr: conf.NoriServer.ControllerAddr,
		LocateAddr:     conf.NoriServer.LocateAddr,
		OSSProxyAddr:   conf.NoriServer.OSSProxyAddr,
		NoriEnv:        conf.NoriServer.NoriEnv,
	})

	lockMap := watcher.NewLockMap()
	sm := watcher.NewStateMachine(lockMap)
	watcherIns := watcher.NewCheckLoop(lockMap, sm, app.MgoClient, kubeClient, aisClient, schedulerClient)
	watcherIns.Run()

	Controller := autolearn.NewController(
		app.MgoClient,
		ossClient,
		sm,
		schedulerClient,
		kubeClient,
		noriControllerClient,
		datahubV1.NewClient(conf.AisEndPoint.DatahubAPIServer),
		publicService.NewClient(conf.AisEndPoint.PublicServer),
	)

	mgr := &Mgr{
		App:                  app,
		KubeClient:           kubeClient,
		AisClient:            aisClient,
		OssClient:            ossClient,
		OssProxyClient:       ossProxyClient,
		SchedulerClient:      schedulerClient,
		DataHubClient:        datahubV1.NewClient(conf.AisEndPoint.DatahubAPIServer),
		NoriControllerClient: noriControllerClient,
		PsClient:             publicService.NewClient(conf.AisEndPoint.PublicServer),
		Controller:           Controller,
	}

	return mgr, nil
}

// 设置Nori加速状态查询间隔和重试次数
func SetNoriStateQueryTime(noriServer *config.NoriServer) *config.NoriServer {
	if noriServer == nil {
		noriServer = &config.NoriServer{}
	}

	if noriServer.StateQueryInterval < config.MinNoriStateQueryInterval || noriServer.StateQueryInterval > config.MaxNoriStateQueryInterval {
		noriServer.StateQueryInterval = config.MinNoriStateQueryInterval
	}
	if noriServer.StateQueryMaxRetry < config.MinNoriStateQueryRetries || noriServer.StateQueryMaxRetry > config.MaxNoriStateQueryRetries {
		noriServer.StateQueryMaxRetry = config.MinNoriStateQueryRetries
	}

	log.Debugf("server nori config: %+v", noriServer)
	return noriServer
}

// SetEvalJobResource 设置评测任务Job的资源配置
func SetEvalJobResource(eval *config.EvalCodebase) *config.EvalCodebase {
	if eval == nil {
		eval = &config.EvalCodebase{
			CPUPerJob:    config.DefaultCPUPerEvalJob,
			GPUPerJob:    config.DefaultGPUPerEvalJob,
			MemoryPerJob: config.DefaultMemoryEvalJob,
		}

		return eval
	}

	if eval.CPUPerJob <= 0 {
		eval.CPUPerJob = config.DefaultCPUPerEvalJob
	}
	if eval.GPUPerJob < 0 {
		eval.GPUPerJob = config.DefaultGPUPerEvalJob
	}
	if eval.MemoryPerJob <= 0 {
		eval.MemoryPerJob = config.DefaultMemoryEvalJob
	}

	log.Infof("eval codebase config: %+v", eval)
	return eval
}
