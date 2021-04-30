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
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strconv"
	"strings"
)

const (
	baseProtocolPort = 55680
)

func isPrimitiveReadyCondition(condition corev1.PodConditionType) bool {
	return strings.HasSuffix(string(condition), ".primitive.atomix.io/ready")
}

// NewPrimitiveConditions returns new conditions helper for the given driver with the given conditions
func NewPrimitiveConditions(primitive string, conditions []corev1.PodCondition) *PrimitiveConditions {
	return &PrimitiveConditions{
		Primitive:  primitive,
		Conditions: conditions,
	}
}

// PrimitiveConditions provides utility functions for driver conditions
type PrimitiveConditions struct {
	Primitive  string
	Conditions []corev1.PodCondition
}

func (d *PrimitiveConditions) getReadyType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("primitives.atomix.io/%s", d.Primitive))
}

func (d *PrimitiveConditions) getReadAccessType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.primitives.atomix.io/read-access", d.Primitive))
}

func (d *PrimitiveConditions) getWriteAccessType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.primitives.atomix.io/write-access", d.Primitive))
}

func (d *PrimitiveConditions) getConditionStatus(conditionType corev1.PodConditionType) corev1.ConditionStatus {
	for _, condition := range d.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionUnknown
}

func (d *PrimitiveConditions) setCondition(condition corev1.PodCondition) []corev1.PodCondition {
	for i, c := range d.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				d.Conditions[i] = condition
			}
			return d.Conditions
		}
	}
	d.Conditions = append(d.Conditions, condition)
	return d.Conditions
}

func (d *PrimitiveConditions) GetReady() corev1.ConditionStatus {
	return d.getConditionStatus(d.getReadyType())
}

func (d *PrimitiveConditions) SetReady(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getReadyType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d *PrimitiveConditions) GetReadAccess() corev1.ConditionStatus {
	return d.getConditionStatus(d.getReadAccessType())
}

func (d *PrimitiveConditions) SetReadAccess(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getReadAccessType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d *PrimitiveConditions) GetWriteAccess() corev1.ConditionStatus {
	return d.getConditionStatus(d.getWriteAccessType())
}

func (d *PrimitiveConditions) SetWriteAccess(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getWriteAccessType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func isProtocolReadyCondition(condition corev1.PodConditionType) bool {
	return strings.HasSuffix(string(condition), ".protocol.atomix.io/ready")
}

// NewProtocolConditions returns new conditions helper for the given driver with the given conditions
func NewProtocolConditions(protocol types.NamespacedName, conditions []corev1.PodCondition) *ProtocolConditions {
	return &ProtocolConditions{
		Protocol:   protocol,
		Conditions: conditions,
	}
}

// ProtocolConditions provides utility functions for driver conditions
type ProtocolConditions struct {
	Protocol   types.NamespacedName
	Conditions []corev1.PodCondition
}

func (d *ProtocolConditions) getReadyType(port int) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("agent.%s.%s.protocols.atomix.io/%d", d.Protocol.Name, d.Protocol.Namespace, port))
}

func (d *ProtocolConditions) findReadyType() (corev1.PodConditionType, bool) {
	for _, condition := range d.Conditions {
		if strings.HasPrefix(string(condition.Type), fmt.Sprintf("agent.%s.%s.protocols.atomix.io/", d.Protocol.Name, d.Protocol.Namespace)) {
			return condition.Type, true
		}
	}
	return "", false
}

func (d *ProtocolConditions) getRevisionType(generation int64) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("revision.%s.%s.protocols.atomix.io/%d", d.Protocol.Name, d.Protocol.Namespace, generation))
}

func (d *ProtocolConditions) getConditionStatus(conditionType corev1.PodConditionType) corev1.ConditionStatus {
	for _, condition := range d.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionUnknown
}

func (d *ProtocolConditions) setCondition(condition corev1.PodCondition) []corev1.PodCondition {
	for i, c := range d.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				d.Conditions[i] = condition
			}
			return d.Conditions
		}
	}
	d.Conditions = append(d.Conditions, condition)
	return d.Conditions
}

func (d *ProtocolConditions) GetReady() corev1.ConditionStatus {
	readyType, ok := d.findReadyType()
	if !ok {
		return corev1.ConditionUnknown
	}
	return d.getConditionStatus(readyType)
}

func (d *ProtocolConditions) SetReady(status corev1.ConditionStatus) []corev1.PodCondition {
	readyType, ok := d.findReadyType()
	if !ok {
		port := baseProtocolPort
		for _, condition := range d.Conditions {
			parts := strings.Split(string(condition.Type), "/")
			if len(parts) != 2 {
				continue
			}
			domain, path := parts[0], parts[1]
			if strings.HasPrefix(domain, "agent.") && strings.HasSuffix(domain, ".protocols.atomix.io") {
				i, _ := strconv.Atoi(path)
				if i > port {
					port = i + 1
				}
			}
		}
		readyType = d.getReadyType(port)
	}
	return d.setCondition(corev1.PodCondition{
		Type:               readyType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d *ProtocolConditions) GetPort() (int, error) {
	readyType, ok := d.findReadyType()
	if !ok {
		println(d.Protocol.Namespace)
		println(d.Protocol.Name)
		println(fmt.Sprintf("%s", d.Conditions))
		println(fmt.Sprintf("%v", d.Conditions))
		return 0, errors.New("missing port condition")
	}
	parts := strings.Split(string(readyType), "/")
	if len(parts) != 2 {
		return 0, errors.New("malformed port condition")
	}
	return strconv.Atoi(parts[1])
}

func (d *ProtocolConditions) GetRevision(revision int64) corev1.ConditionStatus {
	return d.getConditionStatus(d.getRevisionType(revision))
}

func (d *ProtocolConditions) SetRevision(revision int64, status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getRevisionType(revision),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}
