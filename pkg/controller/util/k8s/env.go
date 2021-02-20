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

import (
	"fmt"
	"os"
)

// Scope :
type Scope string

const (
	// ClusterScope :
	ClusterScope Scope = "cluster"

	// NamespaceScope :
	NamespaceScope Scope = "namespace"
)

const (
	nameEnv        = "CONTROLLER_NAME"
	namespaceEnv   = "CONTROLLER_NAMESPACE"
	scopeEnv       = "CONTROLLER_SCOPE"
	webhookNameEnv = "WEBHOOK_NAME"
	webhookPathEnv = "WEBHOOK_PATH"
)

const (
	defaultNamespace = "kube-system"
	defaultScope     = ClusterScope
)

// GetName :
func GetName() string {
	name := os.Getenv(nameEnv)
	if name == "" {
		panic(fmt.Sprintf("'%s' environment variable not defined", nameEnv))
	}
	return name
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

// GetWebhookName :
func GetWebhookName() string {
	name := os.Getenv(webhookNameEnv)
	if name == "" {
		panic(fmt.Sprintf("'%s' environment variable not defined", webhookNameEnv))
	}
	return name
}

// GetWebhookPath :
func GetWebhookPath() string {
	path := os.Getenv(webhookPathEnv)
	if path == "" {
		panic(fmt.Sprintf("'%s' environment variable not defined", webhookPathEnv))
	}
	return path
}
