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

package raft

import (
	"context"
	"fmt"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	k8sutil "github.com/atomix/atomix-k8s-controller/pkg/controller/util/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sort"
)

var log = logf.Log.WithName("controller_raft")

var _ reconcile.Reconciler = &Reconciler{}

const defaultImage = "atomix/atomix-raft-node:latest"

const (
	defaultHeartbeatPeriod = 1000
	defaultElectionTimeout = 5000
)

// NewReconciler returns a new Raft reconciler
func NewReconciler(client client.Client, scheme *runtime.Scheme, config *rest.Config) reconcile.Reconciler {
	return &Reconciler{
		client: client,
		scheme: scheme,
		config: config,
	}
}

// Reconciler reconciles a Partition object
type Reconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reads that state of the partition for a Partition object and makes changes based on the state read
// and what is in the Partition.Spec
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

	v1alpha1.SetPartitionDefaults(partition)

	// Reconcile the partition config map
	err = r.reconcileConfigMap(partition)
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

	// Reconcile the partition status
	err = r.reconcileStatus(partition)
	if err != nil {
		if errors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcileConfigMap(partition *v1alpha1.Partition) error {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetPartitionConfigMapName(partition), Namespace: partition.Namespace}, cm)
	if err != nil && errors.IsNotFound(err) {
		err = r.addConfigMap(partition)
	}
	return err
}

func (r *Reconciler) reconcileDisruptionBudget(partition *v1alpha1.Partition) error {
	budget := &v1beta1.PodDisruptionBudget{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetPartitionDisruptionBudgetName(partition), Namespace: partition.Namespace}, budget)
	if err != nil && errors.IsNotFound(err) {
		err = r.addDisruptionBudget(partition)
	}
	return err
}

func (r *Reconciler) reconcileStatefulSet(partition *v1alpha1.Partition) error {
	set := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetPartitionStatefulSetName(partition), Namespace: partition.Namespace}, set)
	if err != nil && errors.IsNotFound(err) {
		err = r.addStatefulSet(partition)
	}
	return err
}

func (r *Reconciler) reconcileService(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), k8sutil.GetPartitionServiceNamespacedName(partition), service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addService(partition)
	}
	return err
}

func (r *Reconciler) reconcileHeadlessService(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetPartitionHeadlessServiceName(partition), Namespace: partition.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addHeadlessService(partition)
	}
	return err
}

func (r *Reconciler) reconcileEndpoints(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), k8sutil.GetPartitionServiceNamespacedName(partition), service)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if service.Spec.ClusterIP != "" {
		err = r.addEndpoints(partition, service)
	}
	return err
}

func (r *Reconciler) reconcileStatus(partition *v1alpha1.Partition) error {
	set := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetPartitionStatefulSetName(partition), Namespace: partition.Namespace}, set)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if set.Status.ReadyReplicas != partition.Status.ReadyReplicas {
		log.Info("Updating partition status", "Name", partition.Name, "Namespace", partition.Namespace)
		partition.Status.ReadyReplicas = set.Status.ReadyReplicas
		err = r.client.Status().Update(context.TODO(), partition)
	}
	return err
}

func (r *Reconciler) addConfigMap(partition *v1alpha1.Partition) error {
	log.Info("Creating node ConfigMap", "Name", partition.Name, "Namespace", partition.Namespace)
	heartbeatPeriod := partition.Spec.Raft.HeartbeatPeriodMillis
	if heartbeatPeriod == 0 {
		heartbeatPeriod = defaultHeartbeatPeriod
	}
	electionTimeout := partition.Spec.Raft.ElectionTimeoutMillis
	if electionTimeout == 0 {
		electionTimeout = defaultElectionTimeout
	}

	config := map[string]interface{}{
		"heartbeatInterval": fmt.Sprintf("%dms", heartbeatPeriod),
		"electionTimeout":   fmt.Sprintf("%dms", electionTimeout),
	}
	cm, err := k8sutil.NewPartitionConfigMap(partition, config)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(partition, cm, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), cm)
}

func (r *Reconciler) addStatefulSet(partition *v1alpha1.Partition) error {
	log.Info("Creating partition set", "Name", partition.Name, "Namespace", partition.Namespace)
	image := partition.Spec.Raft.Image
	if image == "" {
		image = defaultImage
	}
	pullPolicy := partition.Spec.Raft.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	set, err := k8sutil.NewPartitionStatefulSet(partition, image, pullPolicy, 5678)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(partition, set, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), set)
}

func (r *Reconciler) addService(partition *v1alpha1.Partition) error {
	log.Info("Creating partition service", "Name", partition.Name, "Namespace", partition.Namespace)
	service := k8sutil.NewPartitionService(partition)
	if err := controllerutil.SetControllerReference(partition, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *Reconciler) addHeadlessService(partition *v1alpha1.Partition) error {
	log.Info("Creating headless partition service", "Name", partition.Name, "Namespace", partition.Namespace)
	service := k8sutil.NewPartitionHeadlessService(partition)
	if err := controllerutil.SetControllerReference(partition, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *Reconciler) addDisruptionBudget(partition *v1alpha1.Partition) error {
	log.Info("Creating pod disruption budget", "Name", partition.Name, "Namespace", partition.Namespace)
	budget := k8sutil.NewPartitionDisruptionBudget(partition)
	if err := controllerutil.SetControllerReference(partition, budget, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), budget)
}

func (r *Reconciler) addEndpoints(partition *v1alpha1.Partition, service *corev1.Service) error {
	log.Info("Creating endpoint", "Name", partition.Name, "Namespace", partition.Namespace)
	endpoints := &corev1.Endpoints{}
	err := r.client.Get(context.TODO(), k8sutil.GetPartitionPartitionGroupServiceNamespacedName(partition), endpoints)
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
			notReadyAddresses++
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
		Ports: k8sutil.NewPartitionSetEndpointPorts(),
	})
	notReadyAddresses++

	// Load the parent partition group.
	group := &v1alpha1.PartitionSet{}
	err = r.client.Get(context.TODO(), k8sutil.GetPartitionPartitionGroupNamespacedName(partition), group)
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
				i++
			}
		}
		sort.Slice(addresses, func(i, j int) bool {
			iid, err := k8sutil.GetPartitionIDFromPartitionName(addresses[i].Hostname)
			if err != nil {
				return false
			}
			jid, err := k8sutil.GetPartitionIDFromPartitionName(addresses[j].Hostname)
			if err != nil {
				return false
			}
			return iid < jid
		})

		endpoints.Subsets = []corev1.EndpointSubset{
			{
				Addresses: addresses,
				Ports:     k8sutil.NewPartitionSetEndpointPorts(),
			},
		}
	}
	return r.client.Update(context.TODO(), endpoints)
}
