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

package cluster

import (
	"context"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta2"
	"github.com/atomix/kubernetes-controller/pkg/controller/v1beta2/util/k8s"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	c, err := controller.New("cluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Cluster
	err = c.Watch(&source.Kind{Type: &v1beta2.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Partition
	err = c.Watch(&source.Kind{Type: &v1beta2.Partition{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1beta2.Cluster{},
	})
	if err != nil {
		return err
	}

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
	cluster := &v1beta2.Cluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{Requeue: true}, err
	}

	v1beta2.SetClusterDefaults(cluster)

	// Reconcile the partitions
	err = r.reconcilePartitions(cluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the cluster status
	if err := r.reconcileStatus(cluster); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcilePartitions(cluster *v1beta2.Cluster) error {
	clusterID, err := k8s.GetClusterIDFromClusterAnnotations(cluster)
	if err != nil {
		return err
	}
	for partitionID := (cluster.Spec.Partitions * (clusterID - 1)) + 1; partitionID <= cluster.Spec.Partitions*clusterID; partitionID++ {
		if err := r.reconcilePartition(cluster, partitionID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcilePartition(cluster *v1beta2.Cluster, partitionID int32) error {
	partition := &v1beta2.Partition{}
	err := r.client.Get(context.TODO(), k8s.GetPartitionNamespacedName(cluster, partitionID), partition)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		log.Info("Creating Partition", "Name", cluster.Name, "Namespace", cluster.Namespace)
		partition = k8s.NewPartition(cluster, partitionID)
		if err := controllerutil.SetControllerReference(cluster, partition, r.scheme); err != nil {
			return err
		}
		return r.client.Create(context.TODO(), partition)
	}
	return nil
}

func (r *Reconciler) reconcileStatus(cluster *v1beta2.Cluster) error {
	if cluster.Status.ReadyPartitions < cluster.Spec.Partitions {
		clusterID, err := k8s.GetClusterIDFromClusterAnnotations(cluster)
		if err != nil {
			return err
		}

		var readyPartitions int32
		for partitionID := (cluster.Spec.Partitions * (clusterID - 1)) + 1; partitionID <= cluster.Spec.Partitions*clusterID; partitionID++ {
			partition := &v1beta2.Partition{}
			err := r.client.Get(context.TODO(), k8s.GetPartitionNamespacedName(cluster, partitionID), partition)
			if err != nil {
				if !errors.IsNotFound(err) {
					return err
				}
				return nil
			}
			if partition.Status.Ready {
				readyPartitions++
			}
		}

		if readyPartitions != cluster.Status.ReadyPartitions {
			cluster.Status.ReadyPartitions = readyPartitions
			log.Info("Updating Cluster status", "Name", cluster.Name, "Namespace", cluster.Namespace, "ReadyPartitions", cluster.Status.ReadyPartitions)
			return r.client.Status().Update(context.TODO(), cluster)
		}
	}
	return nil
}
