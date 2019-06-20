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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PartitionSpec defines the desired state of Partition
type PartitionSpec struct {
	Size         int32                   `json:"size,omitempty"`
	Env          []v1.EnvVar             `json:"env,omitempty"`
	Resources    v1.ResourceRequirements `json:"resources,omitempty"`
	StorageClass *string                 `json:"storageClass,omitempty"`
	StorageSize  string                  `json:"storageSize,omitempty"`
	Protocol     string                  `json:"protocol,omitempty"`
	Image        string                  `json:"image,omitempty"`
	Config       string                  `json:"config,omitempty"`
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
	Spec              PartitionSpec   `json:"spec,omitempty"`
	Status            PartitionStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PartitionList contains a list of Partition
type PartitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Partition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Partition{}, &PartitionList{})
}
