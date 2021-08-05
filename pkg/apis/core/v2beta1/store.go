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

package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// StoreSpec is a storage protocol specification
type StoreSpec struct {
	Protocol runtime.RawExtension `json:"protocol,omitempty"`
}

// ReplicaStatus is a storage replica status
type ReplicaStatus struct {
	ID         string           `json:"id,omitempty"`
	NodeID     string           `json:"nodeId,omitempty"`
	Host       *string          `json:"host,omitempty"`
	Port       *int32           `json:"port,omitempty"`
	ExtraPorts map[string]int32 `json:"extraPorts,omitempty"`
	Ready      bool             `json:"ready,omitempty"`
}

// PartitionStatus is a storage partition status
type PartitionStatus struct {
	ID           uint32   `json:"id,omitempty"`
	Host         *string  `json:"host,omitempty"`
	Port         *int32   `json:"port,omitempty"`
	Replicas     []string `json:"replicas,omitempty"`
	ReadReplicas []string `json:"readReplicas,omitempty"`
	Ready        bool     `json:"ready,omitempty"`
}

// StoreStatus is a store status
type StoreStatus struct {
	Ready           bool           `json:"ready,omitempty"`
	Replicas        int32          `json:"replicas,omitempty"`
	ReadyReplicas   int32          `json:"readyReplicas,omitempty"`
	Partitions      int32          `json:"partitions,omitempty"`
	ReadyPartitions int32          `json:"readyPartitions,omitempty"`
	Protocol        ProtocolStatus `json:"protocol,omitempty"`
}

// ProtocolStatus is a protocol status
type ProtocolStatus struct {
	Revision   int64             `json:"revision,omitempty"`
	Replicas   []ReplicaStatus   `json:"replicas,omitempty"`
	Partitions []PartitionStatus `json:"partitions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Store is the Schema for the Store API
// +k8s:openapi-gen=true
type Store struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StoreSpec   `json:"spec,omitempty"`
	Status            StoreStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StoreList contains a list of Store
type StoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Store items in the list
	Items []Store `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Store{}, &StoreList{})
}
