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
	"fmt"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	"github.com/atomix/kubernetes-controller/pkg/controller/v1beta1/util/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"strconv"
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
	err = c.Watch(&source.Kind{Type: &v1beta1.Cluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource StatefulSets for readiness checks
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1beta1.Cluster{},
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
	cluster := &v1beta1.Cluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cluster)
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

	v1beta1.SetClusterDefaults(cluster)

	// Reconcile the partitions
	err = r.reconcilePartitions(cluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the backend
	err = r.reconcileBackend(cluster)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the proxy if configured
	if cluster.Spec.Proxy != nil {
		err = r.reconcileProxy(cluster)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// Reconcile the cluster status
	if err := r.reconcileStatus(cluster); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcilePartitions(cluster *v1beta1.Cluster) error {
	for _, partition := range cluster.Spec.Partitions {
		if err := r.reconcilePartition(cluster, partition); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcilePartition(cluster *v1beta1.Cluster, partitionID int64) error {
	partition := &v1beta1.Partition{}
	err := r.client.Get(context.TODO(), k8s.GetPartitionNamespacedName(cluster, partitionID), partition)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		partition = k8s.NewPartition(cluster, partitionID)
		return r.client.Create(context.TODO(), partition)
	}
	return nil
}

func (r *Reconciler) reconcileProxy(cluster *v1beta1.Cluster) error {
	// Reconcile the sequencer ConfigMap
	err := r.reconcileProxyConfigMap(cluster)
	if err != nil {
		return err
	}

	// Reconcile the sequencer Deployment
	err = r.reconcileProxyDeployment(cluster)
	if err != nil {
		return err
	}

	// Reconcile the sequencer Service
	err = r.reconcileProxyService(cluster)
	if err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) reconcileProxyConfigMap(cluster *v1beta1.Cluster) error {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetProxyConfigMapName(cluster), Namespace: cluster.Namespace}, cm)
	if err != nil && errors.IsNotFound(err) {
		err = r.addProxyConfigMap(cluster)
	}
	return err
}

func (r *Reconciler) reconcileProxyDeployment(cluster *v1beta1.Cluster) error {
	dep := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetProxyDeploymentName(cluster), Namespace: cluster.Namespace}, dep)
	if err != nil && errors.IsNotFound(err) {
		err = r.addProxyDeployment(cluster)
	}
	return err
}

func (r *Reconciler) reconcileProxyService(cluster *v1beta1.Cluster) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), k8s.GetClusterServiceNamespacedName(cluster), service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addProxyService(cluster)
	}
	return err
}

func (r *Reconciler) addProxyConfigMap(cluster *v1beta1.Cluster) error {
	log.Info("Creating proxy ConfigMap", "Name", cluster.Name, "Namespace", cluster.Namespace)
	cm := &corev1.ConfigMap{}
	name := types.NamespacedName{
		Name:      fmt.Sprintf("%s-session", cluster.Name),
		Namespace: cluster.Namespace,
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
			if err := controllerutil.SetControllerReference(cluster, cm, r.scheme); err != nil {
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
	cm, err = k8s.NewProxyConfigMap(cluster, config)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(cluster, cm, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), cm)
}

func (r *Reconciler) addProxyDeployment(cluster *v1beta1.Cluster) error {
	log.Info("Creating proxy Deployment", "Name", cluster.Name, "Namespace", cluster.Namespace)
	image := cluster.Spec.Proxy.Image
	pullPolicy := cluster.Spec.Proxy.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	dep, err := k8s.NewProxyDeployment(image, pullPolicy, cluster)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(cluster, dep, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), dep)
}

func (r *Reconciler) addProxyService(cluster *v1beta1.Cluster) error {
	log.Info("Creating proxy service", "Name", cluster.Name, "Namespace", cluster.Namespace)
	service := k8s.NewProxyService(cluster)
	if err := controllerutil.SetControllerReference(cluster, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *Reconciler) reconcileBackend(cluster *v1beta1.Cluster) error {
	// Reconcile the cluster config map
	err := r.reconcileBackendConfigMap(cluster)
	if err != nil {
		return err
	}

	// Reconcile the pod disruption budget
	err = r.reconcileBackendDisruptionBudget(cluster)
	if err != nil {
		return err
	}

	// Reconcile the StatefulSet
	err = r.reconcileBackendStatefulSet(cluster)
	if err != nil {
		return err
	}

	// Reconcile the headless cluster service
	err = r.reconcileHeadlessService(cluster)
	if err != nil {
		return err
	}

	// Reconcile the cluster service
	if cluster.Spec.Proxy == nil {
		err = r.reconcileBackendService(cluster)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileBackendConfigMap(cluster *v1beta1.Cluster) error {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetClusterConfigMapName(cluster), Namespace: cluster.Namespace}, cm)
	if err != nil && errors.IsNotFound(err) {
		err = r.addBackendConfigMap(cluster)
	}
	return err
}

func (r *Reconciler) reconcileBackendDisruptionBudget(cluster *v1beta1.Cluster) error {
	budget := &policyv1beta1.PodDisruptionBudget{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetClusterDisruptionBudgetName(cluster), Namespace: cluster.Namespace}, budget)
	if err != nil && errors.IsNotFound(err) {
		err = r.addBackendDisruptionBudget(cluster)
	}
	return err
}

