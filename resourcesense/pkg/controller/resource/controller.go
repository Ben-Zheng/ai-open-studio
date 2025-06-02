package resource

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/informers"
	corev1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

const (
	AISResourceControllerName = "ais-resource-controller"
)

func NewAndRunResourceController(ctx context.Context, client *kubernetes.Clientset, sharedInformer informers.SharedInformerFactory, nodeInformer corev1.NodeInformer, podInformer corev1.PodInformer) error {
	run := func(ctx context.Context) {
		var err error
		consts.ResourceClient, err = NewResourceClient(client, nodeInformer, podInformer)
		if err != nil {
			log.WithError(err).Error("failed to NewResourceClient")
			return
		}

		sharedInformer.Start(ctx.Done())

		log.Info("[Resource] Waiting for resource caches to be synced")
		if !cache.WaitForCacheSync(ctx.Done(), nodeInformer.Informer().HasSynced, podInformer.Informer().HasSynced) {
			log.Error("timed out waiting for caches to sync")
			return
		}
		log.Info("[Resource] resource caches init successfully")

		log.Warn("resource.NodeSync start")
		go NodeSync(ctx)
		log.Warn("resource.Sync start")
		go Sync(ctx)
		log.Warn("resource.NodeMonitor start")
		go NodeMonitor(ctx)
	}

	cmLeader, err := aisUtils.GetLeaderElector(client, AISResourceControllerName, "", run)
	if err != nil {
		log.Error("failed to GetLeaderElector")
		return err
	}

	go cmLeader.Run(ctx)

	return nil
}

func NewResourceClient(client *kubernetes.Clientset, nodeInformer corev1.NodeInformer, podInformer corev1.PodInformer) (*resource.Client, error) {
	nodeSelector := labels.NewSelector()
	for _, nodeLabel := range consts.ConfigMap.NodeLabels {
		lab := strings.Split(nodeLabel, ":")
		if len(lab) != 2 {
			log.WithField("node-label", nodeLabel).Warn("params error")
			continue
		}
		req, err := labels.NewRequirement(lab[0], selection.Equals, []string{lab[1]})
		if err != nil {
			log.WithError(err).WithField("node-label", nodeLabel).Warn("failed to NewRequirement")
			continue
		}
		nodeSelector = nodeSelector.Add(*req)
	}

	if len(consts.ConfigMap.NodeLabels) == 0 {
		nodeSelector = labels.Everything()
	}

	c := &resource.Client{
		Clientset:    client,
		NodeSelector: nodeSelector,
		PodLister:    podInformer.Lister(),
		NodeLister:   nodeInformer.Lister(),
	}

	return c, nil
}
