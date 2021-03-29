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
	"github.com/atomix/api/go/atomix/management/broker"
	"github.com/atomix/go-framework/pkg/atomix/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"strings"
)

var log = logging.GetLogger("atomix", "controller", "primitives")

const (
	verbRead  = "read"
	verbWrite = "write"
)

type primitiveType struct {
	object runtime.Object
	list   runtime.Object
}

// DriverReconciler reconciles a Pod object
type DriverReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client            client.Client
	scheme            *runtime.Scheme
	config            *rest.Config
	storageType       string
	storageAPIVersion string
	storageKind       string
	storageController StorageController
	primitiveTypes    []primitiveType
}

// Reconcile reads that state of the cluster for a Pod object and makes changes based on the state
// of the Pod, primitives, and RBAC controls
func (r *DriverReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Pod '%s'", request.NamespacedName)

	// Fetch the Pod instance
	pod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), request.NamespacedName, pod)
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

	// Determine whether the pod supports Atomix primitives by looking for the broker inject annotation
	injectBroker, ok := pod.Annotations[brokerInjectAnnotation]
	if !ok {
		return reconcile.Result{}, nil
	}
	if inject, err := strconv.ParseBool(injectBroker); err != nil {
		return reconcile.Result{}, err
	} else if !inject {
		return reconcile.Result{}, nil
	}

	// Ensure the storage readiness condition is present on the pod status
	if ok, err := r.addStorageConditions(pod); err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}

	// Ensure the injected drivers' readiness conditions are present on the pod status
	for _, driver := range r.getInjectedDrivers(pod) {
		if ok, err := r.addDriverConditions(pod, driver); err != nil {
			return reconcile.Result{}, err
		} else if ok {
			return reconcile.Result{}, nil
		}
	}

	// Get the set of primitives for the pod
	primitives, err := r.getPrimitives(pod)
	if err != nil {
		return reconcile.Result{}, err
	}

	// If ephemeral containers are enabled, add missing drivers
	if r.isEphemeralContainersEnabled(pod) {
		for driver := range primitives {
			if ok, err := r.addDriver(pod, driver); err != nil {
				return reconcile.Result{}, err
			} else if ok {
				return reconcile.Result{}, nil
			}
		}
	}

	// For each driver, configure the driver and its primitives
	for _, driver := range r.getEnabledDrivers(pod) {
		if ok, err := r.configureDriver(pod, driver, primitives[driver]); err != nil {
			return reconcile.Result{}, err
		} else if ok {
			return reconcile.Result{}, nil
		}
	}

	// If all drivers are ready, set the storage condition to ready
	if ok, err := r.setStorageConditions(pod); err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *DriverReconciler) addStorageConditions(pod *corev1.Pod) (bool, error) {
	storageReadyCondition := getStorageReadyCondition(r.storageType)
	for _, condition := range pod.Status.Conditions {
		if condition.Type == storageReadyCondition {
			return false, nil
		}
	}

	log.Infof("Adding storage condition '%s' to Pod '%s'", storageReadyCondition, newName(pod.Namespace, pod.Name))
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               storageReadyCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
	})

	if err := r.client.Status().Update(context.TODO(), pod); err != nil {
		log.Warn("Update failed", err)
		return false, err
	}
	return true, nil
}

func (r *DriverReconciler) isEphemeralContainersEnabled(pod *corev1.Pod) bool {
	return pod.Annotations[enableEphemeralContainersAnnotation] == fmt.Sprint(true)
}

func (r *DriverReconciler) addDriver(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	if ok, err := r.addDriverConditions(pod, driver); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	if ok, err := r.addDriverContainer(pod, driver); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	return false, nil
}

func (r *DriverReconciler) isDriverInjected(pod *corev1.Pod, driver broker.DriverId) bool {
	statusAnnotation := getDriverStatusAnnotation(driver)
	return pod.Annotations[statusAnnotation] == injectedStatus
}

func (r *DriverReconciler) nextDriverPort(pod *corev1.Pod) int32 {
	nextPort := baseDriverPort
	for _, condition := range pod.Status.Conditions {
		if isPortCondition(condition.Type) {
			port := getDriverPortConditionValue(condition.Type)
			if port > nextPort {
				nextPort = port + 1
			}
		}
	}
	return nextPort
}

