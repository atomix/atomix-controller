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
	brokerapi "github.com/atomix/atomix-api/go/atomix/management/broker"
	driverapi "github.com/atomix/atomix-api/go/atomix/management/driver"
	primitiveapi "github.com/atomix/atomix-api/go/atomix/primitive"
	protocolapi "github.com/atomix/atomix-api/go/atomix/protocol"
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
)

const (
	verbRead  = "read"
	verbWrite = "write"
)

func addPrimitiveController(mgr manager.Manager) error {
	r := &ProxyReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("primitive-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Pods
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Primitives and enqueue all pods
	err = c.Watch(&source.Kind{Type: &v2beta1.Primitive{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr, func(object handler.MapObject) string {
			return object.Meta.GetNamespace()
		}),
	})
	if err != nil {
		return err
	}

	// Watch for changes to Stores and enqueue all pods
	err = c.Watch(&source.Kind{Type: &v2beta1.Store{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr, func(object handler.MapObject) string {
			return ""
		}),
	})
	if err != nil {
		return err
	}
	return nil
}

// ProxyReconciler is a Reconciler for Primitive resources
type ProxyReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

func (r *ProxyReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

	ok, err = r.reconcilePrimitives(pod)
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

func (r *ProxyReconciler) prepareStatus(pod *corev1.Pod) (bool, error) {
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

func (r *ProxyReconciler) updateStatus(pod *corev1.Pod) (bool, error) {
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

func (r *ProxyReconciler) reconcileStore(pod *corev1.Pod, store v2beta1.Store) (bool, error) {
	log := newStoreLogger(*pod, store)
	if !store.Status.Ready {
		log.Warn("Store is not ready")
		return false, nil
	}
	if store.Status.Protocol == nil {
		return false, errors.New("store is ready, but no protocol configuration provided by plugin")
	}
	if ok, err := r.prepareAgent(pod, store, log); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	if ok, err := r.updateAgent(pod, store, log); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	return false, nil
}

func (r *ProxyReconciler) prepareAgent(pod *corev1.Pod, store v2beta1.Store, log logging.Logger) (bool, error) {
	name := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Name,
	}
	conditions := NewProtocolConditions(name, pod.Status.Conditions)

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
		port, err := conditions.GetPort()
		if err != nil {
			log.Error(err, "Connecting to protocol driver")
			return false, err
		}
		request := &driverapi.StartAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: store.Namespace,
				Name:      store.Name,
			},
			Address: driverapi.AgentAddress{
				Port: int32(port),
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

func (r *ProxyReconciler) updateAgent(pod *corev1.Pod, store v2beta1.Store, log logging.Logger) (bool, error) {
	name := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Name,
	}
	conditions := NewProtocolConditions(name, pod.Status.Conditions)

	// If the generation status is Unknown, add the status to the pod
	switch conditions.GetRevision(store.Status.Protocol.Revision) {
	case corev1.ConditionUnknown:
		log.Info("Initializing configuration change condition")
		pod.Status.Conditions = conditions.SetRevision(store.Status.Protocol.Revision, corev1.ConditionFalse)
		err := r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Initializing configuration change condition")
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

		log.Info("Reconfiguring protocol agent")
		request := &driverapi.ConfigureAgentRequest{
			AgentID: driverapi.AgentId{
				Namespace: store.Namespace,
				Name:      store.Name,
			},
			Config: driverapi.AgentConfig{
				Protocol: r.getProtocolConfig(store),
			},
		}
		_, err = client.ConfigureAgent(context.TODO(), request)
		if err != nil {
			log.Error(err, "Reconfiguring protocol agent")
			return false, err
		}

		log.Info("Updating configuration change condition")
		pod.Status.Conditions = conditions.SetRevision(store.Status.Protocol.Revision, corev1.ConditionTrue)
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

func (r *ProxyReconciler) getDriverPort(pod *corev1.Pod, store v2beta1.Store) (int, error) {
	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, store.Spec.Protocol.Raw)
	if err != nil {
		return 0, err
	}
	protocol := object.(*unstructured.Unstructured)
	gvc := protocol.GroupVersionKind()

	plugins := &v2beta1.StoragePluginList{}
	err = r.client.List(context.TODO(), plugins)
	if err != nil {
		return 0, err
	}

	for _, plugin := range plugins.Items {
		if plugin.Spec.Group == gvc.Group && plugin.Spec.Kind == gvc.Kind {
			for _, version := range plugin.Spec.Versions {
				if version.Name == gvc.Version {
					portAnnotation := fmt.Sprintf("%s.%s/port", version.Name, plugin.Name)
					portValue := pod.Annotations[portAnnotation]
					if portValue == "" {
						return 0, errors.New("port annotation not found")
					}
					return strconv.Atoi(portValue)
				}
			}
			return 0, errors.New("protocol version not found")
		}
	}
	return 0, errors.New("protocol plugin not found")
}

func (r *ProxyReconciler) connectDriver(pod *corev1.Pod, store v2beta1.Store) (*grpc.ClientConn, error) {
	port, err := r.getDriverPort(pod, store)
	if err != nil {
		return nil, err
	}
	address := fmt.Sprintf("%s:%d", pod.Status.PodIP, port)
	return grpc.Dial(address, grpc.WithInsecure())
}

func (r *ProxyReconciler) getProtocolConfig(protocol v2beta1.Store) protocolapi.ProtocolConfig {
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

func (r *ProxyReconciler) reconcilePrimitives(pod *corev1.Pod) (bool, error) {
	primitives := &v2beta1.PrimitiveList{}
	err := r.client.List(context.TODO(), primitives, &client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		return false, err
	}
	for _, primitive := range primitives.Items {
		if ok, err := r.reconcilePrimitive(pod, primitive); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

func (r *ProxyReconciler) reconcilePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive) (bool, error) {
	log := newPrimitiveLogger(*pod, primitive)
	store := v2beta1.Store{}
	name := types.NamespacedName{
		Namespace: primitive.Spec.Store.Namespace,
		Name:      primitive.Spec.Store.Name,
	}
	if name.Namespace == "" {
		name.Namespace = primitive.Namespace
	}
	if err := r.client.Get(context.TODO(), name, &store); err != nil {
		if k8serrors.IsNotFound(err) {
			log.Warn("Store not found")
			return false, nil
		}
		return false, err
	}

	if ok, err := r.reconcileStore(pod, store); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	conditions := NewProtocolConditions(name, pod.Status.Conditions)
	if conditions.GetReady() != corev1.ConditionTrue {
		log.Warn("Protocol is not ready")
		return false, nil
	}

	if primitive.DeletionTimestamp == nil {
		log.Info("Updating primitive proxy")
		if ok, err := r.updatePrimitive(pod, primitive, log); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	} else {
		log.Info("Deleting primitive proxy")
		if ok, err := r.deletePrimitive(pod, primitive, log); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

func (r *ProxyReconciler) updatePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
	// Update the proxy based on the status of primitive conditions
	conditions := NewPrimitiveConditions(primitive.Name, pod.Status.Conditions)
	switch conditions.GetReady() {
	case corev1.ConditionUnknown:
		// If the primitive ready condition is Unknown, ensure the proxy is deleted and set the
		// ready condition to False so it can be recreated
		if _, err := r.deleteProxy(pod, primitive, log); err != nil {
			return false, err
		}
		log.Info("Updating primitive conditions")
		pod.Status.Conditions = conditions.SetReady(corev1.ConditionFalse)
		err := r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Updating primitive conditions")
			return false, err
		}
		return true, nil
	case corev1.ConditionFalse:
		// If the primitive ready condition is False, ensure the proxy is created and set the
		// ready condition to True
		if _, err := r.createProxy(pod, primitive, log); err != nil {
			return false, err
		}
		if _, err := r.registerPrimitive(pod, primitive, log); err != nil {
			return false, err
		}
		log.Info("Updating primitive conditions")
		pod.Status.Conditions = conditions.SetReady(corev1.ConditionTrue)
		err := r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			log.Error(err, "Updating primitive conditions")
			return false, err
		}
		return true, nil
	case corev1.ConditionTrue:
		// If the primitive ready condition is True, check whether the permissions have been changed and the proxy
		// needs to be updated
		read, write, err := r.getPermissions(pod, primitive)
		if err != nil {
			return false, err
		}

		changed := false
		if read && conditions.GetReadAccess() != corev1.ConditionTrue {
			log.Info("Read permission changed")
			pod.Status.Conditions = conditions.SetReadAccess(corev1.ConditionTrue)
			changed = true
		} else if !read && conditions.GetReadAccess() != corev1.ConditionFalse {
			log.Info("Read permission changed")
			pod.Status.Conditions = conditions.SetReadAccess(corev1.ConditionFalse)
			changed = true
		}

		if write && conditions.GetWriteAccess() != corev1.ConditionTrue {
			log.Info("Write permission changed")
			pod.Status.Conditions = conditions.SetWriteAccess(corev1.ConditionTrue)
			changed = true
		} else if !write && conditions.GetWriteAccess() != corev1.ConditionFalse {
			log.Info("Write permission changed")
			pod.Status.Conditions = conditions.SetWriteAccess(corev1.ConditionFalse)
			changed = true
		}

		if changed {
			log.Info("Updating primitive conditions")
			pod.Status.Conditions = conditions.SetReady(corev1.ConditionUnknown)
			err := r.client.Status().Update(context.TODO(), pod)
			if err != nil {
				log.Error(err, "Updating primitive conditions")
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
	return false, nil
}

func (r *ProxyReconciler) createProxy(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
	conditions := NewPrimitiveConditions(primitive.Name, pod.Status.Conditions)

	log.Info("Connecting to protocol agent")
	conn, err := r.connectAgent(pod, primitive)
	if err != nil {
		log.Error(err, "Connecting to protocol agent")
		return false, err
	}
	defer conn.Close()
	client := driverapi.NewAgentClient(conn)

	log.Info("Creating primitive proxy")
	request := &driverapi.CreateProxyRequest{
		ProxyID: driverapi.ProxyId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
		Options: driverapi.ProxyOptions{
			Read:  conditions.GetReadAccess() == corev1.ConditionTrue,
			Write: conditions.GetWriteAccess() == corev1.ConditionTrue,
		},
	}
	_, err = client.CreateProxy(context.TODO(), request)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return false, nil
		}
		log.Error(err, "Creating primitive proxy")
		return false, err
	}
	return true, nil
}

func (r *ProxyReconciler) registerPrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
	log.Info("Connecting to broker")
	conn, err := r.connectBroker(pod)
	if err != nil {
		log.Error(err, "Connecting to broker")
		return false, err
	}
	defer conn.Close()
	client := brokerapi.NewBrokerClient(conn)

	log.Info("Registering primitive with broker")
	name := types.NamespacedName{
		Namespace: primitive.Spec.Store.Namespace,
		Name:      primitive.Spec.Store.Name,
	}
	if name.Namespace == "" {
		name.Namespace = primitive.Namespace
	}
	conditions := NewProtocolConditions(name, pod.Status.Conditions)
	port, err := conditions.GetPort()
	if err != nil {
		log.Error(err, "Registering primitive with broker")
		return false, err
	}
	request := &brokerapi.RegisterPrimitiveRequest{
		PrimitiveID: brokerapi.PrimitiveId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
		Address: brokerapi.PrimitiveAddress{
			Host: "127.0.0.1",
			Port: int32(port),
		},
	}
	_, err = client.RegisterPrimitive(context.TODO(), request)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		log.Error(err, "Registering primitive with broker")
		return false, err
	}
	return true, nil
}

func (r *ProxyReconciler) deletePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
	conditions := NewPrimitiveConditions(primitive.Name, pod.Status.Conditions)
	if conditions.GetReady() == corev1.ConditionUnknown {
		return false, nil
	}

	if _, err := r.unregisterPrimitive(pod, primitive, log); err != nil {
		return false, err
	}
	if _, err := r.deleteProxy(pod, primitive, log); err != nil {
		return false, err
	}

	log.Info("Updating primitive ready condition")
	pod.Status.Conditions = conditions.SetReady(corev1.ConditionUnknown)
	err := r.client.Status().Update(context.TODO(), pod)
	if err != nil {
		log.Error(err, "Updating primitive ready condition")
		return false, err
	}
	return true, nil
}

func (r *ProxyReconciler) deleteProxy(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
	log.Info("Connecting to protocol agent")
	conn, err := r.connectAgent(pod, primitive)
	if err != nil {
		log.Error(err, "Connecting to protocol agent")
		return false, err
	}
	defer conn.Close()
	client := driverapi.NewAgentClient(conn)

	log.Info("Destroying primitive proxy")
	request := &driverapi.DestroyProxyRequest{
		ProxyID: driverapi.ProxyId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
	}
	_, err = client.DestroyProxy(context.TODO(), request)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		log.Error(err, "Destroying primitive proxy")
		return false, err
	}
	return true, nil
}

func (r *ProxyReconciler) unregisterPrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
	log.Info("Connecting to broker")
	conn, err := r.connectBroker(pod)
	if err != nil {
		log.Error(err, "Connecting to broker")
		return false, err
	}
	defer conn.Close()
	client := brokerapi.NewBrokerClient(conn)

	log.Info("Unregistering primitive with broker")
	request := &brokerapi.UnregisterPrimitiveRequest{
		PrimitiveID: brokerapi.PrimitiveId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
	}
	_, err = client.UnregisterPrimitive(context.TODO(), request)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		log.Error(err, "Unregistering primitive with broker")
		return false, err
	}
	return true, nil
}

