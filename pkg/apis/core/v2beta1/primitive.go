// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PrimitiveSpec is the spec for a Primitive
type PrimitiveSpec struct {
	Type  string         `json:"type,omitempty"`
	Store PrimitiveStore `json:"store,omitempty"`
}

// PrimitiveStore is a primitive store configuration
type PrimitiveStore struct {
	corev1.ObjectReference `json:",inline"`
	Config                 map[string]runtime.RawExtension `json:"config,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Primitive is the Schema for the Primitive API
// +k8s:openapi-gen=true
type Primitive struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PrimitiveSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PrimitiveList contains a list of Primitive
type PrimitiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Primitive items in the list
	Items []Primitive `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Primitive{}, &PrimitiveList{})
}
