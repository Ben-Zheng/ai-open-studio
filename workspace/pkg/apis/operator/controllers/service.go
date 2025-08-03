package controllers

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	workspacev1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/operator/api/v1"
)

func NewService(app *workspacev1.AWorkspace) *corev1.Service {
	labels := app.Spec.Labels
	labels[aisConst.AISWSInstanceID] = app.Spec.Annotations[aisConst.AISWSInstanceID]
	labels[aisConst.AISWSInstanceToken] = app.Spec.Annotations[aisConst.AISWSInstanceToken]

	if app.Spec.Annotations[aisConst.AISWSRuntime] == aisConst.WSRuntimeKubevirt {
		// jupyter lab 由 80 端口改为 8888
		for i := range app.Spec.Ports {
			if app.Spec.Ports[i].Port == 80 {
				app.Spec.Ports[i].TargetPort.IntVal = 8888
			}
		}
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   app.GroupVersionKind().Group,
					Version: app.GroupVersionKind().Version,
					Kind:    "AWorkspace",
				}),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeClusterIP,
			Ports: app.Spec.Ports,
			Selector: map[string]string{
				aisConst.AISWSApp: app.Name,
			},
		},
	}
}

func UpdateService(oldService *corev1.Service, app *workspacev1.AWorkspace) *corev1.Service {
	newService := NewService(app)

	oldService.Labels = newService.Labels
	oldService.Spec.Ports = newService.Spec.Ports
	oldService.Spec.Type = newService.Spec.Type
	oldService.Spec.Selector = newService.Spec.Selector
	return oldService
}
