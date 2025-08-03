/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	v1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	workspacev1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/operator/api/v1"
)

// AWorkspaceReconciler reconciles a AWorkspace object
type AWorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AWorkspaceReconciler) deleteExternalResources() error {
	// delete any external resources associated with the cronJob
	//
	// Ensure that delete implementation is idempotent and safe to invoke
	// multiple times for same object.
	fmt.Println("delete............................")
	return nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

//+kubebuilder:rbac:groups=workspace.aiservice.brainpp.cn,resources=aworkspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workspace.aiservice.brainpp.cn,resources=aworkspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workspace.aiservice.brainpp.cn,resources=aworkspaces/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=extensions,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AWorkspace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *AWorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx).WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling AWorkspace")

	instance := &workspacev1.AWorkspace{}
	err := r.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}
	fmt.Println("status", instance.Status.Status)

	myFinalizerName := "batch.tutorial.kubebuilder.io/finalizer"

	// examine DeletionTimestamp to determine if object is under deletion
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !containsString(instance.GetFinalizers(), myFinalizerName) {
			controllerutil.AddFinalizer(instance, myFinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(instance.GetFinalizers(), myFinalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			instance.Status.Status = workspacev1.StateCompleted
			if err := r.Status().Update(ctx, instance); err != nil {
				fmt.Println(err, "unable to update status")
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(instance, myFinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	if _, ok := instance.Annotations["spec"]; !ok {
		return createForWorkspace(ctx, r, instance)
	}

	oldspec := &workspacev1.AWorkspaceSpec{}
	if err := json.Unmarshal([]byte(instance.Annotations["spec"]), &oldspec); err != nil {
		return reconcile.Result{}, err
	}

	fmt.Println("==============================newSpec==================================")
	fmt.Println(instance.Spec)
	fmt.Println("==============================oldSpec==================================")
	fmt.Println(oldspec)
	fmt.Println("=======================================================================")

	if err := updateForWorkspace(ctx, r, instance, req, !reflect.DeepEqual(instance.Spec, *oldspec)); err != nil {
		return reconcile.Result{}, err
	}

	if instance.Status.Status == workspacev1.StatePending {
		return waitForRunning(ctx, r, instance, req)
	}

	return ctrl.Result{}, nil
}

func waitForRunning(ctx context.Context, r *AWorkspaceReconciler, instance *workspacev1.AWorkspace, req ctrl.Request) (ctrl.Result, error) {
	var isReady bool
	if instance.Spec.Annotations[aisConst.AISWSRuntime] == aisConst.WSRuntimeKubevirt {
		dv := &v1.VirtualMachine{}
		if err := r.Get(context.TODO(), req.NamespacedName, dv); err != nil && !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		if dv.Status.Ready {
			isReady = true
		}
	} else {
		deploy := &appsv1.Deployment{}
		if err := r.Get(context.TODO(), req.NamespacedName, deploy); err != nil && !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		if deploy.Status.ReadyReplicas > 0 {
			isReady = true
		}
	}

	if !isReady {
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	instance.Status.Status = workspacev1.StateRunning
	if err := r.Status().Update(ctx, instance); err != nil {
		fmt.Println(err, "unable to update status")
	}

	return ctrl.Result{}, nil
}

func updateDeployment(ctx context.Context, r *AWorkspaceReconciler, instance *workspacev1.AWorkspace, req ctrl.Request, isSpecChanged bool) (bool, error) {
	isExist := true
	oldDeploy := &appsv1.Deployment{}
	if err := r.Get(context.TODO(), req.NamespacedName, oldDeploy); err != nil && errors.IsNotFound(err) {
		isExist = false
	} else if err != nil {
		return false, err
	}

	var isStateChanged bool
	if instance.Status.Status == workspacev1.StateRunning && !isExist {
		isStateChanged = true
	}

	if instance.Status.Status == workspacev1.StatePending || instance.Status.Status == workspacev1.StateRunning {
		if !isExist {
			newDeploy := NewDeploy(instance)
			if err := r.Create(context.TODO(), newDeploy); err != nil {
				return false, err
			}
		} else if isSpecChanged {
			newDeploy := UpdateDeploy(oldDeploy, instance)
			if err := r.Update(context.TODO(), newDeploy); err != nil {
				return false, err
			}
		}
	} else if isExist {
		if err := r.Delete(context.TODO(), oldDeploy); err != nil {
			return false, err
		}
	}

	return isStateChanged, nil
}

func updateVM(ctx context.Context, r *AWorkspaceReconciler, instance *workspacev1.AWorkspace, req ctrl.Request, isSpecChanged bool) (bool, error) {
	osct := &corev1.Secret{}
	if err := r.Get(context.TODO(), req.NamespacedName, osct); errors.IsNotFound(err) {
		sct := NewSecret(instance)
		if err := r.Create(context.TODO(), sct); err != nil {
			return false, err
		}
	} else if err != nil {
		return false, err
	} else if isSpecChanged {
		newSct := UpdateSecret(osct, instance)
		if err := r.Update(context.TODO(), newSct); err != nil {
			return false, err
		}
	}

	oldVM := &v1.VirtualMachine{}
	if err := r.Get(context.TODO(), req.NamespacedName, oldVM); err != nil {
		return false, err
	}

	var isStateChanged bool
	runnging := instance.Status.Status == workspacev1.StatePending || instance.Status.Status == workspacev1.StateRunning
	if oldVM.Spec.Running != &runnging && instance.Status.Status == workspacev1.StateRunning {
		isSpecChanged = true
	}

	if oldVM.Spec.Running != &runnging || isSpecChanged {
		newVM := UpdateVM(oldVM, instance)
		newVM.Spec.Running = &runnging
		if err := r.Update(context.TODO(), newVM); err != nil {
			return false, err
		}
	}

	return isStateChanged, nil
}

func updateForWorkspace(ctx context.Context, r *AWorkspaceReconciler, instance *workspacev1.AWorkspace, req ctrl.Request, isSpecChanged bool) error {
	var err error
	var isStateChanged bool

	if instance.Spec.Annotations[aisConst.AISWSRuntime] == aisConst.WSRuntimeKubevirt {
		isStateChanged, err = updateVM(ctx, r, instance, req, isSpecChanged)
	} else {
		isStateChanged, err = updateDeployment(ctx, r, instance, req, isSpecChanged)
	}
	if err != nil {
		return err
	}

	fmt.Println("isSpecChanged, isStateChanged: ", isSpecChanged, isStateChanged)
	if isSpecChanged {
		oldService := &corev1.Service{}
		if err := r.Get(context.TODO(), req.NamespacedName, oldService); err != nil {
			return err
		}

		newService := UpdateService(oldService, instance)
		if err := r.Update(context.TODO(), newService); err != nil {
			return err
		}

		data, _ := json.Marshal(instance.Spec)
		instance.Annotations["spec"] = string(data)
		if err := r.Update(context.TODO(), instance); err != nil {
			return err
		}
	}

	if isSpecChanged || isStateChanged {
		if instance.Status.Status != workspacev1.StateCompleted {
			instance.Status.Status = workspacev1.StatePending
		}

		if err := r.Status().Update(ctx, instance); err != nil {
			fmt.Println(err, "unable to update status")
		}
	}

	return nil
}

func createForWorkspace(ctx context.Context, r *AWorkspaceReconciler, instance *workspacev1.AWorkspace) (ctrl.Result, error) {
	// 创建关联资源
	// 1. 创建 runtime
	if instance.Spec.Annotations[aisConst.AISWSRuntime] == aisConst.WSRuntimeKubevirt {
		sct := NewSecret(instance)
		if err := r.Create(context.TODO(), sct); err != nil {
			return reconcile.Result{}, err
		}

		dv := NewVM(instance)
		if err := r.Create(context.TODO(), dv); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		deploy := NewDeploy(instance)
		if err := r.Create(context.TODO(), deploy); err != nil {
			return reconcile.Result{}, err
		}
	}

	// 2. 创建 Service
	service := NewService(instance)
	if err := r.Create(context.TODO(), service); err != nil {
		return reconcile.Result{}, err
	}

	// 3. 关联 Annotations
	data, _ := json.Marshal(instance.Spec)
	if instance.Annotations != nil {
		instance.Annotations["spec"] = string(data)
	} else {
		instance.Annotations = map[string]string{"spec": string(data)}
	}

	if err := r.Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, nil
	}

	instance.Status.Status = workspacev1.StatePending
	if err := r.Status().Update(ctx, instance); err != nil {
		fmt.Println(err, "unable to update status")
	}

	return reconcile.Result{
		Requeue:      true,
		RequeueAfter: 5 * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AWorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workspacev1.AWorkspace{}).
		Complete(r)
}
