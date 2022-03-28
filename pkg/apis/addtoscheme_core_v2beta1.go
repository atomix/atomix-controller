// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package apis

import (
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
)

func init() {
	// register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, corev2beta1.SchemeBuilder.AddToScheme)
}
