// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var log = logging.GetLogger("atomix", "controller", "core")

// AddControllers adds core controllers to the given manager
func AddControllers(mgr manager.Manager) error {
	if err := addStoreController(mgr); err != nil {
		return err
	}
	return nil
}
