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
	"errors"
	"fmt"
	driverapi "github.com/atomix/api/go/atomix/management/driver"
	protocolapi "github.com/atomix/api/go/atomix/protocol"
	"github.com/atomix/go-framework/pkg/atomix/logging"
	"github.com/atomix/kubernetes-controller/pkg/apis/core/v2beta1"
	storagev2beta1 "github.com/atomix/kubernetes-controller/pkg/controller/storage/v2beta1"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
)

const (
	baseProtocolPort = 55680
)

func addStoreController(mgr manager.Manager) error {
	r := &StoreReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("protocol-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Pods
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Stores and enqueue all pods
	err = c.Watch(&source.Kind{Type: &v2beta1.Store{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return err
	}
	return nil
}

// StoreReconciler is a Reconciler for Store resources
type StoreReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

func (r *StoreReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Pod '%s'", request.NamespacedName)
	pod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), request.NamespacedName, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	injected := pod.Annotations[brokerInjectStatusAnnotation]
	if injected != injectedStatus {
		return reconcile.Result{}, nil
	}

	ok, err := r.prepareStatus(pod)
	if err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}

	ok, err = r.reconcileStores(pod)
	if err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}

	ok, err = r.updateStatus(pod)
	if err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *StoreReconciler) prepareStatus(pod *corev1.Pod) (bool, error) {
	log := newPodLogger(*pod)
	for _, condition := range pod.Status.Conditions {
		if condition.Type == brokerReadyCondition {
			return false, nil
		}
	}

	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               brokerReadyCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
	})

	log.Info("Initializing pod readiness condition")
	err := r.client.Status().Update(context.TODO(), pod)
	if err != nil {
		log.Error(err, "Initializing pod readiness condition")
		return false, err
	}
	return true, nil
}

func (r *StoreReconciler) updateStatus(pod *corev1.Pod) (bool, error) {
	for _, condition := range pod.Status.Conditions {
		if isProtocolReadyCondition(condition.Type) && condition.Status != corev1.ConditionTrue {
			return false, nil
		}
		if isPrimitiveReadyCondition(condition.Type) && condition.Status != corev1.ConditionTrue {
			return false, nil
		}
	}

	for i, condition := range pod.Status.Conditions {
		if condition.Type == brokerReadyCondition && condition.Status != corev1.ConditionTrue {
			log.Info("Updating pod readiness condition")
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.Now()
			pod.Status.Conditions[i] = condition
			err := r.client.Status().Update(context.TODO(), pod)
			if err != nil {
				log.Error(err, "Updating pod readiness condition")
				return false, err
			}
			return true, nil
		}
	}
	return true, nil
}

func (r *StoreReconciler) reconcileStores(pod *corev1.Pod) (bool, error) {
	protocols := &v2beta1.StoreList{}
	err := r.client.List(context.TODO(), protocols, &client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		log.Errorf("Listing Store resources failed: %s", err)
		return false, err
	}
	for _, protocol := range protocols.Items {
		if ok, err := r.reconcileStore(pod, protocol, newStoreLogger(*pod, protocol)); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

func (r *StoreReconciler) reconcileStore(pod *corev1.Pod, protocol v2beta1.Store, log logging.Logger) (bool, error) {
	if ok, err := r.prepareAgent(pod, protocol, log); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	if ok, err := r.updateAgent(pod, protocol, log); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	return false, nil
}

func (r *StoreReconciler) prepareAgent(pod *corev1.Pod, store v2beta1.Store, log logging.Logger) (bool, error) {
	conditions := NewProtocolConditions(store.Name, pod.Status.Conditions)

	// If the agent status is Unknown, add the status to the pod
	switch conditions.GetReady() {
	case corev1.ConditionUnknown:
		log.Info("Initializing agent ready condition")
		pod.Status.Conditions = conditions.SetReady(corev1.ConditionFalse)
		err := r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Initializing agent ready condition")
			return false, err
		}
		return true, nil
	case corev1.ConditionFalse:
		log.Info("Connecting to protocol driver")
		conn, err := r.connectDriver(pod, store)
		if err != nil {
			log.Error(err, "Connecting to protocol driver")
			return false, err
		}
		defer conn.Close()
		client := driverapi.NewDriverClient(conn)

		log.Info("Starting protocol agent")
		request := &driverapi.StartAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: store.Namespace,
				Name:      store.Name,
			},
			Address: driverapi.AgentAddress{
				Port: int32(conditions.GetPort()),
			},
			Config: driverapi.AgentConfig{
				Protocol: r.getProtocolConfig(store),
			},
		}
		_, err = client.StartAgent(context.TODO(), request)
		if err != nil {
			log.Error(err, "Starting protocol agent")
			return false, err
		}

		log.Info("Updating agent ready condition")
		pod.Status.Conditions = conditions.SetReady(corev1.ConditionTrue)
		err = r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Updating agent ready condition")
			return false, err
		}
		return true, nil
	default:
		return false, nil
	}
}

