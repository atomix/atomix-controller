// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageProfileSpec is the spec for a StorageProfile
type StorageProfileSpec struct {
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	Drivers  []string              `json:"drivers,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StorageProfile is the Schema for the StorageProfile API
// +k8s:openapi-gen=true
type StorageProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StorageProfileSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StorageProfileList contains a list of StorageProfile
type StorageProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of StorageProfile items in the list
	Items []StorageProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageProfile{}, &StorageProfileList{})
}