func (r *DriverReconciler) getDriverPort(pod *corev1.Pod, driver broker.DriverId) (int32, error) {
	for _, condition := range pod.Status.Conditions {
		if isDriverPortCondition(condition.Type, driver) {
			return getDriverPortConditionValue(condition.Type), nil
		}
	}
	return 0, fmt.Errorf("port not found for driver '%s' in Pod '%s' status", newName(driver.Namespace, driver.Name), newName(pod.Namespace, pod.Name))
}

func (r *DriverReconciler) addDriverConditions(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	driverReadyCondition := getDriverReadyCondition(driver)
	for _, condition := range pod.Status.Conditions {
		if condition.Type == driverReadyCondition {
			return false, nil
		}
	}

	log.Infof("Adding driver condition '%s' to Pod '%s'", driverReadyCondition, newName(pod.Namespace, pod.Name))
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               driverReadyCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
	})

	port := r.nextDriverPort(pod)
	portAnnotation := getDriverPortAnnotation(driver)
	portValue, ok := pod.Annotations[portAnnotation]
	if ok {
		p, err := strconv.Atoi(portValue)
		if err != nil {
			return false, err
		}
		port = int32(p)
	}

	portConditionType := getDriverPortCondition(driver, port)
	log.Infof("Adding driver condition '%s' to Pod '%s'", portConditionType, newName(pod.Namespace, pod.Name))
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               portConditionType,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
	})

	if err := r.client.Status().Update(context.TODO(), pod); err != nil {
		log.Warn("Update failed", err)
		return false, err
	}
	return true, nil
}

func (r *DriverReconciler) addDriverContainer(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	if r.isDriverInjected(pod, driver) {
		return false, nil
	}

	client, err := kubernetes.NewForConfig(r.config)
	if err != nil {
		return false, err
	}

	containers := &corev1.EphemeralContainers{}
	err = client.CoreV1().RESTClient().Get().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("ephemeralcontainers").
		VersionedParams(&metav1.GetOptions{}, scheme.ParameterCodec).
		Do().
		Into(containers)
	if err != nil {
		log.Warn("Get failed", err)
		return false, err
	}

	port, err := r.getDriverPort(pod, driver)
	if err != nil {
		log.Warn(err)
		return false, err
	}

	config := broker.DriverConfig{
		ID:   driver,
		Host: "127.0.0.1",
		Port: port,
	}

	container, err := r.storageController.NewEphemeralContainer(config)
	if err != nil {
		log.Warn("NewEphemeralContainer failed", err)
		return false, err
	}

	for _, c := range containers.EphemeralContainers {
		if c.Name == container.Name {
			return false, nil
		}
	}

	log.Infof("Adding ephemeral container '%s' for driver '%s' to Pod '%s'", container.Name, newName(driver.Namespace, driver.Name), newName(pod.Namespace, pod.Name))
	containers.EphemeralContainers = append(containers.EphemeralContainers, container)

	err = client.CoreV1().RESTClient().Put().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("ephemeralcontainers").
		Body(containers).
		Do().
		Error()
	if err := r.client.Update(context.TODO(), containers); err != nil {
		log.Warn("Update failed", err)
		return false, err
	}
	return true, nil
}

func (r *DriverReconciler) getInjectedDrivers(pod *corev1.Pod) []broker.DriverId {
	driverNames := pod.Annotations[getStorageStatusAnnotation(r.storageType)]
	var drivers []broker.DriverId
	for _, driverName := range strings.Split(driverNames, ",") {
		if driverName != "" {
			driver := broker.DriverId{
				Type:      r.storageType,
				Namespace: pod.Namespace,
				Name:      driverName,
			}
			drivers = append(drivers, driver)
		}
	}
	return drivers
}

func (r *DriverReconciler) getEnabledDrivers(pod *corev1.Pod) []broker.DriverId {
	var drivers []broker.DriverId
	for _, condition := range pod.Status.Conditions {
		if isDriverReadyCondition(condition.Type) {
			driver := broker.DriverId{
				Type:      r.storageType,
				Namespace: pod.Namespace,
				Name:      getDriverReadyConditionDriver(condition.Type),
			}
			drivers = append(drivers, driver)
		}
	}
	return drivers
}

func (r *DriverReconciler) configureDriver(pod *corev1.Pod, driver broker.DriverId, primitives []broker.PrimitiveConfig) (bool, error) {
	if !r.isDriverPortReady(pod, driver) {
		if ok, err := r.enableDriver(pod, driver); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}

	for _, primitive := range primitives {
		if ok, err := r.addPrimitive(pod, primitive); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}

	if ok, err := r.updateDriver(pod, driver); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}
	return false, nil
}

