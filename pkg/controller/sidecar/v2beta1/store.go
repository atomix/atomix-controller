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
	"context"
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	sidecarv2beta1 "github.com/atomix/atomix-controller/pkg/apis/sidecar/v2beta1"
	"github.com/atomix/atomix-controller/pkg/controller/util/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const storeFinalizer = "agent"

func addStoreController(mgr manager.Manager) error {
	// Create a new controller
	options := controller.Options{
		Reconciler: &StoreReconciler{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
			config: mgr.GetConfig(),
		},
	}
	controller, err := controller.New("store-controller", mgr, options)
	if err != nil {
		return err
	}

	// Watch for changes to Stores
	err = controller.Watch(&source.Kind{Type: &corev2beta1.Store{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

// StoreReconciler is a Reconciler for Stores
type StoreReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reconciles Store resources
func (r *StoreReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Store '%s'", request.NamespacedName)
	store := &corev2beta1.Store{}
	err := r.client.Get(context.TODO(), request.NamespacedName, store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err)
		return reconcile.Result{}, err
	}

	if store.DeletionTimestamp != nil {
		return r.reconcileDelete(store)
	}
	return r.reconcileCreate(store)
}

func (r *StoreReconciler) reconcileCreate(store *corev2beta1.Store) (reconcile.Result, error) {
	if !k8s.HasFinalizer(store.Finalizers, storeFinalizer) {
		log.Infof("Adding finalizer to Store %s", types.NamespacedName{Namespace: store.Namespace, Name: store.Name})
		store.Finalizers = k8s.AddFinalizer(store.Finalizers, storeFinalizer)
		if err := r.client.Update(context.TODO(), store); err != nil {
			log.Error(err)
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *StoreReconciler) reconcileDelete(store *corev2beta1.Store) (reconcile.Result, error) {
	if !k8s.HasFinalizer(store.Finalizers, storeFinalizer) {
		return reconcile.Result{}, nil
	}

	log.Infof("Deleting Agents for Store %s", types.NamespacedName{Namespace: store.Namespace, Name: store.Name})
	options := &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"store": string(store.UID),
			}),
		},
	}
	if err := r.client.DeleteAllOf(context.TODO(), &sidecarv2beta1.Agent{}, options); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return reconcile.Result{}, err
		}
	}

	log.Infof("Removing finalizer from Store %s", types.NamespacedName{Namespace: store.Namespace, Name: store.Name})
	store.Finalizers = k8s.RemoveFinalizer(store.Finalizers, storeFinalizer)
	if err := r.client.Update(context.TODO(), store); err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
