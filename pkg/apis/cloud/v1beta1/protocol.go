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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProtocolSpec is the k8s spec for a Protocol resource
type ProtocolSpec struct {
	metav1.GroupVersionKind `json:",inline"`
}

// ProtocolStatus defines the observed state of Protocol
type ProtocolStatus struct{}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Protocol is the Schema for the Protocol API
// +k8s:openapi-gen=true
type Protocol struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the protocol specification
	Spec ProtocolSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProtocolList contains a list of Protocol
type ProtocolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []Protocol `json:"items"`
}

// ProtocolReference is a reference to a protocol configuration
type ProtocolReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Protocol{}, &ProtocolList{})
}