func (r *StoreReconciler) updateAgent(pod *corev1.Pod, protocol v2beta1.Store, log logging.Logger) (bool, error) {
	conditions := NewProtocolConditions(protocol.Name, pod.Status.Conditions)

	// If the generation status is Unknown, add the status to the pod
	switch conditions.GetGeneration(protocol.Generation) {
	case corev1.ConditionUnknown:
		log.Info("Initializing configuration change condition")
		pod.Status.Conditions = conditions.SetGeneration(protocol.Generation, corev1.ConditionFalse)
		err := r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Initializing configuration change condition")
			return false, err
		}
		return true, nil
	case corev1.ConditionFalse:
		log.Info("Connecting to protocol driver")
		conn, err := r.connectDriver(pod, protocol)
		if err != nil {
			log.Error(err, "Connecting to protocol driver")
			return false, err
		}
		defer conn.Close()
		client := driverapi.NewDriverClient(conn)

		log.Info("Reconfiguring protocol agent")
		request := &driverapi.ConfigureAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: protocol.Namespace,
				Name:      protocol.Name,
			},
			Config: driverapi.AgentConfig{
				Protocol: r.getProtocolConfig(protocol),
			},
		}
		_, err = client.ConfigureAgent(context.TODO(), request)
		if err != nil {
			log.Error(err, "Reconfiguring protocol agent")
			return false, err
		}

		log.Info("Updating configuration change condition")
		pod.Status.Conditions = conditions.SetGeneration(protocol.Generation, corev1.ConditionTrue)
		err = r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Updating configuration change condition")
			return false, err
		}
		return true, nil
	default:
		return false, nil
	}
}

func (r *StoreReconciler) connectDriver(pod *corev1.Pod, protocol v2beta1.Store) (*grpc.ClientConn, error) {
	annotations := storagev2beta1.NewAnnotations(protocol.Status.Protocol.Driver.Name, pod.Annotations)
	port, err := annotations.GetDriverPort()
	if err != nil {
		return nil, err
	} else if port == nil {
		return nil, errors.New("no driver port found")
	}

	address := fmt.Sprintf("%s:%d", pod.Status.PodIP, *port)
	return grpc.Dial(address, grpc.WithInsecure())
}

func (r *StoreReconciler) getProtocolConfig(protocol v2beta1.Store) protocolapi.ProtocolConfig {
	replicas := make([]protocolapi.ProtocolReplica, len(protocol.Status.Protocol.Replicas))
	for i, replica := range protocol.Status.Protocol.Replicas {
		var host string
		if replica.Host != nil {
			host = *replica.Host
		}
		var port int32
		if replica.Port != nil {
			port = *replica.Port
		}
		replicas[i] = protocolapi.ProtocolReplica{
			ID:         replica.ID,
			NodeID:     replica.NodeID,
			Host:       host,
			APIPort:    port,
			ExtraPorts: replica.ExtraPorts,
		}
	}

	partitions := make([]protocolapi.ProtocolPartition, len(protocol.Status.Protocol.Partitions))
	for i, partition := range protocol.Status.Protocol.Partitions {
		partitions[i] = protocolapi.ProtocolPartition{
			PartitionID: partition.ID,
			Replicas:    partition.Replicas,
		}
	}

	return protocolapi.ProtocolConfig{
		Replicas:   replicas,
		Partitions: partitions,
	}
}

var _ reconcile.Reconciler = &StoreReconciler{}

func isProtocolReadyCondition(condition corev1.PodConditionType) bool {
	return strings.HasSuffix(string(condition), ".protocol.atomix.io/ready")
}

// NewProtocolConditions returns new conditions helper for the given driver with the given conditions
func NewProtocolConditions(protocol string, conditions []corev1.PodCondition) ProtocolConditions {
	return ProtocolConditions{
		Protocol:   protocol,
		Conditions: conditions,
	}
}

// ProtocolConditions provides utility functions for driver conditions
type ProtocolConditions struct {
	Protocol   string
	Conditions []corev1.PodCondition
}

func (d ProtocolConditions) getReadyType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("protocols.atomix.io/%s", d.Protocol))
}

func (d ProtocolConditions) getGenerationType(generation int64) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.protocols.atomix.io/%d", d.Protocol, generation))
}

func (d ProtocolConditions) getConditionStatus(conditionType corev1.PodConditionType) corev1.ConditionStatus {
	for _, condition := range d.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionUnknown
}

func (d ProtocolConditions) setCondition(condition corev1.PodCondition) []corev1.PodCondition {
	for i, c := range d.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				d.Conditions[i] = condition
			}
			return d.Conditions
		}
	}
	d.Conditions = append(d.Conditions, condition)
	return d.Conditions
}

func (d ProtocolConditions) GetReady() corev1.ConditionStatus {
	return d.getConditionStatus(d.getReadyType())
}

func (d ProtocolConditions) SetReady(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getReadyType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d ProtocolConditions) GetGeneration(generation int64) corev1.ConditionStatus {
	return d.getConditionStatus(d.getGenerationType(generation))
}

func (d ProtocolConditions) SetGeneration(generation int64, status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getGenerationType(generation),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d ProtocolConditions) GetPort() int {
	for i, c := range d.Conditions {
		if c.Type == d.getReadyType() {
			return baseProtocolPort + i
		}
	}
	return baseProtocolPort + len(d.Conditions)
}

func newStoreLogger(pod corev1.Pod, store v2beta1.Store) logging.Logger {
	fields := []logging.Field{
		logging.String("pod", types.NamespacedName{pod.Namespace, pod.Name}.String()),
		logging.String("store", types.NamespacedName{store.Namespace, store.Name}.String()),
	}
	return log.WithFields(fields...)
}
