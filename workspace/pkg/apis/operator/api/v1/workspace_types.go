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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AWorkspaceSpec defines the desired state of AWorkspace
type AWorkspaceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Size        int32                       `json:"size"`
	InstanceID  string                      `json:"instanceID"`
	WorkspaceID string                      `json:"workspaceID"`
	Image       string                      `json:"image"`
	Annotations map[string]string           `json:"annotations,omitempty"`
	Labels      map[string]string           `json:"labels"`
	Resources   corev1.ResourceRequirements `json:"resources,omitempty"`
	Affinity    *corev1.Affinity            `json:"affinity,omitempty"`
	Envs        []corev1.EnvVar             `json:"envs,omitempty"`
	Ports       []corev1.ServicePort        `json:"ports,omitempty"`
}

// AWorkspaceStatus defines the observed state of AWorkspace
type AWorkspaceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Status string `json:"status"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AWorkspace is the Schema for the aworkspaces API
type AWorkspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWorkspaceSpec   `json:"spec,omitempty"`
	Status AWorkspaceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AWorkspaceList contains a list of AWorkspace
type AWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWorkspace `json:"items"`
}

const (
	StatePending   = "Pending"
	StateRunning   = "Running"
	StateCompleted = "Completed"
)

func init() {
	SchemeBuilder.Register(&AWorkspace{}, &AWorkspaceList{})
}
