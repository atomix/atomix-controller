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
	"fmt"
	driverapi "github.com/atomix/api/go/atomix/management/driver"
	protocolapi "github.com/atomix/api/go/atomix/protocol"
	"github.com/atomix/kubernetes-controller/pkg/apis/core/v2beta1"
	storagev2beta1 "github.com/atomix/kubernetes-controller/pkg/controller/storage/v2beta1"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	protocolConditionFormat = "%s.protocol.atomix.io/%s"
	protocolReadyCondition  = "ready"
)

const (
	baseProtocolPort = 55680
)

func addProtocolController(mgr manager.Manager) error {
	r := &ProtocolReconciler{
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

	// Watch for changes to Protocols and enqueue all pods
	err = c.Watch(&source.Kind{Type: &v2beta1.Protocol{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return err
	}
	return nil
}

// ProtocolReconciler is a Reconciler for Protocol resources
type ProtocolReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

func (r *ProtocolReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

	err = r.prepareStatus(pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.reconcileProtocols(pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.updateStatus(pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ProtocolReconciler) prepareStatus(pod *corev1.Pod) error {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == brokerReadyCondition {
			return nil
		}
	}

	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               brokerReadyCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
	})
	return r.client.Status().Update(context.TODO(), pod)
}

func (r *ProtocolReconciler) updateStatus(pod *corev1.Pod) error {
	for _, condition := range pod.Status.Conditions {
		if isProtocolReadyCondition(condition.Type) && condition.Status != corev1.ConditionTrue {
			return nil
		}
		if isPrimitiveReadyCondition(condition.Type) && condition.Status != corev1.ConditionTrue {
			return nil
		}
	}

	for i, condition := range pod.Status.Conditions {
		if condition.Type == brokerReadyCondition && condition.Status != corev1.ConditionTrue {
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.Now()
			pod.Status.Conditions[i] = condition
			return r.client.Status().Update(context.TODO(), pod)
		}
	}
	return nil
}

func (r *ProtocolReconciler) reconcileProtocols(pod *corev1.Pod) error {
	protocols := &v2beta1.ProtocolList{}
	err := r.client.List(context.TODO(), protocols, &client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		return err
	}
	for _, protocol := range protocols.Items {
		err := r.reconcileProtocol(pod, protocol)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ProtocolReconciler) reconcileProtocol(pod *corev1.Pod, protocol v2beta1.Protocol) error {
	conditions := NewProtocolConditions(protocol.Name, pod.Status.Conditions)

	// If the agent status is Unknown, add the status to the pod
	if conditions.GetReady() == corev1.ConditionUnknown {
		pod.Status.Conditions = conditions.SetReady(corev1.ConditionFalse)
		return r.client.Status().Update(context.TODO(), pod)
	}

	replicas := make([]protocolapi.ProtocolReplica, len(protocol.Spec.Replicas))
	for i, replica := range protocol.Spec.Replicas {
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

	partitions := make([]protocolapi.ProtocolPartition, len(protocol.Spec.Partitions))
	for i, partition := range protocol.Spec.Partitions {
		partitions[i] = protocolapi.ProtocolPartition{
			PartitionID: partition.ID,
			Replicas:    partition.Replicas,
		}
	}

	config := protocolapi.ProtocolConfig{
		Replicas:   replicas,
		Partitions: partitions,
	}

	annotations := storagev2beta1.NewAnnotations(protocol.Spec.Driver.Name, pod.Annotations)
	port, err := annotations.GetDriverPort()
	if err != nil {
		return err
	}

	address := fmt.Sprintf("%s:%d", pod.Status.PodIP, port)
	log.Info("Dial %s", address)
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Warn("Dial %s failed", address, err)
		return err
	}
	defer conn.Close()
	client := driverapi.NewDriverClient(conn)

	// If the agent status is False, start the agent and update the pod status
	if conditions.GetReady() == corev1.ConditionFalse {
		request := &driverapi.StartAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: protocol.Namespace,
				Name:      protocol.Name,
			},
			Address: driverapi.AgentAddress{
				Port: int32(conditions.GetPort()),
			},
			Config: driverapi.AgentConfig{
				Protocol: config,
			},
		}
		_, err := client.StartAgent(context.TODO(), request)
		if err != nil {
			return err
		}
		pod.Status.Conditions = conditions.SetReady(corev1.ConditionTrue)
		err = r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			return err
		}
	} else {
		request := &driverapi.ConfigureAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: protocol.Namespace,
				Name:      protocol.Name,
			},
			Config: driverapi.AgentConfig{
				Protocol: config,
			},
		}
		_, err := client.ConfigureAgent(context.TODO(), request)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ reconcile.Reconciler = &ProtocolReconciler{}

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

func (d ProtocolConditions) getConditionType(name string) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf(protocolConditionFormat, d.Protocol, name))
}

func (d ProtocolConditions) getConditionStatus(name string) corev1.ConditionStatus {
	for _, condition := range d.Conditions {
		if condition.Type == d.getConditionType(name) {
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
	return d.getConditionStatus(protocolReadyCondition)
}

func (d ProtocolConditions) SetReady(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getConditionType(protocolReadyCondition),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d ProtocolConditions) GetPort() int {
	for i, c := range d.Conditions {
		if c.Type == d.getConditionType(protocolReadyCondition) {
			return baseProtocolPort + i
		}
	}
	return baseProtocolPort + len(d.Conditions)
}
