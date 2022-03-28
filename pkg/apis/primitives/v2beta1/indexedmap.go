// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IndexedMapSpec specifies an IndexedMap
type IndexedMapSpec struct {
	PrimitiveSpec `json:",inline"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IndexedMap is the Schema for the IndexedMap API
// +k8s:openapi-gen=true
type IndexedMap struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MapSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IndexedMapList contains a list of IndexedMap
type IndexedMapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []IndexedMap `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IndexedMap{}, &IndexedMapList{})
}
