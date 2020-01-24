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

package nopaxos

import (
	"context"
	"fmt"
	"github.com/atomix/kubernetes-controller/pkg/apis/k8s/v1alpha1"
	k8sutil "github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sort"
	"strconv"
)

var log = logf.Log.WithName("controller_nopaxos")

const proxyName = "sequencer"

const (
	defaultProtocolImage  = "atomix/nopaxos-replica:latest"
	defaultSequencerImage = "atomix/nopaxos-proxy:latest"
)

const (
	defaultLeaderTimeoutMillis     = 5000
	defaultPingPeriodMillis        = 1000
	defaultCheckpointPeriodSeconds = 300
	defaultSyncPeriodSeconds       = 300
)

var _ reconcile.Reconciler = &Reconciler{}

// NewReconciler returns a new NOPaxos reconciler
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

	// Reconcile the headless partition service
	err = r.reconcileHeadlessService(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the sequencer ConfigMap
	err = r.reconcileProxyConfigMap(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the sequencer Deployment
	err = r.reconcileProxyDeployment(partition)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the sequencer Service
	err = r.reconcileProxyService(partition)
	if err != nil {
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

func (r *Reconciler) reconcileHeadlessService(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetPartitionHeadlessServiceName(partition), Namespace: partition.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addHeadlessService(partition)
	}
	return err
}

func (r *Reconciler) reconcileProxyConfigMap(partition *v1alpha1.Partition) error {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetProxyConfigMapName(proxyName, partition), Namespace: partition.Namespace}, cm)
	if err != nil && errors.IsNotFound(err) {
		err = r.addProxyConfigMap(partition)
	}
	return err
}

func (r *Reconciler) reconcileProxyDeployment(partition *v1alpha1.Partition) error {
	dep := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8sutil.GetProxyDeploymentName(proxyName, partition), Namespace: partition.Namespace}, dep)
	if err != nil && errors.IsNotFound(err) {
		err = r.addProxyDeployment(partition)
	}
	return err
}

func (r *Reconciler) reconcileProxyService(partition *v1alpha1.Partition) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), k8sutil.GetPartitionServiceNamespacedName(partition), service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addProxyService(partition)
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
	leaderTimeout := partition.Spec.NOPaxos.Protocol.LeaderTimeoutMillis
	if leaderTimeout == 0 {
		leaderTimeout = defaultLeaderTimeoutMillis
	}
	pingPeriod := partition.Spec.NOPaxos.Protocol.PingPeriodMillis
	if pingPeriod == 0 {
		pingPeriod = defaultPingPeriodMillis
	}
	checkpointPeriod := partition.Spec.NOPaxos.Protocol.CheckpointPeriodSeconds
	if checkpointPeriod == 0 {
		checkpointPeriod = defaultCheckpointPeriodSeconds
	}
	syncPeriod := partition.Spec.NOPaxos.Protocol.SyncPeriodSeconds
	if syncPeriod == 0 {
		syncPeriod = defaultSyncPeriodSeconds
	}

	config := map[string]interface{}{
		"leaderTimeout":      fmt.Sprintf("%dms", leaderTimeout),
		"pingInterval":       fmt.Sprintf("%dms", pingPeriod),
		"checkpointInterval": fmt.Sprintf("%dms", checkpointPeriod),
		"syncInterval":       fmt.Sprintf("%dms", syncPeriod),
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
	image := partition.Spec.NOPaxos.Protocol.Image
	if image == "" {
		image = defaultProtocolImage
	}
	pullPolicy := partition.Spec.NOPaxos.Protocol.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	set, err := k8sutil.NewPartitionStatefulSet(partition, image, pullPolicy, 5679)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(partition, set, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), set)
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

func (r *Reconciler) addProxyConfigMap(partition *v1alpha1.Partition) error {
	log.Info("Creating sequencer ConfigMap", "Name", partition.Name, "Namespace", partition.Namespace)
	cm := &corev1.ConfigMap{}
	name := types.NamespacedName{
		Name:      fmt.Sprintf("%s-session", partition.Name),
		Namespace: partition.Namespace,
	}
	if err := r.client.Get(context.TODO(), name, cm); err != nil {
		if errors.IsNotFound(err) {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.Name,
					Namespace: name.Namespace,
				},
				Data: map[string]string{
					"session": "0",
				},
			}
			if err := controllerutil.SetControllerReference(partition, cm, r.scheme); err != nil {
				return err
			}
			if err := r.client.Create(context.TODO(), cm); err != nil {
				return err
			}
			if err := r.client.Get(context.TODO(), name, cm); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	session, err := strconv.Atoi(cm.Data["session"])
	if err != nil {
		return err
	}
	session++
	cm.Data["session"] = fmt.Sprintf("%d", session)
	if err := r.client.Update(context.TODO(), cm); err != nil {
		return err
	}

	config := map[string]interface{}{
		"sessionId": fmt.Sprintf("%d", session),
	}
	cm, err = k8sutil.NewProxyConfigMap(proxyName, partition, config)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(partition, cm, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), cm)
}

func (r *Reconciler) addProxyDeployment(partition *v1alpha1.Partition) error {
	log.Info("Creating sequencer Deployment", "Name", partition.Name, "Namespace", partition.Namespace)
	image := partition.Spec.NOPaxos.Sequencer.Image
	if image == "" {
		image = defaultSequencerImage
	}
	pullPolicy := partition.Spec.NOPaxos.Sequencer.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	dep, err := k8sutil.NewProxyDeployment(proxyName, image, pullPolicy, partition)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(partition, dep, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), dep)
}

func (r *Reconciler) addProxyService(partition *v1alpha1.Partition) error {
	log.Info("Creating sequencer service", "Name", partition.Name, "Namespace", partition.Namespace)
	service := k8sutil.NewProxyService(partition)
	if err := controllerutil.SetControllerReference(partition, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
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
