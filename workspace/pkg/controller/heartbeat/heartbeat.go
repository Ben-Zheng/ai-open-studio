package heartbeat

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	coreV1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/workspace"
)

func Detect() {
	interval := 20 * time.Second

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		<-t.C
		log.Info("======================================================")
		log.Info("start Detect")

		if err := doDetect(); err != nil {
			log.WithError(err).Error("failed to DetectByRecentID")
		}
	}
}

func doDetect() error {
	gc := ginlib.NewMockGinContext()
	page := 1
	pageSize := 100

	for {
		_, inss, err := workspace.ListInstance(gc, true, primitive.ObjectID{}, page, pageSize)
		if err != nil {
			log.WithError(err).Error("failed to list workspace")
			return err
		}

		var wg sync.WaitGroup
		for i := range inss {
			wg.Add(1)
			go watchWorkspaceHeartbeat(gc, &wg, &inss[i])
		}

		wg.Wait()

		if len(inss) < pageSize {
			break
		}
		page++
	}

	return nil
}

func updateInstance(gc *ginlib.GinContext, ins *workspace.Instance) error {
	pods, err := consts.Clientsets.K8sClient.CoreV1().Pods(ins.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", aisConst.AISWSInstanceID, ins.ID.Hex()),
	})
	if err != nil || pods == nil || len(pods.Items) == 0 {
		log.WithError(err).WithField(aisConst.AISWSInstanceID, ins.ID.Hex()).Error("list all pods failed")

		if pods, err = consts.Clientsets.K8sClient.CoreV1().Pods(ins.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("instance=%s", ins.ID.Hex()),
		}); err != nil {
			log.WithError(err).WithField("instance", ins.ID.Hex()).Error("list all pods failed")
			return err
		}
	}

	for i := range pods.Items {
		if pods.Items[i].Status.Phase == coreV1.PodRunning && (ins.PodName != pods.Items[i].Name ||
			ins.PodIP != pods.Items[i].Status.PodIP || ins.NodeName != pods.Items[i].Spec.NodeName) {
			ins.PodName = pods.Items[i].Name
			ins.PodIP = pods.Items[i].Status.PodIP
			ins.NodeName = pods.Items[i].Spec.NodeName
			if err := workspace.UpdateInstance(gc, ins.ID, ins); err != nil {
				log.WithError(err).Error("faield to UpdateInstance")
				return err
			}
		}
	}

	return nil
}

func watchWorkspaceHeartbeat(gc *ginlib.GinContext, wg *sync.WaitGroup, ins *workspace.Instance) {
	defer wg.Done()

	ws, err := workspace.GetWorkspaceByID(gc, ins.WorkspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetWorkspaceByID")
		return
	}

	aws, err := consts.Clientsets.WSClient.Workspaces().Get(ins.Name, ins.Namespace)
	if !k8sErrors.IsNotFound(err) && err != nil {
		log.WithError(err).Error("failed to Get Workspaces")
		return
	}

	var reason string
	var state consts.InstanceState
	if aws.Spec.WorkspaceID == "" {
		state, reason = consts.InstanceStatePending, ""
		if ins.CreatedAt+consts.InstancePendingDuration < time.Now().Unix() {
			state, reason = consts.InstanceStateFailed, consts.ReasonNotFound
		}
	} else {
		if err := updateInstance(gc, ins); err != nil {
			log.WithField("ins", ins).WithError(err).Error("failed to updateInstance")
		}

		if consts.ConfigMap.TimeoutExitEnable && ins.LastPingAt+ws.ExitTimeout < time.Now().Unix() ||
			aws.Status.Status != string(consts.InstanceStateRunning) && ins.CreatedAt+consts.InstancePendingDuration < time.Now().Unix() {
			if err := consts.Clientsets.WSClient.Workspaces().Stop(ins.Name, ins.Namespace); err != nil {
				log.WithError(err).Error("failed to Stop Workspaces")
				return
			}

			state = consts.InstanceStateFailed
			if consts.ConfigMap.TimeoutExitEnable && ins.LastPingAt+ws.ExitTimeout < time.Now().Unix() {
				reason = consts.ReasonRunningTimeout
			} else if aws.Status.Status == string(consts.InstanceStatePending) {
				reason = consts.ReasonScheduleFailed
			} else {
				state = consts.InstanceStateCompleted
			}
		} else if aws.Status.Status != string(consts.InstanceStateRunning) {
			state = consts.InstanceStatePending
		} else {
			state = consts.InstanceStateRunning
		}
	}

	if state != ins.State || reason != ins.Reason {
		if err := workspace.SetInstanceState(gc, ins.ID, state, reason); err != nil {
			log.WithError(err).Error("faield to SetInstanceState")
			return
		}
	}
}
