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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PartitionSpec is the k8s configuration for a single partition
type PartitionSpec struct {
	// Size is the number of nodes in the partition
	Size int32 `json:"size,omitempty"`

	// Env is a set of environment variables to pass to partition nodes
	Env []v1.EnvVar `json:"env,omitempty"`

	// Resources is the resources to allocate for the partition nodes
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// StorageClass is an optional storage class to configure storage for the nodes. If the StorageClass is
	// nil then EmptyDir will be used for persistent protocols.
	StorageClass *string `json:"storageClass,omitempty"`

	// StorageSize is the size of the storage to allocate for the each node in the partition
	StorageSize string `json:"storageSize,omitempty"`

	// Protocol is the protocol to run in the partition
	Protocol string `json:"protocol,omitempty"`

	// Image is the image to run in the partition. If the image is unspecified, the default image for the protocol
	// will be used.
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the pull policy to apply to partition images
	ImagePullPolicy v1.PullPolicy `json:"pullPolicy:omitempty"`

	// Config is the partition configuration to pass to its nodes
	Config string `json:"config,omitempty"`
}

// PartitionStatus defines the observed state of Partition
type PartitionStatus struct {
	// ReadyReplicas is the number of replicas in the partition that have been marked ready
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Partition is the Schema for the atomixclusters API
// +k8s:openapi-gen=true
type Partition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the spec for the partition
	Spec PartitionSpec `json:"spec,omitempty"`

	// Status is the status of the partition
	Status PartitionStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PartitionList contains a list of Partition
type PartitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []Partition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Partition{}, &PartitionList{})
}
