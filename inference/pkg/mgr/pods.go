package mgr

// Client Wrapper for kubernetes pods object
import (
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

func (m *Mgr) getKFServicePods(serviceName, projectID string) []string {
	servicePods, ok := m.podsMap.Load(genServiceName(GetNamespace(projectID), serviceName, KserveWebhookLabelValuePredictor))
	if !ok {
		return []string{}
	}

	var pods []string
	for podName, alive := range servicePods.(map[string]bool) {
		if alive {
			pods = append(pods, podName)
		}
	}

	if pods == nil {
		return []string{}
	}
	return pods
}

func (m *Mgr) addOrUpdateKFServicePods(pod *corev1.Pod) {
	serviceName, ok := pod.ObjectMeta.Labels[KserveWebhookLabelKeyIsvc] //"serving.kserve.io/inferenceservice"
	if !ok {
		log.Infof("no service name")
		return
	}
	instanceType, ok := pod.ObjectMeta.Labels[KserveWebhookLabelKeyComponent] //"component"
	if !ok {
		log.Infof("no instance type")
		return
	}
	if instanceType != KserveWebhookLabelValuePredictor { //predictor"
		log.Infof("no predictor type")
		return
	}

	serviceName = genServiceName(pod.ObjectMeta.Namespace, serviceName, instanceType)
	log.Debugf("add or update service pod for service %s", serviceName)

	curPods, ok := m.podsMap.Load(serviceName)
	var curPodsMap map[string]bool
	if !ok {
		curPodsMap = make(map[string]bool)
	} else {
		curPodsMap = curPods.(map[string]bool)
	}

	podName := pod.ObjectMeta.Name
	if pod.Status.Phase == corev1.PodRunning {
		curPodsMap[podName] = true
		col := getInferenceCollectionByTenant(m.app.MgoClient, pod.ObjectMeta.Labels[aisTypes.AISTenantID])
		//col *mongo.Collection, inferServiceID, projectID, podName, instanceType string, expire time.Duration
		if err := AddInstancePod(col, pod.ObjectMeta.Annotations[aisTypes.AISResourceID], pod.ObjectMeta.Labels[aisTypes.AISProject], podName, instanceType,
			time.Duration(m.config.PodNamesRemainSeconds)); err != nil {
			log.WithError(err).Error("failed to addInstancePod")
		}
	} else {
		delete(curPodsMap, podName)
	}
	m.podsMap.Store(serviceName, curPodsMap)
}

func (m *Mgr) addKFServicePodsHandler(obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	log.Infof("Receive Add Pods event, pod name: %s", pod.ObjectMeta.Name)

	m.addOrUpdateKFServicePods(pod)
}

func (m *Mgr) updateKFServicePodsHandler(old, cur any) {
	oldPod := old.(*corev1.Pod)
	newPod := cur.(*corev1.Pod)
	if oldPod.ResourceVersion == newPod.ResourceVersion {
		return
	}
	log.Infof("Receive Update Pods event, pod name: %s", newPod.ObjectMeta.Name)

	m.addOrUpdateKFServicePods(newPod)
}

func (m *Mgr) deleteKFServicePodsHandler(obj any) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	log.Infof("Receive Delete Pods event, pod name: %s", pod.ObjectMeta.Name)

	serviceName, ok := pod.ObjectMeta.Labels[KserveWebhookLabelKeyIsvc]
	if !ok {
		log.Infof("service name is not found")
		return
	}
	instanceType, ok := pod.ObjectMeta.Labels[KserveWebhookLabelKeyComponent]
	if !ok {
		log.Infof("instance type is not found")
		return
	}
	serviceName = genServiceName(pod.Namespace, serviceName, instanceType)
	log.Debugf("delete service pod: %s", serviceName)

	curPods, ok := m.podsMap.Load(serviceName)
	var curPodsMap map[string]bool
	if !ok {
		curPodsMap = make(map[string]bool)
	} else {
		curPodsMap = curPods.(map[string]bool)
	}

	podName := pod.ObjectMeta.Name
	if _, ok := curPodsMap[podName]; ok {
		delete(curPodsMap, podName)
		m.podsMap.Store(serviceName, curPodsMap)
		log.Infof("curPodsMap: %+v", curPodsMap)
	}
}
