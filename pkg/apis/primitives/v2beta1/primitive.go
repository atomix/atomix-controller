// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PrimitiveSpec is the base specification for all primitives
type PrimitiveSpec struct {
	Store PrimitiveStore `json:"store,omitempty"`
}

// PrimitiveStore is a primitive store configuration
type PrimitiveStore struct {
	corev1.ObjectReference `json:",inline"`
	Config                 map[string]runtime.RawExtension `json:"config,omitempty"`
}
