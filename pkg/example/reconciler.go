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

package example

import (
	"context"
	cloudv1beta1 "github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	protocolv1beta1 "github.com/atomix/kubernetes-controller/pkg/apis/protocol/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cluster")

// Add creates a new Partition ManagementGroup and adds it to the Manager. The Manager will set fields on the ManagementGroup
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &Reconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("example-protocol-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to resource ExampleProtocol and enqueue Clusters that reference it
	err = c.Watch(&source.Kind{Type: &protocolv1beta1.ExampleProtocol{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &clusterMapper{
			client: r.client,
		},
	})

	// Watch for changes to referencing resource Clusters
	err = c.Watch(&source.Kind{Type: &cloudv1beta1.Cluster{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &protocolFilter{
			client: r.client,
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
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reads that state of the cluster for a Cluster object and makes changes based on the state read
// and what is in the Cluster.Spec
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Cluster")

	// Fetch the Cluster instance
	cluster := &cloudv1beta1.Cluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{Requeue: true}, err
	}

	// Fetch the ExampleProtocol instance referenced by the Cluster
	protocol := &protocolv1beta1.ExampleProtocol{}
	err = r.client.Get(context.TODO(), request.NamespacedName, cluster)
	if err != nil {
		// If the ExampleProtocol instance is not found we cannot reconcile the Cluster.
		// Wait for something else to happen.
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{Requeue: true}, err
	}

	// The Cluster and ExampleProtocol are both available. Deploy the hell out of it!
	// ......
	return reconcile.Result{}, nil
}

// clusterMapper is a request mapper that triggers the reconciler for referencing Clusters
// when an ExampleProtocol is changed
type clusterMapper struct {
	client client.Client
}

func (m *clusterMapper) Map(object handler.MapObject) []reconcile.Request {
	protocol := object.Object.(*protocolv1beta1.ExampleProtocol)

	// Find all clusters that reference the changed protocol
	clusters := &cloudv1beta1.ClusterList{}
	err := m.client.List(context.TODO(), &client.ListOptions{}, clusters)
	if err != nil {
		return []reconcile.Request{}
	}

	// Iterate through clusters and requeue any that reference the protocol
	requests := []reconcile.Request{}
	for _, cluster := range clusters.Items {
		if cluster.Spec.Protocol.GroupVersionKind().Group == protocolv1beta1.SchemeGroupVersion.Group &&
			cluster.Spec.Protocol.GroupVersionKind().Version == protocolv1beta1.SchemeGroupVersion.Version &&
			cluster.Spec.Protocol.GroupVersionKind().Kind == protocolv1beta1.ExampleProtocolName &&
			cluster.Spec.Protocol.Name == protocol.Name {
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

// protocolFilter is a request mapper that triggers the reconciler for the protocol when
// referenced by a watched Cluster
type protocolFilter struct {
	client client.Client
}

func (m *protocolFilter) Map(object handler.MapObject) []reconcile.Request {
	cluster := object.Object.(*cloudv1beta1.Cluster)

	// If the Cluster uses this controller's protocol, enqueue the request
	if cluster.Spec.Protocol.GroupVersionKind().Group == protocolv1beta1.SchemeGroupVersion.Group &&
		cluster.Spec.Protocol.GroupVersionKind().Version == protocolv1beta1.SchemeGroupVersion.Version &&
		cluster.Spec.Protocol.GroupVersionKind().Kind == protocolv1beta1.ExampleProtocolName {
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
