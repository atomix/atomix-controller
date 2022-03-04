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
	driverapi "github.com/atomix/atomix-api/go/atomix/management/driver"
	protocolapi "github.com/atomix/atomix-api/go/atomix/protocol"
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	sidecarv2beta1 "github.com/atomix/atomix-controller/pkg/apis/sidecar/v2beta1"
	"github.com/atomix/atomix-controller/pkg/controller/util/k8s"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"time"
)

const agentFinalizer = "agent"

func addAgentController(mgr manager.Manager) error {
	// Create a new controller
	options := controller.Options{
		Reconciler: &AgentReconciler{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
			config: mgr.GetConfig(),
			events: mgr.GetEventRecorderFor("atomix"),
		},
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Millisecond*10, time.Second*5),
	}
	controller, err := controller.New("agent-controller", mgr, options)
	if err != nil {
		return err
	}

	// Watch for changes to ProtocolAgents
	err = controller.Watch(&source.Kind{Type: &sidecarv2beta1.Agent{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Stores
	err = controller.Watch(&source.Kind{Type: &corev2beta1.Store{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newStoreAgentMapper(mgr),
	})
	if err != nil {
		return err
	}

	// Watch for changes to Pods
	err = controller.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodAgentMapper(mgr),
	})
	if err != nil {
		return err
	}
	return nil
}

// AgentReconciler is a Reconciler for Agent resources
type AgentReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	events record.EventRecorder
}

// Reconcile reconciles Agent resources
func (r *AgentReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Agent '%s'", request.NamespacedName)
	agent := &sidecarv2beta1.Agent{}
	err := r.client.Get(context.TODO(), request.NamespacedName, agent)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if agent.DeletionTimestamp != nil {
		return r.reconcileDelete(agent)
	}
	return r.reconcileCreate(agent)
}