func (r *ProxyReconciler) connectAgent(pod *corev1.Pod, primitive v2beta1.Primitive) (*grpc.ClientConn, error) {
	name := types.NamespacedName{
		Namespace: primitive.Spec.Store.Namespace,
		Name:      primitive.Spec.Store.Name,
	}
	if name.Namespace == "" {
		name.Namespace = primitive.Namespace
	}
	conditions := NewProtocolConditions(name, pod.Status.Conditions)
	port, err := conditions.GetPort()
	if err != nil {
		return nil, err
	}
	address := fmt.Sprintf("%s:%d", pod.Status.PodIP, port)
	return grpc.Dial(address, grpc.WithInsecure())
}

func (r *ProxyReconciler) connectBroker(pod *corev1.Pod) (*grpc.ClientConn, error) {
	address := fmt.Sprintf("%s:5678", pod.Status.PodIP)
	return grpc.Dial(address, grpc.WithInsecure())
}

func (r *ProxyReconciler) getPermissions(pod *corev1.Pod, primitive v2beta1.Primitive) (read bool, write bool, err error) {
	if pod.Spec.ServiceAccountName == "" {
		return true, true, nil
	}

	serviceAccount := &corev1.ServiceAccount{}
	serviceAccountName := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Spec.ServiceAccountName,
	}
	err = r.client.Get(context.TODO(), serviceAccountName, serviceAccount)
	if err != nil {
		return false, false, err
	}

	owner := primitive.OwnerReferences[0]
	groupVersion, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return false, false, err
	}
	primitiveKind := schema.GroupVersionKind{
		Group:   groupVersion.Group,
		Version: groupVersion.Version,
		Kind:    owner.Kind,
	}

	clusterRoleKind, err := getGroupVersionKind(r.scheme, &rbacv1.ClusterRole{})
	if err != nil {
		return false, false, err
	}

	roleKind, err := getGroupVersionKind(r.scheme, &rbacv1.Role{})
	if err != nil {
		return false, false, err
	}

	roleBindings := &rbacv1.RoleBindingList{}
	if err := r.client.List(context.TODO(), roleBindings, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return false, false, err
	}

	for _, roleBinding := range roleBindings.Items {
		for _, subject := range roleBinding.Subjects {
			if subject.Kind == rbacv1.ServiceAccountKind &&
				((subject.Name == "default" && pod.Spec.ServiceAccountName == "") ||
					subject.Name == pod.Spec.ServiceAccountName) {
				if roleBinding.RoleRef.Kind == clusterRoleKind.Kind {
					clusterRole := &rbacv1.ClusterRole{}
					clusterRoleName := types.NamespacedName{
						Name: roleBinding.RoleRef.Name,
					}
					if err := r.client.Get(context.TODO(), clusterRoleName, clusterRole); err != nil {
						return false, false, err
					}
					for _, rule := range clusterRole.Rules {
						if isPrimitiveGroupRule(rule, primitiveKind) && isPrimitiveTypeRule(rule, primitiveKind) && isPrimitiveRule(rule, primitive.Name) {
							if isPrimitiveReadRule(rule) {
								read = true
							}
							if isPrimitiveWriteRule(rule) {
								write = true
							}
						}
					}
				} else if roleBinding.RoleRef.Kind == roleKind.Kind {
					role := &rbacv1.Role{}
					roleName := types.NamespacedName{
						Namespace: roleBinding.Namespace,
						Name:      roleBinding.RoleRef.Name,
					}
					if err := r.client.Get(context.TODO(), roleName, role); err != nil {
						return false, false, err
					}
					for _, rule := range role.Rules {
						if isPrimitiveGroupRule(rule, primitiveKind) && isPrimitiveTypeRule(rule, primitiveKind) && isPrimitiveRule(rule, primitive.Name) {
							if isPrimitiveReadRule(rule) {
								read = true
							}
							if isPrimitiveWriteRule(rule) {
								write = true
							}
						}
					}
				}
			}
		}
	}

	clusterRoleBindings := &rbacv1.RoleBindingList{}
	if err := r.client.List(context.TODO(), clusterRoleBindings); err != nil {
		return false, false, err
	}

	for _, clusterRoleBinding := range clusterRoleBindings.Items {
		for _, subject := range clusterRoleBinding.Subjects {
			if subject.Kind == rbacv1.ServiceAccountKind &&
				((subject.Name == "default" && pod.Spec.ServiceAccountName == "") ||
					subject.Name == pod.Spec.ServiceAccountName) {
				if clusterRoleBinding.RoleRef.Kind == clusterRoleKind.Kind {
					clusterRole := &rbacv1.ClusterRole{}
					clusterRoleName := types.NamespacedName{
						Name: clusterRoleBinding.RoleRef.Name,
					}
					if err := r.client.Get(context.TODO(), clusterRoleName, clusterRole); err != nil {
						return false, false, err
					}
					for _, rule := range clusterRole.Rules {
						if isPrimitiveGroupRule(rule, primitiveKind) && isPrimitiveTypeRule(rule, primitiveKind) && isPrimitiveRule(rule, primitive.Name) {
							if isPrimitiveReadRule(rule) {
								read = true
							}
							if isPrimitiveWriteRule(rule) {
								write = true
							}
						}
					}
				} else if clusterRoleBinding.RoleRef.Kind == roleKind.Kind {
					role := &rbacv1.Role{}
					roleName := types.NamespacedName{
						Namespace: clusterRoleBinding.Namespace,
						Name:      clusterRoleBinding.RoleRef.Name,
					}
					if err := r.client.Get(context.TODO(), roleName, role); err != nil {
						return false, false, err
					}
					for _, rule := range role.Rules {
						if isPrimitiveGroupRule(rule, primitiveKind) && isPrimitiveTypeRule(rule, primitiveKind) && isPrimitiveRule(rule, primitive.Name) {
							if isPrimitiveReadRule(rule) {
								read = true
							}
							if isPrimitiveWriteRule(rule) {
								write = true
							}
						}
					}
				}
			}
		}
	}
	return read, write, err
}

