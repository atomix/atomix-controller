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

package v2beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PluginSpec is a storage plugin specification
type PluginSpec struct {
	Protocol PluginProtocol `json:"protocol,omitempty"`
	Drivers  []PluginDriver `json:"drivers,omitempty"`

	DeprecatedGroup    string                    `json:"group,omitempty"`
	DeprecatedKind     string                    `json:"kind,omitempty"`
	DeprecatedVersions []DeprecatedPluginVersion `json:"versions,omitempty"`
}

// PluginProtocol describes a storage protocol CRD
type PluginProtocol struct {
	Group string `json:"group,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// PluginDriver is storage plugin driver information
type PluginDriver struct {
	Version string        `json:"version,omitempty"`
	Image   string        `json:"image,omitempty"`
	Config  *DriverConfig `json:"config,omitempty"`
}

// DriverConfig describes a driver configuration CRD
type DriverConfig struct {
	Group string `json:"group,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// DeprecatedPluginVersion is a storage plugin version
type DeprecatedPluginVersion struct {
	Name   string                 `json:"name,omitempty"`
	Driver DeprecatedPluginDriver `json:"driver,omitempty"`
}

// DeprecatedPluginDriver is storage plugin driver information
type DeprecatedPluginDriver struct {
	Image string `json:"image,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StoragePlugin is the Schema for the StoragePlugin API
// +k8s:openapi-gen=true
type StoragePlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PluginSpec `json:"spec,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StoragePluginList contains a list of StoragePlugin
type StoragePluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of StoragePlugin items in the list
	Items []StoragePlugin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StoragePlugin{}, &StoragePluginList{})
}