func (r *AgentReconciler) reconcileCreate(agent *sidecarv2beta1.Agent) (reconcile.Result, error) {
	agentName := types.NamespacedName{
		Namespace: agent.Namespace,
		Name:      agent.Name,
	}
	if !k8s.HasFinalizer(agent.Finalizers, agentFinalizer) {
		log.Infof("Initializing Agent %s", agentName)
		return reconcile.Result{}, r.addFinalizer(agent)
	}

	podName := types.NamespacedName{
		Namespace: agent.Spec.Pod.Namespace,
		Name:      agent.Spec.Pod.Name,
	}
	pod, err := r.getPod(agent)
	if err != nil {
		return reconcile.Result{}, err
	} else if pod == nil {
		log.Infof("Pod %s not found. Deleting Agent %s", podName, agentName)
		if err := r.client.Delete(context.TODO(), agent); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	storeName := types.NamespacedName{
		Namespace: agent.Spec.Store.Namespace,
		Name:      agent.Spec.Store.Name,
	}
	store, err := r.getStore(agent)
	if err != nil {
		return reconcile.Result{}, err
	} else if store == nil || store.DeletionTimestamp != nil {
		log.Infof("Store %s not found. Deleting Agent %s", storeName, agentName)
		if err := r.client.Delete(context.TODO(), agent); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	log.Info("Connecting to driver")
	driverContainer, driverPort, err := r.getDriverContainerAndPort(pod, store)
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}

	// Get the container status for the driver container.
	var containerStatus *corev1.ContainerStatus
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == driverContainer {
			containerStatus = &cs
			break
		}
	}

	// If no status is found for the driver container, skip reconciliation to wait for the container
	// status to be populated by Kubernetes.
	if containerStatus == nil {
		log.Warnf("Container status not found for driver container '%s'", driverContainer)
		return reconcile.Result{}, nil
	}

	// If the agent's container ID is different from the running container ID, update the agent's
	// container ID and reset the agent status to not ready.
	if agent.Status.ContainerID != containerStatus.ContainerID {
		log.Infof("Updating agent %s status for container '%s'", agentName, containerStatus.ContainerID)
		agent.Status.ContainerID = containerStatus.ContainerID
		agent.Status.Ready = false
		agent.Status.Revision = 0
		if err := r.client.Status().Update(context.TODO(), agent); err != nil {
			log.Error(err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if !store.Status.Ready || (agent.Status.Ready && agent.Status.Revision == store.Status.Protocol.Revision) {
		return reconcile.Result{}, nil
	}

	driverConn, err := grpc.Dial(fmt.Sprintf("%s:%d", pod.Status.PodIP, driverPort), grpc.WithInsecure())
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	driverClient := driverapi.NewDriverClient(driverConn)

	if !agent.Status.Ready {
		log.Info("Starting protocol agent")
		r.events.Eventf(pod, "Normal", "StartingAgent", "Starting agent for store '%s'", agent.Spec.Store.Name)
		startAgentRequest := &driverapi.StartAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: store.Namespace,
				Name:      store.Name,
			},
			Address: driverapi.AgentAddress{
				Port: agent.Spec.Port,
			},
			Config: driverapi.AgentConfig{
				Protocol: r.getProtocolConfig(store),
			},
		}
		_, err = driverClient.StartAgent(context.TODO(), startAgentRequest)
		if err != nil && status.Code(err) != codes.AlreadyExists {
			log.Error(err, "Starting protocol agent")
			r.events.Eventf(pod, "Warning", "StartingAgentFailed", "Failed starting agent for store '%s': %s", agent.Spec.Store.Name, err)
			return reconcile.Result{}, err
		}
		r.events.Eventf(pod, "Normal", "StartedAgent", "Started agent for store '%s'", agent.Spec.Store.Name)
	} else {
		log.Info("Configuring protocol agent")
		r.events.Eventf(pod, "Normal", "ReconfiguringAgent", "Reconfiguring agent for store '%s'", agent.Spec.Store.Name)
		configureAgentRequest := &driverapi.ConfigureAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: store.Namespace,
				Name:      store.Name,
			},
			Config: driverapi.AgentConfig{
				Protocol: r.getProtocolConfig(store),
			},
		}
		_, err = driverClient.ConfigureAgent(context.TODO(), configureAgentRequest)
		if err != nil {
			log.Error(err, "Configuring protocol agent")
			r.events.Eventf(pod, "Warning", "ReconfiguringAgentFailed", "Failed reconfiguring agent for store '%s': %s", agent.Spec.Store.Name, err)
			return reconcile.Result{}, err
		}
		r.events.Eventf(pod, "Normal", "ReconfiguredAgent", "Reconfigured agent for store '%s'", agent.Spec.Store.Name)
	}

	agent.Status.Ready = true
	agent.Status.Revision = store.Status.Protocol.Revision
	if err := r.client.Status().Update(context.TODO(), agent); err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *AgentReconciler) getProtocolConfig(store *corev2beta1.Store) protocolapi.ProtocolConfig {
	replicas := make([]protocolapi.ProtocolReplica, len(store.Status.Protocol.Replicas))
	for i, replica := range store.Status.Protocol.Replicas {
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

	partitions := make([]protocolapi.ProtocolPartition, len(store.Status.Protocol.Partitions))
	for i, partition := range store.Status.Protocol.Partitions {
		var host string
		if partition.Host != nil {
			host = *partition.Host
		}
		var port int32
		if partition.Port != nil {
			port = *partition.Port
		}
		partitions[i] = protocolapi.ProtocolPartition{
			PartitionID:  partition.ID,
			Replicas:     partition.Replicas,
			ReadReplicas: partition.ReadReplicas,
			Host:         host,
			APIPort:      port,
		}
	}

	return protocolapi.ProtocolConfig{
		Replicas:   replicas,
		Partitions: partitions,
	}
}

func (r *AgentReconciler) reconcileDelete(agent *sidecarv2beta1.Agent) (reconcile.Result, error) {
	if !k8s.HasFinalizer(agent.Finalizers, agentFinalizer) {
		return reconcile.Result{}, nil
	}

	log.Infof("Finalizing Agent %s", types.NamespacedName{Namespace: agent.Namespace, Name: agent.Name})
	pod, err := r.getPod(agent)
	if err != nil {
		return reconcile.Result{}, err
	} else if pod == nil {
		return reconcile.Result{}, r.removeFinalizer(agent)
	}

	store, err := r.getStore(agent)
	if err != nil {
		return reconcile.Result{}, err
	} else if store == nil {
		return reconcile.Result{}, r.removeFinalizer(agent)
	}

	log.Info("Connecting to driver")
	_, driverPort, err := r.getDriverContainerAndPort(pod, store)
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}

	driverConn, err := grpc.Dial(fmt.Sprintf("%s:%d", pod.Status.PodIP, driverPort), grpc.WithInsecure())
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	driverClient := driverapi.NewDriverClient(driverConn)

	log.Info("Stopping protocol agent")
	r.events.Eventf(pod, "Normal", "StoppingAgent", "Stopping agent for store '%s'", agent.Spec.Store.Name)
	stopAgentRequest := &driverapi.StopAgentRequest{
		AgentID: driverapi.AgentId{
			Namespace: store.Namespace,
			Name:      store.Name,
		},
	}
	_, err = driverClient.StopAgent(context.TODO(), stopAgentRequest)
	if err != nil && status.Code(err) != codes.NotFound {
		log.Error(err, "Stopping protocol agent")
		r.events.Eventf(pod, "Warning", "StoppingAgentFailed", "Failed stopping agent for store '%s': %s", agent.Spec.Store.Name, err)
		return reconcile.Result{}, err
	}
	r.events.Eventf(pod, "Normal", "StoppedAgent", "Stopped agent for store '%s'", agent.Spec.Store.Name)
	return reconcile.Result{}, r.removeFinalizer(agent)
}

