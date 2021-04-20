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
)

// ProtocolSpec is a storage protocol specification
type ProtocolSpec struct {
	Driver     DriverSpec      `json:"driver,omitempty"`
	Replicas   []ReplicaSpec   `json:"replicas,omitempty"`
	Partitions []PartitionSpec `json:"partitions,omitempty"`
}

// DriverSpec is a storage driver specification
type DriverSpec struct {
	Name string `json:"name,omitempty"`
}

// ReplicaSpec is a storage replica specification
type ReplicaSpec struct {
	ID         string           `json:"id,omitempty"`
	NodeID     string           `json:"nodeId,omitempty"`
	Host       *string          `json:"host,omitempty"`
	Port       *int32           `json:"port,omitempty"`
	ExtraPorts map[string]int32 `json:"extraPorts,omitempty"`
}

// PartitionSpec is a storage partition specification
type PartitionSpec struct {
	ID       uint32   `json:"id,omitempty"`
	Replicas []string `json:"replicas,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Protocol is the Schema for the Protocol API
// +k8s:openapi-gen=true
type Protocol struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ProtocolSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProtocolList contains a list of Protocol
type ProtocolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Protocol items in the list
	Items []Protocol `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Protocol{}, &ProtocolList{})
}
