package initial

import (
	"errors"

	log "github.com/sirupsen/logrus"
	coreInformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func InitWorkloadsLister(podInformer coreInformers.PodInformer, pvcInformer coreInformers.PersistentVolumeClaimInformer, nodeInformer coreInformers.NodeInformer, stopCh <-chan struct{}) {
	log.Info("[Quota] Waiting for workload caches to be synced")
	if !cache.WaitForCacheSync(stopCh, podInformer.Informer().HasSynced, pvcInformer.Informer().HasSynced, nodeInformer.Informer().HasSynced) {
		panic(errors.New("timed out waiting for caches to sync"))
	}
	log.Info("[Quota] Workload caches init successfully")
}
