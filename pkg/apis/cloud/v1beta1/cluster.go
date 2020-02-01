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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterSpec is the k8s configuration for a single partition
type ClusterSpec struct {
	// Partitions is the number of partitions in the cluster
	Partitions int32 `json:"partitions,omitempty"`

	// Proxy is an optional proxy configuration
	Proxy *Proxy `json:"proxy,omitempty"`

	// Backend is the cluster backend
	Backend Backend `json:"backend,omitempty"`

	// VolumeClaimTemplates is a list of templates for persistent volume claims
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	// ReadyPartitions is the number of partitions that are ready in the cluster
	ReadyPartitions int32 `json:"readyPartitions,omitempty"`

	// Proxy is the proxy status
	Proxy *ProxyStatus `json:"proxy,omitempty"`

	// Backend is the backend status
	Backend BackendStatus `json:"backend,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Cluster is the Schema for the atomixclusters API
// +k8s:openapi-gen=true
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the spec for the partition
	Spec ClusterSpec `json:"spec,omitempty"`

	// Status is the status of the partition
	Status ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
