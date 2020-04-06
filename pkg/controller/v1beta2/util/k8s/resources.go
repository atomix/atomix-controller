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

package k8s

import (
	"fmt"
	"os"
)

const (
	appKey       = "app"
	atomixApp    = "atomix"
	typeKey      = "type"
	databaseKey  = "database"
	clusterKey   = "cluster"
	partitionKey = "partition"
)

const (
	controllerAnnotation = "cloud.atomix.io/controller"
	typeAnnotation       = "cloud.atomix.io/type"
	databaseAnnotation   = "cloud.atomix.io/database"
	clusterAnnotation    = "cloud.atomix.io/cluster"
)

const (
	clusterType   = "cluster"
	partitionType = "partition"
)

const (
	controllerNameVar      = "CONTROLLER_NAME"
	controllerNamespaceVar = "CONTROLLER_NAMESPACE"
)

const (
	defaultNamespace = "default"
)

// GetControllerName gets the name of the current controller from the environment
func GetControllerName() string {
	return os.Getenv(controllerNameVar)
}

// GetControllerNamespace gets the controller's namespace from the environment
func GetControllerNamespace() string {
	return os.Getenv(controllerNamespaceVar)
}

// GetQualifiedControllerName returns the qualified controller name
func GetQualifiedControllerName() string {
	return fmt.Sprintf("%s.%s", GetControllerNamespace(), GetControllerName())
}
