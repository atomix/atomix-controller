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

// ProtocolType is a primitive protocol type
type ProtocolType struct {
	Database      *DatabaseProtocolType      `json:"database,omitempty"`
	Gossip        *GossipProtocolType        `json:"gossip,omitempty"`
	PrimaryBackup *PrimaryBackupProtocolType `json:"primaryBackup,omitempty"`
}

// DatabaseProtocolType is a protocol type for databases
type DatabaseProtocolType struct {
	metav1.ObjectMeta `json:"database,omitempty"`
}

// GossipProtocolType is a gossip protocol type
type GossipProtocolType struct {
	metav1.ObjectMeta `json:"membershipGroup,omitempty"`
}

// PrimaryBackupProtocolType is a primary-backup protocol type
type PrimaryBackupProtocolType struct {
	metav1.ObjectMeta `json:"partitionGroup,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Protocol is the Schema for the protocol API
// +k8s:openapi-gen=true
type Protocol struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Type              ProtocolType `json:"type,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProtocolList contains a list of Protocol
type ProtocolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []Protocol `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Protocol{}, &ProtocolList{})
}
