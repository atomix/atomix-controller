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

// DatabaseSpec is the k8s spec for a Database resource
type DatabaseSpec struct {
	// Clusters is the number of clusters in the database
	Clusters int32 `json:"clusters,omitempty"`

	// Partitions is the number of partitions in the database
	Partitions int32 `json:"partitions,omitempty"`

	// Template is the PartitionTemplateSpec to use to configure the partitions
	Template ClusterTemplateSpec `json:"template,omitempty"`
}

// ClusterTemplateSpec is a template for clusters managed by the set
type ClusterTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the ClusterSpec for the clusters in the set
	Spec ClusterSpec `json:"spec,omitempty"`
}

// DatabaseStatus defines the observed state of Partition
type DatabaseStatus struct {
	// ReadyClusters is the number of clusters in the database that have been marked ready
	ReadyClusters int32 `json:"readyClusters,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Database is the Schema for the Database API
// +k8s:openapi-gen=true
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the Database specification
	Spec DatabaseSpec `json:"spec,omitempty"`

	// Status if the current status of the Database
	Status DatabaseStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