func (r *DriverReconciler) enableDriver(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	log.Infof("Configuring driver '%s' in pod '%s'", newName(driver.Namespace, driver.Name), newName(pod.Namespace, pod.Name))
	conn, err := grpc.Dial(fmt.Sprintf("%s:5151", pod.Status.PodIP), grpc.WithInsecure())
	if err != nil {
		log.Warn("Dial failed", err)
		return false, err
	}
	defer conn.Close()
	client := broker.NewDriverManagementServiceClient(conn)
	config, err := r.getDriverConfig(pod, driver)
	if err != nil {
		log.Warn("DriverConfig failed", err)
		return false, err
	}
	request := &broker.AddDriverRequest{
		Driver: config,
	}
	_, err = client.AddDriver(context.TODO(), request)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return false, nil
		}
		log.Warn("AddDriver failed", err)
		return false, err
	}
	return r.setDriverPortReady(pod, driver)
}

func (r *DriverReconciler) updateDriver(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	log.Infof("Reconfiguring driver '%s' in pod '%s'", newName(driver.Namespace, driver.Name), newName(pod.Namespace, pod.Name))
	conn, err := grpc.Dial(fmt.Sprintf("%s:5151", pod.Status.PodIP), grpc.WithInsecure())
	if err != nil {
		log.Warn("Dial failed", err)
		return false, err
	}
	defer conn.Close()
	client := broker.NewDriverManagementServiceClient(conn)
	config, err := r.getDriverConfig(pod, driver)
	if err != nil {
		log.Warn("DriverConfig failed", err)
		return false, err
	}
	request := &broker.UpdateDriverRequest{
		Driver: config,
	}
	_, err = client.UpdateDriver(context.TODO(), request)
	if err != nil {
		log.Warn("UpdateDriver failed", err)
		return false, err
	}
	return r.setDriverConditions(pod, driver)
}

func (r *DriverReconciler) getDriverConfig(pod *corev1.Pod, driver broker.DriverId) (broker.DriverConfig, error) {
	port, err := r.getDriverPort(pod, driver)
	if err != nil {
		return broker.DriverConfig{}, err
	}
	protocol, err := r.storageController.GetProtocol(driver)
	if err != nil {
		log.Warn("GetProtocol failed", err)
		return broker.DriverConfig{}, err
	}
	return broker.DriverConfig{
		ID:       driver,
		Host:     "127.0.0.1",
		Port:     port,
		Protocol: &protocol,
	}, nil
}

