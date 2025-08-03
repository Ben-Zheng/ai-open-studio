package client

import (
	"context"

	"github.com/apex/log"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	workspacev1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/operator/api/v1"
)

type WorkspaceInterface interface {
	List(labelSelector string) (*workspacev1.AWorkspaceList, error)
	Get(name, namespace string) (*workspacev1.AWorkspace, error)
	Create(ws *workspacev1.AWorkspace, namespace string) (*workspacev1.AWorkspace, error)
	Update(ws *workspacev1.AWorkspace, name, namespace string) (*workspacev1.AWorkspace, error)
	UpdateStatus(ws *workspacev1.AWorkspace, name, namespace string) (*workspacev1.AWorkspace, error)
	Startup(ws *workspacev1.AWorkspace, name, namespace string) error
	Stop(name, namespace string) error
	Delete(name, namespace string) error
	Watch(namespace string) (watch.Interface, error)
}

type WorkspaceClient struct {
	restClient rest.Interface
}

func (c *WorkspaceClient) List(labelSelector string) (*workspacev1.AWorkspaceList, error) {
	result := workspacev1.AWorkspaceList{}
	err := c.restClient.
		Get().
		Resource(workspacev1.AWorkspaceResource).
		VersionedParams(&metav1.ListOptions{LabelSelector: labelSelector}, scheme.ParameterCodec).
		Do(context.Background()).
		Into(&result)

	return &result, err
}

func (c *WorkspaceClient) Get(name, namespace string) (*workspacev1.AWorkspace, error) {
	result := workspacev1.AWorkspace{}
	err := c.restClient.
		Get().
		Namespace(namespace).
		Resource(workspacev1.AWorkspaceResource).
		Name(name).
		Do(context.Background()).
		Into(&result)

	return &result, err
}

func (c *WorkspaceClient) Stop(name, namespace string) error {
	aw, err := c.Get(name, namespace)
	if err != nil {
		return err
	}

	aw.Status.Status = workspacev1.StateCompleted
	_, err = c.UpdateStatus(aw, name, namespace)

	return err
}

func (c *WorkspaceClient) Delete(name, namespace string) error {
	result := workspacev1.AWorkspace{}
	err := c.restClient.
		Delete().
		Namespace(namespace).
		Resource(workspacev1.AWorkspaceResource).
		Name(name).
		Do(context.Background()).Into(&result)

	return err
}

func (c *WorkspaceClient) Create(ic *workspacev1.AWorkspace, namespace string) (*workspacev1.AWorkspace, error) {
	result := workspacev1.AWorkspace{}
	err := c.restClient.
		Post().
		Namespace(namespace).
		Resource(workspacev1.AWorkspaceResource).
		Body(ic).
		Do(context.Background()).
		Into(&result)

	return &result, err
}

func (c *WorkspaceClient) Update(ic *workspacev1.AWorkspace, name, namespace string) (*workspacev1.AWorkspace, error) {
	result := workspacev1.AWorkspace{}
	err := c.restClient.
		Put().
		Namespace(namespace).
		Resource(workspacev1.AWorkspaceResource).
		Name(name).
		VersionedParams(&metav1.UpdateOptions{}, scheme.ParameterCodec).
		Body(ic).
		Do(context.Background()).
		Into(&result)

	return &result, err
}

func (c *WorkspaceClient) UpdateStatus(ic *workspacev1.AWorkspace, name, namespace string) (*workspacev1.AWorkspace, error) {
	result := workspacev1.AWorkspace{}
	err := c.restClient.
		Put().
		Namespace(namespace).
		Resource(workspacev1.AWorkspaceResource).
		Name(name).
		SubResource("status").
		VersionedParams(&metav1.UpdateOptions{}, scheme.ParameterCodec).
		Body(ic).
		Do(context.Background()).
		Into(&result)

	return &result, err
}

func (c *WorkspaceClient) Startup(ic *workspacev1.AWorkspace, name, namespace string) error {
	ic.Status.Status = workspacev1.StatePending

	ows, err := c.Get(name, namespace)
	if k8sErrors.IsNotFound(err) {
		if _, err := c.Create(ic, namespace); err != nil {
			log.WithError(err).Error("failed to Create Workspaces")
			return err
		}
		return nil
	} else if err != nil {
		log.WithError(err).Error("failed to Get Workspaces")
		return err
	}

	ows.Spec = ic.Spec
	ows.Labels = ic.Labels

	if _, err := c.Update(ows, name, namespace); err != nil {
		log.WithError(err).Error("failed to Update Workspaces")
		return err
	}

	ows, err = c.Get(name, namespace)
	if err != nil {
		log.WithError(err).Error("failed to Get Workspaces")
		return err
	}
	ows.Status = ic.Status

	if _, err = c.UpdateStatus(ows, name, namespace); err != nil {
		log.WithError(err).Error("failed to UpdateStatus")
		return err
	}

	return nil
}

func (c *WorkspaceClient) Watch(namespace string) (watch.Interface, error) {
	return c.restClient.
		Get().
		Namespace(namespace).
		Resource(workspacev1.AWorkspaceResource).
		Watch(context.Background())
}

type WorkspaceBetaInterface interface {
	Workspaces() WorkspaceInterface
}

type WorkspaceBetaClient struct {
	restClient rest.Interface
}

func NewForConfig(c *rest.Config) (*WorkspaceBetaClient, error) {
	if err := workspacev1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: workspacev1.GroupVersion.Group, Version: workspacev1.GroupVersion.Version}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs}
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.UnversionedRESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &WorkspaceBetaClient{restClient: client}, nil
}

func (c *WorkspaceBetaClient) Workspaces() WorkspaceInterface {
	return &WorkspaceClient{
		restClient: c.restClient,
	}
}
