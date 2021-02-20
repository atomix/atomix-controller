// Copyright 2020-present Open Networking Foundation.
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

package k8s

import "os"

// Scope :
type Scope string

const (
	// ClusterScope :
	ClusterScope Scope = "cluster"

	// NamespaceScope :
	NamespaceScope Scope = "namespace"
)

const (
	nameEnv      = "CONTROLLER_NAME"
	namespaceEnv = "CONTROLLER_NAMESPACE"
	scopeEnv     = "CONTROLLER_SCOPE"
)

const (
	defaultNamespace = "kube-system"
	defaultScope     = ClusterScope
)

// GetName :
func GetName(def string) string {
	name := os.Getenv(nameEnv)
	if name != "" {
		return name
	}
	return def
}

// GetNamespace :
func GetNamespace() string {
	namespace := os.Getenv(namespaceEnv)
	if namespace != "" {
		return namespace
	}
	return defaultNamespace
}

// GetScope :
func GetScope() Scope {
	scope := os.Getenv(scopeEnv)
	if scope != "" {
		return Scope(scope)
	}
	return defaultScope
}
