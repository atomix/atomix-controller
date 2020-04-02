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

// Proxy is a proxy configuration
type Proxy struct {
	// Image is the image to run
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the pull policy to apply
	ImagePullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// Env is a set of environment variables to pass to proxy node
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources is the resources to allocate for the proxy node
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Protocol is the proxy protocol configuration
	Protocol *Protocol `json:"protocol,omitempty"`

	// LivenessProbe backend livenessProbe configuration
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`

	// ReadinessProbe backend readinessProbe configuration
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// Args proxy container args
	Args []string `json:"args,omitempty"`

	// Metadata backend metadata
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`
}

// ProxyStatus is the cluster proxy status
type ProxyStatus struct {
	// Ready indicates whether the proxy is ready
	Ready bool `json:"ready,omitempty"`
}
