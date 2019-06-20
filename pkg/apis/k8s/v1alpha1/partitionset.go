/*
 * Copyright 2019 Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PartitionSetSpec struct {
	Partitions int                   `json:"partitions,omitempty"`
	Template   PartitionTemplateSpec `json:"template,omitempty"`
}

type PartitionTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PartitionSpec `json:"spec,omitempty"`
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
	Spec              PartitionSetSpec   `json:"spec,omitempty"`
	Status            PartitionSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PartitionSetList contains a list of PartitionSet
type PartitionSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PartitionSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PartitionSet{}, &PartitionSetList{})
}
