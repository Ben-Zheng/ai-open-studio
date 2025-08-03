package client

import (
	"encoding/json"

	workspacev1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/operator/api/v1"

	"k8s.io/client-go/tools/clientcmd"
)

type AISWorkspace struct {
	Workspace workspacev1.AWorkspace
}

func NewAISWorkspace() (*AISWorkspace, error) {
	var kp = AISWorkspace{
		Workspace: workspacev1.AWorkspace{},
	}

	err := json.Unmarshal([]byte(template), &kp.Workspace)
	if err != nil {
		return nil, err
	}

	return &kp, nil
}

func NewClientSet(kubeconfigPath string) (*WorkspaceBetaClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	clientSet, err := NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientSet, nil
}

func (kp *AISWorkspace) Deploy(kubeconfigPath string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		panic(err)
	}

	clientSet, err := NewForConfig(config)
	if err != nil {
		panic(err)
	}

	_, err = clientSet.Workspaces().Create(&kp.Workspace, "default")
	if err != nil {
		panic(err)
	}

	return err
}
