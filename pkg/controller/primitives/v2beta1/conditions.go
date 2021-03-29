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
	corev1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

func getStorageReadyCondition(storageType string) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.storage.atomix.io/ready", storageType))
}

func getDriverReadyCondition(driver broker.DriverId) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.driver.atomix.io/ready", driver.Name))
}

func isDriverReadyCondition(condition corev1.PodConditionType) bool {
	return strings.HasSuffix(string(condition), ".driver.atomix.io/ready")
}

func getDriverReadyConditionDriver(condition corev1.PodConditionType) string {
	return string(condition[:strings.Index(string(condition), ".")])
}

func getDriverPortCondition(driver broker.DriverId, port int32) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.port.atomix.io/%d", driver.Name, port))
}

func isPortCondition(condition corev1.PodConditionType) bool {
	parts := strings.Split(string(condition), "/")
	return len(parts) == 2 && strings.HasSuffix(parts[0], ".port.atomix.io")
}

func isDriverPortCondition(condition corev1.PodConditionType, driver broker.DriverId) bool {
	parts := strings.Split(string(condition), "/")
	return len(parts) == 2 && parts[0] == fmt.Sprintf("%s.port.atomix.io", driver.Name)
}

func getDriverPortConditionValue(condition corev1.PodConditionType) int32 {
	parts := strings.Split(string(condition), "/")
	p, _ := strconv.Atoi(parts[1])
	return int32(p)
}
