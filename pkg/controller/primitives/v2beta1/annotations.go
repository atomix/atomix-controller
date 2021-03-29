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
	"fmt"
	"github.com/atomix/api/go/atomix/management/broker"
)

const (
	brokerInjectAnnotation              = "broker.atomix.io/inject"
	brokerInjectStatusAnnotation        = "broker.atomix.io/status"
	profileAnnotation                   = "storage.atomix.io/profile"
	enableEphemeralContainersAnnotation = "storage.atomix.io/enable-ephemeral-containers"
	injectedStatus                      = "injected"
)

func getStorageInjectAnnotation(storageType string) string {
	return fmt.Sprintf("%s.storage.atomix.io/inject", storageType)
}

func getStorageStatusAnnotation(storageType string) string {
	return fmt.Sprintf("%s.storage.atomix.io/status", storageType)
}

func getStorageInstancesAnnotation(storageType string) string {
	return fmt.Sprintf("%s.storage.atomix.io/instances", storageType)
}

func getDriverStatusAnnotation(driver broker.DriverId) string {
	return fmt.Sprintf("%s.driver.atomix.io/status", driver.Name)
}

func getDriverPortAnnotation(driver broker.DriverId) string {
	return fmt.Sprintf("%s.driver.atomix.io/port", driver.Name)
}
