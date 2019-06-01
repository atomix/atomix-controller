/*
 * Copyright 2019 Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package partitiongroup

import (
	"context"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/util"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_partitiongroup")

// AddController creates a new PartitionGroup controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &PartitionGroupReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("partitiongroup-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PartitionGroup
	err = c.Watch(&source.Kind{Type: &v1alpha1.PartitionGroup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Partitions and requeue the owner PartitionGroup
	err = c.Watch(&source.Kind{Type: &v1alpha1.Partition{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1alpha1.PartitionGroup{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &PartitionGroupReconciler{}

// PartitionGroupReconciler reconciles a PartitionGroup object
type PartitionGroupReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reads that state of the partition for a PartitionGroup object and makes changes based on the state read
// and what is in the PartitionGroup.Spec
func (r *PartitionGroupReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling PartitionGroup")

	// Fetch the PartitionGroup instance
	group := &v1alpha1.PartitionGroup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, group)
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

	v1alpha1.SetDefaults_PartitionGroup(group)

	for i := 1; i <= group.Spec.Partitions; i++ {
		if err = r.reconcilePartition(group, i); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}


func (r *PartitionGroupReconciler) reconcilePartition(group *v1alpha1.PartitionGroup, id int) error {
	partition := &v1alpha1.Partition{}
	err := r.client.Get(context.TODO(), util.GetPartitionNamespacedName(group, id), partition)
	if err != nil && errors.IsNotFound(err) {
		err = r.addPartition(group, id)
	}
	return err
}

func (r *PartitionGroupReconciler) addPartition(group *v1alpha1.PartitionGroup, id int) error {
	log.Info("Creating partition", "Name", util.GetPartitionName(group, id), "Namespace", group.Namespace)
	partition := util.NewPartition(group, id)
	if err := controllerutil.SetControllerReference(group, partition, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), partition)
}