func (r *DriverReconciler) isDriverPortReady(pod *corev1.Pod, driver broker.DriverId) bool {
	for _, condition := range pod.Status.Conditions {
		if isDriverPortCondition(condition.Type, driver) {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *DriverReconciler) setDriverPortReady(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	for i, condition := range pod.Status.Conditions {
		if isDriverPortCondition(condition.Type, driver) {
			if condition.Status == corev1.ConditionTrue {
				return false, nil
			}
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.Now()
			pod.Status.Conditions[i] = condition
			if err := r.client.Status().Update(context.TODO(), pod); err != nil {
				log.Warn("Update failed", err)
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (r *DriverReconciler) isDriverReady(pod *corev1.Pod, driver broker.DriverId) bool {
	driverReadyCondition := getDriverReadyCondition(driver)
	for _, condition := range pod.Status.Conditions {
		if condition.Type == driverReadyCondition {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (r *DriverReconciler) setDriverConditions(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	driverReady := getDriverReadyCondition(driver)
	for i, condition := range pod.Status.Conditions {
		if condition.Type == driverReady {
			if condition.Status == corev1.ConditionTrue {
				return false, nil
			}
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.Now()
			pod.Status.Conditions[i] = condition
			if err := r.client.Status().Update(context.TODO(), pod); err != nil {
				log.Warn("Update failed", err)
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (r *DriverReconciler) addPrimitive(pod *corev1.Pod, primitive broker.PrimitiveConfig) (bool, error) {
	conn, err := grpc.Dial(fmt.Sprintf("%s:5151", pod.Status.PodIP), grpc.WithInsecure())
	if err != nil {
		return false, err
	}
	defer conn.Close()
	client := broker.NewPrimitiveManagementServiceClient(conn)
	request := &broker.AddPrimitiveRequest{
		Primitive: primitive,
	}
	_, err = client.AddPrimitive(context.TODO(), request)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return false, nil
		}
		return false, err
	}
	return false, nil
}

func (r *DriverReconciler) setStorageConditions(pod *corev1.Pod) (bool, error) {
	storageReady := getStorageReadyCondition(r.storageType)
	for i, condition := range pod.Status.Conditions {
		if condition.Type == storageReady {
			if condition.Status == corev1.ConditionTrue {
				return false, nil
			}
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.Now()
			pod.Status.Conditions[i] = condition
			if err := r.client.Status().Update(context.TODO(), pod); err != nil {
				log.Warn("Update failed", err)
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (r *DriverReconciler) listPrimitives(t primitiveType) ([]broker.PrimitiveConfig, error) {
	objectKind, err := getGroupVersionKind(r.scheme, t.object)
	if err != nil {
		return nil, err
	}

	listKind, err := getGroupVersionKind(r.scheme, t.list)
	if err != nil {
		return nil, err
	}

	primitives, err := r.scheme.New(listKind)
	if err != nil {
		return nil, err
	}
	if err := r.client.List(context.TODO(), primitives); err != nil {
		return nil, err
	}

	value := reflect.ValueOf(primitives).Elem()
	slice := value.FieldByName("Items")
	names := make([]broker.PrimitiveConfig, 0, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		primitive := slice.Index(i)
		spec := primitive.FieldByName("Spec")
		storage := spec.FieldByName("Storage")
		apiVersion := storage.FieldByName("APIVersion").String()
		if apiVersion != r.storageAPIVersion {
			continue
		}
		kind := storage.FieldByName("Kind").String()
		if kind != r.storageKind {
			continue
		}
		storageNamespace := storage.FieldByName("Namespace").String()
		storageName := storage.FieldByName("Name").String()
		primitiveNamespace := primitive.FieldByName("Namespace").String()
		primitiveName := primitive.FieldByName("Name").String()
		if storageNamespace == "" {
			storageNamespace = primitiveNamespace
		}
		config := broker.PrimitiveConfig{
			ID: broker.PrimitiveId{
				Type:      objectKind.Kind,
				Namespace: primitiveNamespace,
				Name:      primitiveName,
			},
			Driver: broker.DriverId{
				Namespace: storageNamespace,
				Name:      storageName,
				Type:      r.storageType,
			},
		}
		names = append(names, config)
	}
	return names, nil
}

func (r *DriverReconciler) getPrimitives(pod *corev1.Pod) (map[broker.DriverId][]broker.PrimitiveConfig, error) {
	primitivesSet := make(map[broker.PrimitiveId]broker.PrimitiveConfig)

	primitiveTypeNames := make(map[schema.GroupVersionKind][]broker.PrimitiveConfig)
	for _, t := range r.primitiveTypes {
		primitiveNames, err := r.listPrimitives(t)
		if err != nil {
			return nil, err
		}
		primitiveKind, err := getGroupVersionKind(r.scheme, t.object)
		if err != nil {
			return nil, err
		}
		primitiveTypeNames[primitiveKind] = primitiveNames
	}

	roleBindings := &rbacv1.RoleBindingList{}
	if err := r.client.List(context.TODO(), roleBindings, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return nil, err
	}

	clusterRoleKind, err := getGroupVersionKind(r.scheme, &rbacv1.ClusterRole{})
	if err != nil {
		return nil, err
	}

	roleKind, err := getGroupVersionKind(r.scheme, &rbacv1.Role{})
	if err != nil {
		return nil, err
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
						return nil, err
					}
					for primitiveType, primitiveNames := range primitiveTypeNames {
						primitivesSet = mergePrimitives(primitivesSet, getPrimitivesForClusterRole(clusterRole, primitiveType, primitiveNames))
					}
				} else if roleBinding.RoleRef.Kind == roleKind.Kind {
					role := &rbacv1.Role{}
					roleName := types.NamespacedName{
						Namespace: roleBinding.Namespace,
						Name:      roleBinding.RoleRef.Name,
					}
					if err := r.client.Get(context.TODO(), roleName, role); err != nil {
						return nil, err
					}
					for primitiveType, primitiveNames := range primitiveTypeNames {
						primitivesSet = mergePrimitives(primitivesSet, getPrimitivesForRole(role, primitiveType, primitiveNames))
					}
				}
			}
		}
	}

	clusterRoleBindings := &rbacv1.RoleBindingList{}
	if err := r.client.List(context.TODO(), clusterRoleBindings); err != nil {
		return nil, err
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
						return nil, err
					}
					for primitiveType, primitiveNames := range primitiveTypeNames {
						primitivesSet = mergePrimitives(primitivesSet, getPrimitivesForClusterRole(clusterRole, primitiveType, primitiveNames))
					}
				} else if clusterRoleBinding.RoleRef.Kind == roleKind.Kind {
					role := &rbacv1.Role{}
					roleName := types.NamespacedName{
						Namespace: clusterRoleBinding.Namespace,
						Name:      clusterRoleBinding.RoleRef.Name,
					}
					if err := r.client.Get(context.TODO(), roleName, role); err != nil {
						return nil, err
					}
					for primitiveType, primitiveNames := range primitiveTypeNames {
						primitivesSet = mergePrimitives(primitivesSet, getPrimitivesForRole(role, primitiveType, primitiveNames))
					}
				}
			}
		}
	}

	drivers := make(map[broker.DriverId][]broker.PrimitiveConfig)
	for _, primitive := range primitivesSet {
		primitives := drivers[primitive.Driver]
		primitives = append(primitives, primitive)
		drivers[primitive.Driver] = primitives
	}
	return drivers, nil
}

func mergePrimitives(primitives map[broker.PrimitiveId]broker.PrimitiveConfig, updates map[broker.PrimitiveId]broker.PrimitiveConfig) map[broker.PrimitiveId]broker.PrimitiveConfig {
	for resource, update := range updates {
		primitive, ok := primitives[resource]
		if !ok {
			primitive = update
		} else {
			if update.Driver.Type != "" {
				primitive.Driver.Type = update.Driver.Type
			}
			if update.Driver.Namespace != "" {
				primitive.Driver.Namespace = update.Driver.Namespace
			}
			if update.Driver.Name != "" {
				primitive.Driver.Name = update.Driver.Name
			}
			if update.Proxy.Read {
				primitive.Proxy.Read = true
			}
			if update.Proxy.Write {
				primitive.Proxy.Write = true
			}
		}
		primitives[resource] = primitive
	}
	return primitives
}

func getPrimitivesForRole(role *rbacv1.Role, primitiveType schema.GroupVersionKind, resources []broker.PrimitiveConfig) map[broker.PrimitiveId]broker.PrimitiveConfig {
	primitives := make(map[broker.PrimitiveId]broker.PrimitiveConfig)
	for _, rule := range role.Rules {
		primitives = mergePrimitives(primitives, getPrimitivesForPolicyRule(rule, primitiveType, resources))
	}
	return primitives
}

func getPrimitivesForClusterRole(clusterRole *rbacv1.ClusterRole, primitiveType schema.GroupVersionKind, resources []broker.PrimitiveConfig) map[broker.PrimitiveId]broker.PrimitiveConfig {
	primitives := make(map[broker.PrimitiveId]broker.PrimitiveConfig)
	for _, rule := range clusterRole.Rules {
		primitives = mergePrimitives(primitives, getPrimitivesForPolicyRule(rule, primitiveType, resources))
	}
	return primitives
}

func getPrimitivesForPolicyRule(rule rbacv1.PolicyRule, primitiveType schema.GroupVersionKind, resources []broker.PrimitiveConfig) map[broker.PrimitiveId]broker.PrimitiveConfig {
	primitives := make(map[broker.PrimitiveId]broker.PrimitiveConfig)
	if !isPrimitiveGroupRule(rule, primitiveType) {
		return primitives
	}
	if !isPrimitiveTypeRule(rule, primitiveType) {
		return primitives
	}

	for _, resource := range resources {
		if isPrimitiveRule(rule, resource) {
			primitive, ok := primitives[resource.ID]
			if !ok {
				primitive = resource
			}
			if isPrimitiveReadRule(rule) {
				primitive.Proxy.Read = true
			}
			if isPrimitiveWriteRule(rule) {
				primitive.Proxy.Write = true
			}
			primitives[resource.ID] = primitive
		}
	}
	return primitives
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

func isPrimitiveRule(rule rbacv1.PolicyRule, name broker.PrimitiveConfig) bool {
	if len(rule.ResourceNames) == 0 {
		return true
	}
	for _, resourceName := range rule.ResourceNames {
		if resourceName == name.ID.Name {
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

func newName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

var _ reconcile.Reconciler = &DriverReconciler{}
