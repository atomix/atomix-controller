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

// ExampleProtocolName is the name of the example protocol
const ExampleProtocolName = "ExampleProtocol"

// ExampleProtocolSpec is the k8s spec for an ExampleProtocol resource
type ExampleProtocolSpec struct {
	Proxy   ExampleProtocolProxy   `json:"proxy,omitempty"`
	Backend ExampleProtocolBackend `json:"backend,omitempty"`
}

// ExampleProtocolProxy is the configuration for the example protocol proxy
type ExampleProtocolProxy struct {
}

// ExampleProtocolBackend is the configuration for the example protocol backend
type ExampleProtocolBackend struct {
	// Replicas is the number of backend replicas
	Replicas int32 `json:"replicas,omitempty"`
}

// ExampleProtocolStatus is the observed state of an ExampleProtocol
type ExampleProtocolStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExampleProtocol is the Schema for the ExampleProtocol API
// +k8s:openapi-gen=true
type ExampleProtocol struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the ExampleProtocol specification
	Spec ExampleProtocolSpec `json:"spec,omitempty"`

	// Status if the current status of the ExampleProtocol
	Status ExampleProtocolStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExampleProtocolList contains a list of Database
type ExampleProtocolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []ExampleProtocol `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExampleProtocol{}, &ExampleProtocolList{})
}
