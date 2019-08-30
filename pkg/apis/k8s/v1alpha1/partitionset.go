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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PartitionSetSpec is the k8s spec for a PartitionSet resource
type PartitionSetSpec struct {
	// Partitions is the number of partitions in the set
	Partitions int `json:"partitions,omitempty"`

	// Template is the PartitionTemplateSpec to use to configure the partitions
	Template PartitionTemplateSpec `json:"template,omitempty"`
}

// PartitionTemplateSpec is a template for partitions managed by the set
type PartitionTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the PartitionSpec for the partitions in the set
	Spec PartitionSpec `json:"spec,omitempty"`
}

// PartitionSetStatus defines the observed state of Partition
type PartitionSetStatus struct {
	// ReadyPartitions is the number of partitions in the set that have been marked ready
	ReadyPartitions int32 `json:"readyPartitions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PartitionSet is the Schema for the partitionsets API
// +k8s:openapi-gen=true
type PartitionSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the PartitionSet specification
	Spec PartitionSetSpec `json:"spec,omitempty"`

	// Status if the current status of the PartitionSet
	Status PartitionSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PartitionSetList contains a list of PartitionSet
type PartitionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []PartitionSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PartitionSet{}, &PartitionSetList{})
}
