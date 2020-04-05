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

package protocol

import (
	"context"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_database")

const finalizerName = "protocol.cloud.atomix.io"

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &Reconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("protocol-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Protocol
	err = c.Watch(&source.Kind{Type: &v1beta1.Protocol{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles a Database object
type Reconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reads that state of the cluster for a Database object and makes changes based on the state read
// and what is in the Database.Spec
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Protocol")

	// Fetch the Protocol instance
	protocol := &v1beta1.Protocol{}
	err := r.client.Get(context.TODO(), request.NamespacedName, protocol)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Add finalizers to the protocol if necessary. Otherwise, ensure resources have been deleted.
	if protocol.DeletionTimestamp == nil {
		if err := r.reconcileCreate(protocol); err != nil {
			return reconcile.Result{}, err
		}
	} else {
		if err := r.reconcileDelete(protocol); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcileCreate(protocol *v1beta1.Protocol) error {
	if protocol.Finalizers == nil || len(protocol.Finalizers) == 0 {
		protocol.Finalizers = []string{finalizerName}
		return r.client.Update(context.TODO(), protocol)
	}
	return nil
}

func (r *Reconciler) reconcileDelete(protocol *v1beta1.Protocol) error {
	if protocol.Finalizers != nil && len(protocol.Finalizers) > 0 && protocol.Finalizers[0] == finalizerName {
		databases := &v1beta1.DatabaseList{}
		err := r.client.List(context.TODO(), &client.ListOptions{}, databases)
		if err != nil {
			return err
		}
		for _, database := range databases.Items {
			kind := database.Spec.Template.Spec.Protocol.GroupVersionKind()
			if kind.Group == protocol.Spec.Group && kind.Version == protocol.Spec.Version && kind.Kind == protocol.Spec.Kind {
				err := r.client.Delete(context.TODO(), &database)
				if err != nil && !errors.IsNotFound(err) {
					return err
				}
			}
		}
	}
	protocol.Finalizers = []string{}
	return r.client.Update(context.TODO(), protocol)
}
