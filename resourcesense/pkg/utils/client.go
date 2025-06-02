package utils

import (
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func NewCoreV1Client() (*corev1.CoreV1Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Error("failed to InClusterConfig")
		return nil, err
	}

	return corev1.NewForConfig(config)
}