func (r *Reconciler) reconcileBackendStatefulSet(cluster *v1beta1.Cluster) error {
	set := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetClusterStatefulSetName(cluster), Namespace: cluster.Namespace}, set)
	if err != nil && errors.IsNotFound(err) {
		err = r.addBackendStatefulSet(cluster)
	}
	return err
}

func (r *Reconciler) reconcileBackendService(cluster *v1beta1.Cluster) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), k8s.GetClusterServiceNamespacedName(cluster), service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addBackendService(cluster)
	}
	return err
}

func (r *Reconciler) reconcileHeadlessService(cluster *v1beta1.Cluster) error {
	service := &corev1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetClusterHeadlessServiceName(cluster), Namespace: cluster.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		err = r.addBackendHeadlessService(cluster)
	}
	return err
}

func (r *Reconciler) addBackendConfigMap(cluster *v1beta1.Cluster) error {
	log.Info("Creating backend ConfigMap", "Name", cluster.Name, "Namespace", cluster.Namespace)
	var config interface{}
	if cluster.Spec.Backend.Protocol != nil {
		config = cluster.Spec.Backend.Protocol.Custom
	} else {
		config = ""
	}
	cm, err := k8s.NewClusterConfigMap(cluster, config)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(cluster, cm, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), cm)
}

func (r *Reconciler) addBackendStatefulSet(cluster *v1beta1.Cluster) error {
	log.Info("Creating backend replicas", "Name", cluster.Name, "Namespace", cluster.Namespace)
	image := cluster.Spec.Backend.Image
	pullPolicy := cluster.Spec.Backend.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}
	set, err := k8s.NewBackendStatefulSet(cluster, image, pullPolicy, 5679)
	if err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(cluster, set, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), set)
}

func (r *Reconciler) addBackendService(cluster *v1beta1.Cluster) error {
	log.Info("Creating backend service", "Name", cluster.Name, "Namespace", cluster.Namespace)
	service := k8s.NewClusterService(cluster)
	if err := controllerutil.SetControllerReference(cluster, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *Reconciler) addBackendHeadlessService(cluster *v1beta1.Cluster) error {
	log.Info("Creating headless backend service", "Name", cluster.Name, "Namespace", cluster.Namespace)
	service := k8s.NewClusterHeadlessService(cluster)
	if err := controllerutil.SetControllerReference(cluster, service, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), service)
}

func (r *Reconciler) addBackendDisruptionBudget(cluster *v1beta1.Cluster) error {
	log.Info("Creating pod disruption budget", "Name", cluster.Name, "Namespace", cluster.Namespace)
	budget := k8s.NewClusterDisruptionBudget(cluster)
	if err := controllerutil.SetControllerReference(cluster, budget, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), budget)
}

func (r *Reconciler) reconcileStatus(cluster *v1beta1.Cluster) error {
	set := &appsv1.StatefulSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetClusterStatefulSetName(cluster), Namespace: cluster.Namespace}, set)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Update the backend replicas status
	if set.Status.ReadyReplicas != cluster.Status.Backend.ReadyReplicas {
		log.Info("Updating cluster status", "Name", cluster.Name, "Namespace", cluster.Namespace)
		cluster.Status.Backend.ReadyReplicas = set.Status.ReadyReplicas
		err = r.client.Status().Update(context.TODO(), cluster)
		if err != nil {
			return err
		}
		return nil
	}

	// Update the proxy status
	if cluster.Spec.Proxy != nil {
		deployment := &appsv1.Deployment{}
		err := r.client.Get(context.TODO(), types.NamespacedName{Name: k8s.GetProxyDeploymentName(cluster), Namespace: cluster.Namespace}, deployment)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}

		if cluster.Status.Proxy == nil {
			cluster.Status.Proxy = &v1beta1.ProxyStatus{
				Ready: false,
			}
			err = r.client.Update(context.TODO(), cluster)
			if err != nil {
				return err
			}
			return nil
		} else if deployment.Status.ReadyReplicas > 0 != cluster.Status.Proxy.Ready {
			cluster.Status.Proxy.Ready = deployment.Status.ReadyReplicas > 0
			log.Info("Updating cluster status", "Name", cluster.Name, "Namespace", cluster.Namespace)
			err = r.client.Update(context.TODO(), cluster)
			if err != nil {
				return err
			}
			return nil
		}
	}

	// If the backend replicas status is ready and the proxy is ready, update partition statuses
	if int(cluster.Status.ReadyPartitions) < len(cluster.Spec.Partitions) &&
		cluster.Status.Backend.ReadyReplicas == cluster.Spec.Backend.Replicas &&
		(cluster.Spec.Proxy == nil || cluster.Status.Proxy.Ready) {
		for _, id := range cluster.Spec.Partitions {
			partition := &v1beta1.Partition{}
			err := r.client.Get(context.TODO(), k8s.GetPartitionNamespacedName(cluster, id), partition)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
			if !partition.Status.Ready {
				partition.Status.Ready = true
				log.Info("Updating partition status", "Name", partition.Name, "Namespace", partition.Namespace)
				err = r.client.Update(context.TODO(), partition)
				if err != nil {
					return err
				}
			}
		}

		// If we've made it this far, all partitions are ready. Update the cluster status
		cluster.Status.ReadyPartitions = int32(len(cluster.Spec.Partitions))
		log.Info("Updating cluster status", "Name", cluster.Name, "Namespace", cluster.Namespace)
		return r.client.Update(context.TODO(), cluster)
	}
	return nil
}
