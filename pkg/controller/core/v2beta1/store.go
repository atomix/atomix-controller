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
	"github.com/atomix/kubernetes-controller/pkg/apis/core/v2beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func addStoreController(mgr manager.Manager) error {
	r := &StoreReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("store-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Pods
	err = c.Watch(&source.Kind{Type: &v2beta1.Store{}}, &handler.EnqueueRequestForObject{})
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

func (r *StoreReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Store '%s'", request.NamespacedName)
	store := &v2beta1.Store{}
	err := r.client.Get(context.TODO(), request.NamespacedName, store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err)
		return reconcile.Result{}, err
	}

	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, store.Spec.Protocol.Raw)
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}

	protocol := object.(*unstructured.Unstructured)
	stored := &unstructured.Unstructured{}
	stored.SetGroupVersionKind(protocol.GroupVersionKind())
	name := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Name,
	}
	err = r.client.Get(context.TODO(), name, stored)
	if err == nil {
		return reconcile.Result{}, nil
	} else if !k8serrors.IsNotFound(err) {
		log.Error(err)
		return reconcile.Result{}, err
	}

	gvk, err := apiutil.GVKForObject(store, r.scheme)
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               store.GetName(),
		UID:                store.GetUID(),
		BlockOwnerDeletion: pointer.BoolPtr(true),
		Controller:         pointer.BoolPtr(true),
	}
	protocol.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	err = r.client.Create(context.TODO(), protocol)
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
