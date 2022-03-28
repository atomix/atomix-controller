// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ProxySpec is a proxy specification
type ProxySpec struct {
	Pod         corev1.ObjectReference `json:"pod,omitempty"`
	Primitive   corev1.ObjectReference `json:"primitive,omitempty"`
	Agent       corev1.ObjectReference `json:"agent,omitempty"`
	Permissions ProxyPermissions       `json:"permissions,omitempty"`
	Config      *runtime.RawExtension  `json:"config,omitempty"`
}

// ProxyStatus is a proxy status
type ProxyStatus struct {
	ContainerID string `json:"containerID,omitempty"`
	Ready       bool   `json:"ready,omitempty"`
}

// ProxyPermissions is a proxy permissions status
type ProxyPermissions struct {
	Read  bool `json:"read,omitempty"`
	Write bool `json:"write,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Proxy is the Schema for the Proxy API
// +k8s:openapi-gen=true
type Proxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ProxySpec   `json:"spec,omitempty"`
	Status            ProxyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ProxyList contains a list of Proxy
type ProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Proxy items in the list
	Items []Proxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Proxy{}, &ProxyList{})
}