func (r *AgentReconciler) addFinalizer(agent *sidecarv2beta1.Agent) error {
	agent.Finalizers = k8s.AddFinalizer(agent.Finalizers, agentFinalizer)
	if err := r.client.Update(context.TODO(), agent); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (r *AgentReconciler) removeFinalizer(agent *sidecarv2beta1.Agent) error {
	agent.Finalizers = k8s.RemoveFinalizer(agent.Finalizers, agentFinalizer)
	if err := r.client.Update(context.TODO(), agent); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (r *AgentReconciler) getDriverContainerAndPort(pod *corev1.Pod, store *corev2beta1.Store) (string, int, error) {
	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, store.Spec.Protocol.Raw)
	if err != nil {
		log.Error(err)
		return "", 0, err
	}

	gvc := object.GetObjectKind().GroupVersionKind()

	plugins := &corev2beta1.StoragePluginList{}
	err = r.client.List(context.TODO(), plugins)
	if err != nil {
		log.Error(err)
		return "", 0, err
	}

	for _, plugin := range plugins.Items {
		if plugin.Spec.Protocol.Group == gvc.Group && plugin.Spec.Protocol.Kind == gvc.Kind {
			for _, driver := range plugin.Spec.Drivers {
				if driver.Version == gvc.Version {
					containerAnnotation := getDriverContainerAnnotation(plugin.Name, driver.Version)
					containerName, ok := pod.Annotations[containerAnnotation]
					if !ok {
						return "", 0, fmt.Errorf("could not find container for %s", gvc)
					}
					portAnnotation := getDriverPortAnnotation(plugin.Name, driver.Version)
					portValue, ok := pod.Annotations[portAnnotation]
					if !ok {
						return "", 0, fmt.Errorf("could not find port for %s", gvc)
					}
					port, err := strconv.Atoi(portValue)
					if err != nil {
						return "", 0, err
					}
					return containerName, port, nil
				}
			}
			return "", 0, fmt.Errorf("could not find plugin for %s", gvc)
		}
		if plugin.Spec.DeprecatedGroup == gvc.Group && plugin.Spec.DeprecatedKind == gvc.Kind {
			for _, version := range plugin.Spec.DeprecatedVersions {
				if version.Name == gvc.Version {
					containerAnnotation := getDriverContainerAnnotation(plugin.Name, version.Name)
					containerName, ok := pod.Annotations[containerAnnotation]
					if !ok {
						return "", 0, fmt.Errorf("could not find container for %s", gvc)
					}
					portAnnotation := getDriverPortAnnotation(plugin.Name, version.Name)
					portValue, ok := pod.Annotations[portAnnotation]
					if !ok {
						return "", 0, fmt.Errorf("could not find port for %s", gvc)
					}
					port, err := strconv.Atoi(portValue)
					if err != nil {
						return "", 0, err
					}
					return containerName, port, nil
				}
			}
			return "", 0, fmt.Errorf("could not find plugin for %s", gvc)
		}
	}
	return "", 0, fmt.Errorf("could not find plugin for %s", gvc)
}

func (r *AgentReconciler) getPod(agent *sidecarv2beta1.Agent) (*corev1.Pod, error) {
	podName := types.NamespacedName{
		Namespace: agent.Spec.Pod.Namespace,
		Name:      agent.Spec.Pod.Name,
	}
	pod := &corev1.Pod{}
	if err := r.client.Get(context.TODO(), podName, pod); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return nil, err
		}
		return nil, nil
	}
	return pod, nil
}

func (r *AgentReconciler) getStore(agent *sidecarv2beta1.Agent) (*corev2beta1.Store, error) {
	storeName := types.NamespacedName{
		Namespace: agent.Spec.Store.Namespace,
		Name:      agent.Spec.Store.Name,
	}
	store := &corev2beta1.Store{}
	if err := r.client.Get(context.TODO(), storeName, store); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return nil, err
		}
		return nil, nil
	}
	return store, nil
}

var _ reconcile.Reconciler = &AgentReconciler{}

func newStoreAgentMapper(mgr manager.Manager) handler.Mapper {
	return &storeAgentMapper{
		client: mgr.GetClient(),
	}
}

type storeAgentMapper struct {
	client client.Client
}

func (m *storeAgentMapper) Map(object handler.MapObject) []reconcile.Request {
	store := object.Object.(*corev2beta1.Store)
	agents := &sidecarv2beta1.AgentList{}
	options := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"store": string(store.UID),
		}),
	}
	if err := m.client.List(context.TODO(), agents, options); err != nil {
		log.Error(err)
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, 0, len(agents.Items))
	for _, agent := range agents.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: agent.Namespace,
				Name:      agent.Name,
			},
		})
	}
	return requests
}

var _ handler.Mapper = &storeAgentMapper{}

func newPodAgentMapper(mgr manager.Manager) handler.Mapper {
	return &podAgentMapper{
		client: mgr.GetClient(),
	}
}

type podAgentMapper struct {
	client client.Client
}

func (m *podAgentMapper) Map(object handler.MapObject) []reconcile.Request {
	pod := object.Object.(*corev1.Pod)
	agents := &sidecarv2beta1.AgentList{}
	options := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"pod": string(pod.UID),
		}),
	}
	if err := m.client.List(context.TODO(), agents, options); err != nil {
		log.Error(err)
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, 0, len(agents.Items))
	for _, agent := range agents.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: agent.Namespace,
				Name:      agent.Name,
			},
		})
	}
	return requests
}

var _ handler.Mapper = &podAgentMapper{}
