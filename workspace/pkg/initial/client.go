package initial

import (
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

// InitOSS 创建 oss 客户端
func InitClient() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Error("failed to InClusterConfig")
		panic(err)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithError(err).Error("failed to NewForConfig for k8s")
		panic(err)
	}

	wsClient, err := client.NewForConfig(config)
	if err != nil {
		log.WithError(err).Error("failed to NewForConfig for ws")
		panic(err)
	}

	serviceLister, err := BuildCacheController(k8sClient)
	if err != nil {
		log.WithError(err).Error("failed to NewForConfig for ws")
		panic(err)
	}

	consts.Clientsets = &consts.Clientset{
		K8sClient:     k8sClient,
		WSClient:      wsClient,
		ServiceLister: serviceLister,
	}
}

// BuildCacheController 创建 k8s 缓存控制器
func BuildCacheController(clientset *kubernetes.Clientset) (listerscorev1.ServiceLister, error) {
	stop := make(chan struct{})
	sharedInformerFactory := informers.NewSharedInformerFactory(clientset, 30*time.Second)

	genericInformer, err := sharedInformerFactory.ForResource(schema.GroupVersionResource{
		Group:    corev1.GroupName,
		Version:  corev1.SchemeGroupVersion.Version,
		Resource: "services",
	})
	if err != nil {
		return nil, err
	}

	go genericInformer.Informer().Run(stop)

	sharedInformerFactory.Start(stop)

	return sharedInformerFactory.Core().V1().Services().Lister(), nil
}
