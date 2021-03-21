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

package database

import (
	"context"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
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

var log = logf.Log.WithName("controller_database")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &Reconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("database-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Database
	err = c.Watch(&source.Kind{Type: &v1beta3.Database{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Partition and requeue the owner Database
	err = c.Watch(&source.Kind{Type: &v1beta3.Partition{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1beta3.Database{},
	})
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
	logger.Info("Reconciling Database")

	// Fetch the Database instance
	database := &v1beta3.Database{}
	err := r.client.Get(context.TODO(), request.NamespacedName, database)
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

	v1beta3.SetDatabaseDefaults(database)

	for i := 1; i <= int(database.Spec.Partitions); i++ {
		if err = r.reconcilePartition(database, i); err != nil {
			return reconcile.Result{}, err
		}
	}

	if err = r.reconcileStatus(database); err != nil {
		if errors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcilePartition(database *v1beta3.Database, partitionID int) error {
	partition := &v1beta3.Partition{}
	err := r.client.Get(context.TODO(), k8s.GetPartitionNamespacedName(database, partitionID), partition)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return r.addPartition(database, partitionID)
	}
	return nil
}

func (r *Reconciler) addPartition(database *v1beta3.Database, partitionID int) error {
	log.Info("Creating Partition", "Name", k8s.GetPartitionName(database, partitionID), "Namespace", database.Namespace)
	partition := k8s.NewPartition(database, partitionID)
	if err := controllerutil.SetControllerReference(database, partition, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), partition)
}

func (r *Reconciler) reconcileStatus(database *v1beta3.Database) error {
	readyPartitions := 0
	for i := 1; i <= int(database.Spec.Partitions); i++ {
		partition := &v1beta3.Partition{}
		err := r.client.Get(context.TODO(), k8s.GetPartitionNamespacedName(database, i), partition)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return err
		}
		if partition.Status.Ready {
			readyPartitions++
		}
	}

	if int(database.Status.ReadyPartitions) != readyPartitions {
		log.Info("Updating Database status", "Name", database.Name, "Namespace", database.Namespace, "ReadyPartitions", readyPartitions)
		database.Status.ReadyPartitions = int32(readyPartitions)
		return r.client.Status().Update(context.TODO(), database)
	}
	return nil
}
