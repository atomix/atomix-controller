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

package partition

import (
	"context"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
)

var log = logf.Log.WithName("controller_partition")

// AddController creates a new Partition ManagementGroup and adds it to the Manager. The Manager will set fields on the ManagementGroup
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &PartitionReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("partition-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Partition
	err = c.Watch(&source.Kind{Type: &v1alpha1.Partition{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource StatefulSets and requeue the owner Partition
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1alpha1.Partition{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &PartitionReconciler{}

// PartitionReconciler reconciles a Partition object
type PartitionReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reads that state of the partition for a Partition object and makes changes based on the state read
// and what is in the Partition.Spec
func (r *PartitionReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Partition")

	// Fetch the Partition instance
	partition := &v1alpha1.Partition{}
	err := r.client.Get(context.TODO(), request.NamespacedName, partition)
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

	v1alpha1.SetDefaults_Partition(partition)

	// Reconcile the init script
	err = r.reconcileInitScript(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the pod disruption budget
	err = r.reconcileDisruptionBudget(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the StatefulSet
	err = r.reconcileStatefulSet(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the partition service
	err = r.reconcileService(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the headless partition service
	err = r.reconcileHeadlessService(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the partition group service endpoints
	err = r.reconcileEndpoints(partition)
	if err != nil {
		if errors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *PartitionReconciler) reconcileInitScript(partition *v1alpha1.Partition) error {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: util.GetPartitionInitConfigMapName(partition), Namespace: partition.Namespace}, cm)
	if err != nil && errors.IsNotFound(err) {
		err = r.addInitScript(partition)
	}
	return err
}

func (r *PartitionReconciler) reconcileDisruptionBudget(partition *v1alpha1.Partition) error {
	budget := &v1beta1.PodDisruptionBudget{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: util.GetPartitionDisruptionBudgetName(partition), Namespace: partition.Namespace}, budget)
	if err != nil && errors.IsNotFound(err) {
		err = r.addDisruptionBudget(partition)
	}
	return err
}

func (r *PartitionReconciler) reconcileStatefulSet(partition *v1alpha1.Partition) error {
	set := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: util.GetPartitionStatefulSetName(partition), Namespace: partition.Namespace}, set)
	if err != nil && errors.IsNotFound(err) {
		err = r.addStatefulSet(partition)
	}
	return err
}

func (r *PartitionReconciler) reconcileService(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), util.GetPartitionServiceNamespacedName(partition), service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addService(partition)
	}
	return err
}

func (r *PartitionReconciler) reconcileHeadlessService(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: util.GetPartitionHeadlessServiceName(partition), Namespace: partition.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addHeadlessService(partition)
	}
	return err
}

func (r *PartitionReconciler) reconcileEndpoints(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), util.GetPartitionServiceNamespacedName(partition), service)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}
	if service.Spec.ClusterIP != "" {
		err = r.addEndpoints(partition, service)
	}
	return err
}

func (r *PartitionReconciler) addInitScript(partition *v1alpha1.Partition) error {
	log.Info("Creating init ConfigMap", "Name", partition.Name, "Namespace", partition.Namespace)
	cm := util.NewPartitionInitConfigMap(partition)
	if err := controllerutil.SetControllerReference(partition, cm, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), cm)
}

func (r *PartitionReconciler) addStatefulSet(partition *v1alpha1.Partition) error {
	log.Info("Creating partition set", "Name", partition.Name, "Namespace", partition.Namespace)
	set, err := util.NewPartitionStatefulSet(partition)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(partition, set, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), set)
}

func (r *PartitionReconciler) addService(partition *v1alpha1.Partition) error {
	log.Info("Creating partition service", "Name", partition.Name, "Namespace", partition.Namespace)
	service := util.NewPartitionService(partition)
	if err := controllerutil.SetControllerReference(partition, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *PartitionReconciler) addHeadlessService(partition *v1alpha1.Partition) error {
	log.Info("Creating headless partition service", "Name", partition.Name, "Namespace", partition.Namespace)
	service := util.NewPartitionHeadlessService(partition)
	if err := controllerutil.SetControllerReference(partition, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *PartitionReconciler) addDisruptionBudget(partition *v1alpha1.Partition) error {
	log.Info("Creating pod disruption budget", "Name", partition.Name, "Namespace", partition.Namespace)
	budget := util.NewPartitionDisruptionBudget(partition)
	if err := controllerutil.SetControllerReference(partition, budget, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), budget)
}

func (r *PartitionReconciler) addEndpoints(partition *v1alpha1.Partition, service *corev1.Service) error {
	log.Info("Creating endpoint", "Name", partition.Name, "Namespace", partition.Namespace)
	endpoints := &corev1.Endpoints{}
	err := r.client.Get(context.TODO(), util.GetPartitionPartitionGroupServiceNamespacedName(partition), endpoints)
	if err != nil {
		return nil
	}

	// If the partition's service is already a member of the subsets, return.
	notReadyAddresses := 0
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.NotReadyAddresses {
			if address.Hostname == service.Name {
				return nil
			}
			notReadyAddresses += 1
		}
		for _, address := range subset.Addresses {
			if address.Hostname == service.Name {
				return nil
			}
		}
	}

	// Append the partition service to the endpoint subsets.
	endpoints.Subsets = append(endpoints.Subsets, corev1.EndpointSubset{
		NotReadyAddresses: []corev1.EndpointAddress{
			{
				IP:       service.Spec.ClusterIP,
				Hostname: service.Name,
			},
		},
		Ports: util.NewPartitionGroupEndpointPorts(),
	})
	notReadyAddresses += 1

	// Load the parent partition group.
	group := &v1alpha1.PartitionGroup{}
	err = r.client.Get(context.TODO(), util.GetPartitionPartitionGroupNamespacedName(partition), group)
	if err != nil {
		return err
	}

	// If all the partition services have been added to the endpoint subsets, sort the subsets.
	if notReadyAddresses == group.Spec.Partitions {
		log.Info("Updating endpoint addresses to ready", "Name", partition.Name, "Namespace", partition.Namespace)
		addresses := make([]corev1.EndpointAddress, notReadyAddresses)
		i := 0
		for _, subset := range endpoints.Subsets {
			for _, address := range subset.NotReadyAddresses {
				addresses[i] = address
				i += 1
			}
		}
		sort.Slice(addresses, func(i, j int) bool {
			iid, err := util.GetPartitionIdFromPartitionName(addresses[i].Hostname)
			if err != nil {
				return false
			}
			jid, err := util.GetPartitionIdFromPartitionName(addresses[j].Hostname)
			if err != nil {
				return false
			}
			return iid < jid
		})

		endpoints.Subsets = []corev1.EndpointSubset{
			{
				Addresses: addresses,
				Ports:     util.NewPartitionGroupEndpointPorts(),
			},
		}
	}
	return r.client.Update(context.TODO(), endpoints)
}