func isPrimitiveGroupRule(rule rbacv1.PolicyRule, primitiveType schema.GroupVersionKind) bool {
	for _, apiGroup := range rule.APIGroups {
		if apiGroup == rbacv1.APIGroupAll || apiGroup == primitiveType.Group {
			return true
		}
	}
	return false
}

func isPrimitiveTypeRule(rule rbacv1.PolicyRule, primitiveType schema.GroupVersionKind) bool {
	for _, resource := range rule.Resources {
		if resource == rbacv1.ResourceAll || resource == primitiveType.Kind {
			return true
		}
	}
	return false
}

func isPrimitiveRule(rule rbacv1.PolicyRule, name string) bool {
	if len(rule.ResourceNames) == 0 {
		return true
	}
	for _, resourceName := range rule.ResourceNames {
		if resourceName == name {
			return true
		}
	}
	return false
}

func isPrimitiveReadRule(rule rbacv1.PolicyRule) bool {
	for _, verb := range rule.Verbs {
		if verb == rbacv1.VerbAll || verb == verbRead {
			return true
		}
	}
	return false
}

func isPrimitiveWriteRule(rule rbacv1.PolicyRule) bool {
	for _, verb := range rule.Verbs {
		if verb == rbacv1.VerbAll || verb == verbWrite {
			return true
		}
	}
	return false
}

func getGroupVersionKind(scheme *runtime.Scheme, object runtime.Object) (schema.GroupVersionKind, error) {
	kinds, _, err := scheme.ObjectKinds(object)
	if err != nil {
		return schema.GroupVersionKind{}, err
	} else if len(kinds) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("kind %s not found", object)
	}
	return kinds[0], nil
}

var _ reconcile.Reconciler = &ProxyReconciler{}
