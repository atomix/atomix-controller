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

import corev1 "k8s.io/api/core/v1"

// Backend is a database backend configuration
type Backend struct {
	// Replicas is the number of backend replicas
	Replicas int32 `json:"replicas,omitempty"`

	// Image is the image to run
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the pull policy to apply
	ImagePullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// Env is a set of environment variables to pass to backend nodes
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources is the resources to allocate for the backend nodes
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Protocol is the backend protocol configuration
	Protocol *Protocol `json:"protocol,omitempty"`

	// VolumeClaim is an optional persistent volume claim to use for persistence
	VolumeClaim *corev1.PersistentVolumeClaim `json:"volumeClaim,omitempty"`
}

// BackendStatus is the cluster backend status
type BackendStatus struct {
	// ReadyReplicas is the number of replicas in the cluster that have been marked ready
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}
