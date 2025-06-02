package main

import (
	"context"
	"flag"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	v1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota/datasetstorage"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/resource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/site"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/initial"
)

// configPath 配置文件路径
var configPath string

// etcdCertPath etct证书路径
var etcdCertPath string

var webhookCert, webhookKey string

func init() {
	klog.InitFlags(nil)
	flag.StringVar(&configPath, "config", "./projects/aiservice/publicservice/hack/publicservice.yaml", "Config file for public service")
	flag.StringVar(&etcdCertPath, "etcd-cert", "./project/aiservice/publicservice/hack/etcd-cert", "certificate path for etcd")

	flag.StringVar(&webhookCert, "tlsCertFile", "/var/lib/publicservice/cert.pem", "File containing the x509 Certificate for HTTPS")
	flag.StringVar(&webhookKey, "tlsKeyFile", "/var/lib/publicservice/key.pem", "File containing the x509 private key to --tlsCertFile")
}

func main() {
	flag.Parse()

	sentry.InitClient()
	initial.InitConfig(configPath)
	initial.InitLog()
	initial.InitDB(consts.ConfigMap)
	initial.InitRouter(consts.ConfigMap.GinConf)
	initial.InitClients(etcdCertPath)
	initial.InitOSS()
	config, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Error("failed to InClusterConfig")
		panic(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithError(err).Error("failed to NewForConfig")
		panic(err)
	}
	sdg := aisUtils.GetShutdownGraceFully()

	// xxxx.Informer() 同一类型只会初始化一个
	sharedInformerFactory := informers.NewSharedInformerFactory(client, 0)
	nodeInformer := sharedInformerFactory.Core().V1().Nodes()
	podInformer := sharedInformerFactory.Core().V1().Pods()
	namespaceInformer := sharedInformerFactory.Core().V1().Namespaces()

	if err := resource.NewAndRunResourceController(sdg.Ctx, client, sharedInformerFactory, nodeInformer, podInformer); err != nil {
		log.WithError(err).Error("failed to NewAndRunResourceController")
		panic(err)
	}

	// 判断是否开启配额
	if features.IsQuotaEnabled() {
		authClient := v1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer, consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
		pvcInformer := sharedInformerFactory.Core().V1().PersistentVolumeClaims()
		namespaceInformer.Lister()
		consts.WorkloadLister = quota.NewWorkloadsLister(podInformer.Lister(), pvcInformer.Lister(), nodeInformer.Lister(), consts.DatahubClient)
		sharedInformerFactory.Start(sdg.Ctx.Done())

		// InitWorkloadsLister存在阻塞，每个节点都需要lister
		go initial.InitWorkloadsLister(podInformer, pvcInformer, nodeInformer, sdg.Ctx.Done())
		quotaServer := quota.NewServer(":6443", ":9200", webhookCert, webhookKey, namespaceInformer, podInformer)
		go quotaServer.Run(sdg.Ctx.Done())

		// 创建quota controller leader节点，只有leader节点才能reconcile
		if err := quota.NewAndRunControllerManager(sdg.Ctx, podInformer, pvcInformer, namespaceInformer, client, authClient); err != nil {
			log.WithError(err).Error("failed to NewAndRunControllerManager")
			panic(err)
		}

		// 是否开启数据集存储配额
		if features.IsQuotaSupportDatasetStorageEnabled() {
			go datasetstorage.GetControllerManager(client).Run(sdg.Ctx)
		}
	}

	// 判断是否开启多集群
	if features.IsMultiSiteEnabled() {
		go site.PeriodReportSiteStatus()
		authClient := v1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
			consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
		syncer := site.NewAuthSideEffectSyncer(namespaceInformer, authClient, client)
		go syncer.Run(sdg.Ctx.Done())
	}

	// 启动路由
	httpServer := &http.Server{
		Addr:    consts.GinEngine.Conf.ServerAddr,
		Handler: consts.GinEngine,
	}
	go httpServer.ListenAndServe()

	aisUtils.WaitUntilShutdownGracefully(10*time.Second,
		func(ctx context.Context) { httpServer.Shutdown(ctx) },
		func(ctx context.Context) { aisUtils.GetShutdownGraceFully().Shutdown(ctx) },
	)
}
