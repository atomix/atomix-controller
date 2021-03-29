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
	"github.com/atomix/api/go/atomix/protocol"
	"github.com/atomix/kubernetes-controller/pkg/apis/primitives/v2beta1"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type ProtocolResolver func(driver broker.DriverId) (protocol.ProtocolConfig, error)

// StorageController is an interface for implementing storage controllers
type StorageController interface {
	GetProtocol(driver broker.DriverId) (protocol.ProtocolConfig, error)
	NewContainer(driver broker.DriverConfig) (corev1.Container, error)
	NewEphemeralContainer(driver broker.DriverConfig) (corev1.EphemeralContainer, error)
}

// NewControllerBuilder creates a new controller builder
func NewControllerBuilder(name string) *ControllerBuilder {
	return &ControllerBuilder{
		name: name,
	}
}

// ControllerBuilder is a Kubernetes controller builder
type ControllerBuilder struct {
	name        string
	controller  StorageController
	storageType runtime.Object
	primitives  []primitiveType
}

func (b *ControllerBuilder) WithResourceType(storageType runtime.Object) *ControllerBuilder {
	b.storageType = storageType
	return b
}

func (b *ControllerBuilder) WithStorageController(controller StorageController) *ControllerBuilder {
	b.controller = controller
	return b
}

func (b *ControllerBuilder) AddPrimitiveType(object runtime.Object, list runtime.Object) *ControllerBuilder {
	b.primitives = append(b.primitives, primitiveType{
		object: object,
		list:   list,
	})
	return b
}

func (b *ControllerBuilder) Build(mgr manager.Manager) (controller.Controller, error) {
	kinds, _, err := mgr.GetScheme().ObjectKinds(b.storageType)
	if err != nil {
		return nil, err
	} else if len(kinds) == 0 {
		return nil, fmt.Errorf("unknown object kind %s", reflect.TypeOf(b.storageType).Name())
	}
	storageAPIVersion, storageKind := kinds[0].ToAPIVersionAndKind()

	mgr.GetWebhookServer().Register(k8s.GetWebhookPath(), &webhook.Admission{
		Handler: &DriverWebhook{
			client:            mgr.GetClient(),
			scheme:            mgr.GetScheme(),
			storageType:       b.name,
			storageAPIVersion: storageAPIVersion,
			storageKind:       storageKind,
			storageController: b.controller,
		},
	})

	r := &DriverReconciler{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		config:            mgr.GetConfig(),
		storageType:       b.name,
		storageAPIVersion: storageAPIVersion,
		storageKind:       storageKind,
		storageController: b.controller,
		primitiveTypes:    b.primitives,
	}

	// Create a new controller
	c, err := controller.New("primitives", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Watch for changes to primary resource Pod
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return nil, err
	}

	// Watch for changes to storage resource
	err = c.Watch(&source.Kind{Type: b.storageType}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Counter resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Counter{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Election resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Election{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Lock resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Lock{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary List resource
	err = c.Watch(&source.Kind{Type: &v2beta1.List{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Map resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Map{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Set resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Set{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Value resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Value{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource Role
	err = c.Watch(&source.Kind{Type: &rbacv1.Role{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource RoleBinding
	err = c.Watch(&source.Kind{Type: &rbacv1.RoleBinding{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource ClusterRole
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRole{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource ClusterRoleBinding
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}
