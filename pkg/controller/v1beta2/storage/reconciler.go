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

package storage

import (
	"context"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// AddClusterReconciler adds a new cluster reconciler for the given controller manager and reconciler
func AddClusterReconciler(mgr manager.Manager, reconciler reconcile.Reconciler, kind schema.GroupVersionKind) error {
	r := &Reconciler{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		config:     mgr.GetConfig(),
		reconciler: reconciler,
	}

	obj, err := r.scheme.New(kind)
	if err != nil {
		return err
	}

	// Create a new controller
	c, err := controller.New(mgr.GetScheme().Name(), mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to the storage resource and enqueue Clusters that reference it
	err = c.Watch(&source.Kind{Type: obj}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &clusterMapper{
			client: r.client,
			kind:   kind,
		},
	})

	// Watch for changes to referencing resource Clusters
	err = c.Watch(&source.Kind{Type: &v1beta2.Cluster{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &storageFilter{
			client: r.client,
			kind:   kind,
		},
	})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles a Cluster object
type Reconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client     client.Client
	scheme     *runtime.Scheme
	config     *rest.Config
	reconciler reconcile.Reconciler
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	cluster := &v1beta2.Cluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{Requeue: true}, err
	}

	name := types.NamespacedName{
		Namespace: cluster.Spec.Storage.Namespace,
		Name:      cluster.Spec.Storage.Name,
	}
	gvk := schema.GroupVersionKind{
		Group:   cluster.Spec.Storage.Group,
		Version: cluster.Spec.Storage.Version,
		Kind:    cluster.Spec.Storage.Kind,
	}
	obj, err := r.scheme.New(gvk)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.client.Get(context.TODO(), name, obj)
	if err != nil {
		// If the TestStorage instance is not found we cannot reconcile the Cluster.
		// Wait for something else to happen.
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{Requeue: true}, err
	}
	return r.reconciler.Reconcile(request)
}

// clusterMapper is a request mapper that triggers the reconciler for referencing Clusters
// when an TestStorage is changed
type clusterMapper struct {
	client client.Client
	kind   schema.GroupVersionKind
}

func (m *clusterMapper) Map(object handler.MapObject) []reconcile.Request {
	// Find all clusters that reference the changed storage
	clusters := &v1beta2.ClusterList{}
	err := m.client.List(context.TODO(), clusters, &client.ListOptions{})
	if err != nil {
		return []reconcile.Request{}
	}

	// Iterate through clusters and requeue any that reference the storage controller
	requests := []reconcile.Request{}
	for _, cluster := range clusters.Items {
		if cluster.Spec.Storage.Group == m.kind.Group &&
			cluster.Spec.Storage.Version == m.kind.Version &&
			cluster.Spec.Storage.Kind == m.kind.Kind &&
			cluster.Spec.Storage.Name == object.Meta.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: cluster.GetNamespace(),
					Name:      cluster.GetName(),
				},
			})
		}
	}
	return requests
}

// storageFilter is a request mapper that triggers the reconciler for the storage controller when
// referenced by a watched Cluster
type storageFilter struct {
	client client.Client
	kind   schema.GroupVersionKind
}

func (m *storageFilter) Map(object handler.MapObject) []reconcile.Request {
	cluster := object.Object.(*v1beta2.Cluster)

	// If the Cluster uses this controller's protocol, enqueue the request
	if cluster.Spec.Storage.Group == m.kind.Group &&
		cluster.Spec.Storage.Version == m.kind.Version &&
		cluster.Spec.Storage.Kind == m.kind.Kind {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: cluster.GetNamespace(),
					Name:      cluster.GetName(),
				},
			},
		}
	}
	return []reconcile.Request{}
}
