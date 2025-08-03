package controllers

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	workspacev1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/operator/api/v1"
)

func NewDeploy(app *workspacev1.AWorkspace) *appsv1.Deployment {
	labels := map[string]string{}
	for key, value := range app.Spec.Labels {
		labels[key] = value
	}

	selector := &metav1.LabelSelector{MatchLabels: app.Spec.Labels}
	labels[aisConst.AISWSInstanceID] = app.Spec.Annotations[aisConst.AISWSInstanceID]
	labels[aisConst.AISWSInstanceToken] = app.Spec.Annotations[aisConst.AISWSInstanceToken]
	sl := app.Spec.Resources.Limits.Memory()
	sl.Set(sl.Value() / 2)

	volumes := []corev1.Volume{
		{
			Name: "shm", VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium:    corev1.StorageMediumMemory,
					SizeLimit: sl,
				},
			},
		},
		{
			Name: "pv-storage", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: app.Name,
				},
			},
		},
		{
			Name: "docker-daemon-storage", VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
			Namespace:   app.Namespace,
			Labels:      labels,
			Annotations: app.Spec.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   app.GroupVersionKind().Group,
					Version: app.GroupVersionKind().Version,
					Kind:    "AWorkspace",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &app.Spec.Size,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: app.Spec.Annotations,
				},
				Spec: corev1.PodSpec{
					Volumes:          volumes,
					Containers:       newContainers(app),
					Affinity:         app.Spec.Affinity,
					ImagePullSecrets: buildImagePullSecrets(app),
				},
			},
			Selector: selector,
		},
	}
}

func buildImagePullSecrets(app *workspacev1.AWorkspace) []corev1.LocalObjectReference {
	var re []corev1.LocalObjectReference
	if pullSecretName, ok := app.Spec.Annotations[aisConst.AISWSImagePullSecretName]; ok {
		re = append(re, corev1.LocalObjectReference{Name: pullSecretName})
	}

	return re
}

func newContainers(app *workspacev1.AWorkspace) []corev1.Container {
	containerPorts := []corev1.ContainerPort{}
	for _, svcPort := range app.Spec.Ports {
		cport := corev1.ContainerPort{}
		cport.ContainerPort = svcPort.TargetPort.IntVal
		containerPorts = append(containerPorts, cport)
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "shm",
			MountPath: "/dev/shm",
		},
		{
			Name:      "pv-storage",
			MountPath: "/home/aiservice/workspace",
		},
	}

	cts := []corev1.Container{
		{
			Name:            app.Name,
			Image:           app.Spec.Image,
			Resources:       app.Spec.Resources,
			Ports:           containerPorts,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env: append(app.Spec.Envs, corev1.EnvVar{
				Name: "DOCKER_HOST", Value: "tcp://localhost:2375",
			}),
			VolumeMounts: volumeMounts,
		},
	}

	if _, ok := app.Spec.Annotations[aisConst.AISWSDockerImage]; ok {
		privileged := true

		dockerCT := corev1.Container{
			Name:            "docker-daemon",
			Image:           app.Spec.Annotations[aisConst.AISWSDockerImage],
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env: []corev1.EnvVar{
				{Name: "DOCKER_TLS_CERTDIR", Value: ""},
			},
			SecurityContext: &corev1.SecurityContext{
				Privileged: &privileged,
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "docker-daemon-storage", MountPath: "/var/lib/docker"},
			},
		}

		if bip := app.Spec.Annotations[aisConst.AISWSDockerBIP]; bip != "" {
			dockerCT.Args = []string{fmt.Sprintf("--bip=%s", app.Spec.Annotations[aisConst.AISWSDockerBIP])}
		}

		cts = append(cts, dockerCT)
	}

	return cts
}

func UpdateDeploy(oldDeploy *appsv1.Deployment, app *workspacev1.AWorkspace) *appsv1.Deployment {
	newDeploy := NewDeploy(app)

	oldDeploy.Labels = newDeploy.Labels
	oldDeploy.Annotations = newDeploy.Annotations
	oldDeploy.Spec.Replicas = newDeploy.Spec.Replicas
	oldDeploy.Spec.Template.Labels = newDeploy.Spec.Template.Labels
	oldDeploy.Spec.Template.Annotations = newDeploy.Spec.Template.Annotations
	oldDeploy.Spec.Template.Spec.Volumes = newDeploy.Spec.Template.Spec.Volumes
	oldDeploy.Spec.Template.Spec.Containers = newDeploy.Spec.Template.Spec.Containers
	oldDeploy.Spec.Template.Spec.ImagePullSecrets = newDeploy.Spec.Template.Spec.ImagePullSecrets

	return oldDeploy
}
