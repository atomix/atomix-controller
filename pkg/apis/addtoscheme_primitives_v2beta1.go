// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package apis

import (
	primitivesv2beta1 "github.com/atomix/atomix-controller/pkg/apis/primitives/v2beta1"
)

func init() {
	// register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, primitivesv2beta1.SchemeBuilder.AddToScheme)
}
