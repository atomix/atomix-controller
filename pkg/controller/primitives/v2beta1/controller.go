// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	"fmt"
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	primitivesv2beta1 "github.com/atomix/atomix-controller/pkg/apis/primitives/v2beta1"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
)

var log = logging.GetLogger("atomix", "controller", "primitives")

// AddControllers adds primitive controllers to the given manager
func AddControllers(mgr manager.Manager) error {
	if err := addController(mgr, &primitivesv2beta1.Counter{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Election{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.IndexedMap{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.List{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Lock{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Map{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Set{}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Value{}); err != nil {
		return err
	}
	return nil
}

func addController(mgr manager.Manager, object runtime.Object) error {
	kinds, _, err := mgr.GetScheme().ObjectKinds(object)
	if err != nil {
		return err
	}
	kind := kinds[0]

	r := &PrimitiveReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
		kind:   kind,
	}

	// Create a new controller
	c, err := controller.New(fmt.Sprintf("%s-controller", strings.ToLower(kind.Kind)), mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource
	err = c.Watch(&source.Kind{Type: object}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Primitive{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    object,
		IsController: true,
	})
	if err != nil {
		return err
	}
	return nil
}
