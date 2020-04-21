// Copyright 2019-present Open Networking Foundation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1beta3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageControllerSpec is the k8s spec for a StorageController resource
type StorageControllerSpec struct {
	metav1.GroupVersionKind `json:",inline"`
}

// StorageControllerStatus defines the observed state of StorageController
type StorageControllerStatus struct{}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StorageController is the Schema for the StorageController API
// +k8s:openapi-gen=true
type StorageController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Type is the storage controller type
	Type StorageControllerSpec `json:"type,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StorageControllerList contains a list of StorageControllers
type StorageControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []StorageController `json:"items"`
}

// StorageClassReference is a reference to a storage configuration
type StorageClassReference struct {
	metav1.GroupVersionKind `json:",inline"`
	Namespace               string `json:"namespace,omitempty"`
	Name                    string `json:"name,omitempty"`
}

func init() {
	SchemeBuilder.Register(&StorageController{}, &StorageControllerList{})
}
