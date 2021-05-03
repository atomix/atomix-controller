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
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	primitivesv2beta1 "github.com/atomix/atomix-controller/pkg/apis/primitives/v2beta1"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
)

var log = logging.GetLogger("atomix", "controller", "primitives")

func AddControllers(mgr manager.Manager) error {
	if err := addController(mgr, &primitivesv2beta1.Counter{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.Counter).Spec.Store
	}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Election{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.Election).Spec.Store
	}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.List{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.List).Spec.Store
	}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Lock{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.Lock).Spec.Store
	}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Map{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.Map).Spec.Store
	}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Set{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.Set).Spec.Store
	}); err != nil {
		return err
	}
	if err := addController(mgr, &primitivesv2beta1.Value{}, func(object runtime.Object) corev1.ObjectReference {
		return object.(*primitivesv2beta1.Value).Spec.Store
	}); err != nil {
		return err
	}
	return nil
}

func addController(mgr manager.Manager, object runtime.Object, storeGetter func(object runtime.Object) corev1.ObjectReference) error {
	kinds, _, err := mgr.GetScheme().ObjectKinds(object)
	if err != nil {
		return err
	}
	kind := kinds[0]

	r := &PrimitiveReconciler{
		client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		config:      mgr.GetConfig(),
		kind:        kind,
		storeGetter: storeGetter,
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
