/*
Copyright 2023.

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
	v1 "k8s.io/api/batch/v1"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Backup struct {
	Schedule string              `json:"schedule,omitempty"`
	NFS      v12.NFSVolumeSource `json:"nfs"`
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FrontendSpec defines the desired state of Frontend

type FrontendSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Config string `json:"config,omitempty"`
	// +kubebuilder:validation:Required
	Image        string            `json:"image"`
	Backup       Backup            `json:"backup,omitempty"`
	NodeSelector map[string]string `json:"nodeSelector"`
}

// FrontendStatus defines the observed state of Frontend

type FrontendStatus struct {
	LastBackupTime   metav1.Time         `json:"lastScheduleTime,omitempty"`
	LastBackupStatus v1.JobConditionType `json:"lastBackupStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Frontend is the Schema for the frontends API
type Frontend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FrontendSpec   `json:"spec,omitempty"`
	Status FrontendStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FrontendList contains a list of Frontend
type FrontendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Frontend `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Frontend{}, &FrontendList{})
}
