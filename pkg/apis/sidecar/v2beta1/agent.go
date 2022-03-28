// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec is an agent specification
type AgentSpec struct {
	Port  int32                  `json:"port,omitempty"`
	Pod   corev1.ObjectReference `json:"pod,omitempty"`
	Store corev1.ObjectReference `json:"store,omitempty"`
}

// AgentStatus is an agent status
type AgentStatus struct {
	ContainerID string `json:"containerID,omitempty"`
	Ready       bool   `json:"ready,omitempty"`
	Revision    int64  `json:"revision,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Agent is the Schema for the Agent API
// +k8s:openapi-gen=true
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AgentSpec   `json:"spec,omitempty"`
	Status            AgentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of Agent items in the list
	Items []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
