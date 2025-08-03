package main

import (
	"fmt"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/client"
)

func CreateWorkspace() error {
	mc, err := client.NewAISWorkspace()
	if err != nil {
		return err
	}
	fmt.Println(mc.Workspace.Namespace)

	if err := mc.Deploy(".kube/config"); err != nil {
		return err
	}

	return nil
}

func ListWorkspace() error {
	clientSet, err := client.NewClientSet(".kube/config")
	if err != nil {
		return err
	}
	wl, err := clientSet.Workspaces().List("environment=dev")
	if err != nil {
		return err
	}

	for i := range wl.Items {
		fmt.Println(wl.Items[i].Name, wl.Items[i].Namespace)
		nws, err := clientSet.Workspaces().Get(wl.Items[i].Name, wl.Items[i].Namespace)
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println(nws.Name, nws.Namespace)
		fmt.Println("============================")
	}

	return nil
}

func main() {
	if err := ListWorkspace(); err != nil {
		fmt.Println(err)
	}
}
