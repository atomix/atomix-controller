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
	"github.com/atomix/kubernetes-controller/pkg/apis/primitives/v2beta1"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"strings"
	"time"
)

var log = logf.Log.WithName("primitives_controller")

const (
	verbRead  = "read"
	verbWrite = "write"
)

const (
	driverTypeEnv      = "ATOMIX_DRIVER_TYPE"
	driverNodeEnv      = "ATOMIX_DRIVER_NODE"
	driverNamespaceEnv = "ATOMIX_DRIVER_NAMESPACE"
	driverNameEnv      = "ATOMIX_DRIVER_NAME"
	driverPortEnv      = "ATOMIX_DRIVER_PORT"
)

type primitiveType struct {
	object runtime.Object
	list   runtime.Object
}

// NewBuilder creates a new controller builder
func NewBuilder() *Builder {
	return &Builder{}
}

// Builder is a Kubernetes controller builder
type Builder struct {
	storageType   runtime.Object
	driverImage   string
	driverEnvFunc func(driverID broker.DriverId) []corev1.EnvVar
	primitives    []primitiveType
}

func (b *Builder) WithDriverImage(image string) *Builder {
	b.driverImage = image
	return b
}

func (b *Builder) WithDriverEnv(f func(driverID broker.DriverId) []corev1.EnvVar) *Builder {
	b.driverEnvFunc = f
	return b
}

func (b *Builder) WithStorageType(storageType runtime.Object) *Builder {
	b.storageType = storageType
	return b
}

func (b *Builder) AddPrimitiveType(object runtime.Object, list runtime.Object) *Builder {
	b.primitives = append(b.primitives, primitiveType{
		object: object,
		list:   list,
	})
	return b
}

func (b *Builder) Build(mgr manager.Manager) (controller.Controller, error) {
	r := &Reconciler{
		client:         mgr.GetClient(),
		scheme:         mgr.GetScheme(),
		config:         mgr.GetConfig(),
		driverImage:    b.driverImage,
		driverEnvFunc:  b.driverEnvFunc,
		storageType:    b.storageType,
		primitiveTypes: b.primitives,
	}

	// Create a new controller
	c, err := controller.New("primitives", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Watch for changes to primary resource Pod
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Counter resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Counter{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Election resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Election{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Lock resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Lock{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary List resource
	err = c.Watch(&source.Kind{Type: &v2beta1.List{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Map resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Map{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Set resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Set{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary Value resource
	err = c.Watch(&source.Kind{Type: &v2beta1.Value{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource Role
	err = c.Watch(&source.Kind{Type: &rbacv1.Role{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource RoleBinding
	err = c.Watch(&source.Kind{Type: &rbacv1.RoleBinding{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource ClusterRole
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRole{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	// Watch for changes to RBAC resource ClusterRoleBinding
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPodMapper(mgr),
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles a Pod object
type Reconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client         client.Client
	scheme         *runtime.Scheme
	config         *rest.Config
	driverImage    string
	driverEnvFunc  func(driverID broker.DriverId) []corev1.EnvVar
	storageType    runtime.Object
	primitiveTypes []primitiveType
}

// Reconcile reads that state of the cluster for a Pod object and makes changes based on the state
// of the Pod, primitives, and RBAC controls
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("Reconciling Pod", "Namespace", request.Namespace, "Name", request.Name)

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

	primitives, err := r.getPrimitivesForPod(pod)
	if err != nil {
		return reconcile.Result{}, err
	}

	if ok, err := r.addDrivers(pod, r.getDrivers(primitives)); err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}

	if ok, err := r.addPrimitives(pod, primitives); err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) getDrivers(primitives []broker.PrimitiveConfig) []broker.DriverId {
	driversSet := make(map[broker.DriverId]bool)
	for _, primitive := range primitives {
		driversSet[primitive.Driver] = true
	}
	drivers := make([]broker.DriverId, 0, len(driversSet))
	for driver := range driversSet {
		drivers = append(drivers, driver)
	}
	return drivers
}

func (r *Reconciler) addDrivers(pod *corev1.Pod, drivers []broker.DriverId) (bool, error) {
	for _, driver := range drivers {
		if ok, err := r.addDriverCondition(pod, driver); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	for _, driver := range drivers {
		if ok, err := r.addDriver(pod, driver); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	for _, driver := range drivers {
		if ok, err := r.setDriverCondition(pod, driver); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler) getDriverPort(pod *corev1.Pod, driver broker.DriverId) (int, bool, ) {
	conditionTypePrefix := fmt.Sprintf("%s.%s.storage.atomix.io", driver.Name, driver.Namespace)
	nextPort := 55680
while:
	for {
		for _, condition := range pod.Status.Conditions {
			parts := strings.Split(string(condition.Type), "/")
			if len(parts) != 2 {
				continue
			}
			if parts[0] == conditionTypePrefix {
				p, _ := strconv.Atoi(parts[1])
				return p, true
			}
			if strings.HasSuffix(parts[0], ".storage.atomix.io") {
				p, _ := strconv.Atoi(parts[1])
				if p >= nextPort {
					nextPort = p + 1
					continue while
				}
			}
		}
		return nextPort, false
	}
}

func (r *Reconciler) addDriverCondition(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	port, ok := r.getDriverPort(pod, driver)
	if ok {
		return false, nil
	}

	conditionType := corev1.PodConditionType(fmt.Sprintf("%s.%s.storage.atomix.io/%d", driver.Name, driver.Namespace, port))
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
		ConditionType: conditionType,
	})
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               conditionType,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
	if err := r.client.Update(context.TODO(), pod); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Reconciler) addDriver(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	containers := &corev1.EphemeralContainers{}
	name := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}
	if err := r.client.Get(context.TODO(), name, containers); err != nil {
		return false, err
	}
	containerName := strings.ToLower(fmt.Sprintf("%s-%s-%s", driver.Type, driver.Namespace, driver.Name))
	for _, container := range containers.EphemeralContainers {
		if container.Name == containerName {
			return false, nil
		}
	}

	port, ok := r.getDriverPort(pod, driver)
	if ok {
		return false, nil
	}

	env := []corev1.EnvVar{
		{
			Name:  driverTypeEnv,
			Value: driver.Type,
		},
		{
			Name:  driverNamespaceEnv,
			Value: driver.Namespace,
		},
		{
			Name:  driverNameEnv,
			Value: driver.Name,
		},
		{
			Name:  driverPortEnv,
			Value: fmt.Sprint(port),
		},
		{
			Name: driverNodeEnv,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		},
	}

	if r.driverEnvFunc != nil {
		env = append(env, r.driverEnvFunc(driver)...)
	}

	container := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            containerName,
			Image:           r.driverImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env:             env,
		},
	}

	containers.EphemeralContainers = append(containers.EphemeralContainers, container)
	if err := r.client.Update(context.TODO(), containers); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Reconciler) setDriverCondition(pod *corev1.Pod, driver broker.DriverId) (bool, error) {
	port, ok := r.getDriverPort(pod, driver)
	if !ok {
		return false, nil
	}

	conn, err := grpc.Dial(fmt.Sprintf("%s:5151", pod.Status.PodIP), grpc.WithInsecure())
	if err != nil {
		return false, err
	}
	defer conn.Close()
	client := broker.NewDriverManagementServiceClient(conn)
	request := &broker.AddDriverRequest{
		Driver: broker.DriverConfig{
			ID:   driver,
			Host: "127.0.0.1",
			Port: int32(port),
		},
	}
	_, err = client.AddDriver(context.TODO(), request)
	if err != nil {
		return false, err
	}

	conditionType := corev1.PodConditionType(fmt.Sprintf("%s.%s.storage.atomix.io/%d", driver.Name, driver.Namespace, port))
	for i, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.NewTime(time.Now())
			pod.Status.Conditions[i] = condition
			if err := r.client.Status().Update(context.TODO(), pod); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler) addPrimitives(pod *corev1.Pod, primitives []broker.PrimitiveConfig) (bool, error) {
	for _, primitive := range primitives {
		if ok, err := r.addPrimitiveCondition(pod, primitive); err != nil {
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
	return false, nil
}

func (r *Reconciler) addPrimitiveCondition(pod *corev1.Pod, primitive broker.PrimitiveConfig) (bool, error) {
	conditionType := corev1.PodConditionType(fmt.Sprintf("%s.primitive.atomix.io/%s", primitive.ID.Namespace, primitive.ID.Name))
	for _, gate := range pod.Spec.ReadinessGates {
		if gate.ConditionType == conditionType {
			return false, nil
		}
	}

	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
		ConditionType: conditionType,
	})
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               conditionType,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})
	if err := r.client.Update(context.TODO(), pod); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Reconciler) addPrimitive(pod *corev1.Pod, primitive broker.PrimitiveConfig) (bool, error) {
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
		return false, err
	}

	conditionType := corev1.PodConditionType(fmt.Sprintf("%s.primitive.atomix.io/%s", primitive.ID.Namespace, primitive.ID.Name))
	for i, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			condition.Status = corev1.ConditionTrue
			condition.LastTransitionTime = metav1.NewTime(time.Now())
			pod.Status.Conditions[i] = condition
			if err := r.client.Status().Update(context.TODO(), pod); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler) listPrimitives(t primitiveType) ([]broker.PrimitiveConfig, error) {
	listKinds, _, err := r.scheme.ObjectKinds(t.list)
	if err != nil {
		return nil, err
	} else if len(listKinds) == 0 {
		return nil, nil
	}
	listKind := listKinds[0]

	primitives, err := r.scheme.New(listKind)
	if err != nil {
		return nil, err
	}
	if err := r.client.List(context.TODO(), primitives); err != nil {
		return nil, err
	}

	storageKinds, _, err := r.scheme.ObjectKinds(r.storageType)
	if err != nil {
		return nil, err
	} else if len(storageKinds) == 0 {
		return nil, nil
	}
	storageKind := storageKinds[0]

	value := reflect.ValueOf(primitives).Elem()
	slice := value.FieldByName("Items")
	names := make([]broker.PrimitiveConfig, 0, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		primitive := slice.Index(i)
		spec := primitive.FieldByName("Spec")
		storage := spec.FieldByName("Storage")
		apiVersion := storage.FieldByName("APIVersion").String()
		if apiVersion != storageKind.GroupVersion().Identifier() {
			continue
		}
		kind := storage.FieldByName("Kind").String()
		if kind != storageKind.Kind {
			continue
		}
		storageNamespace := storage.FieldByName("Namespace").String()
		storageName := storage.FieldByName("Name").String()
		primitiveNamespace := primitive.FieldByName("Namespace").String()
		primitiveName := primitive.FieldByName("Name").String()
		config := broker.PrimitiveConfig{
			ID: broker.PrimitiveId{
				Namespace: primitiveNamespace,
				Name:      primitiveName,
			},
			Driver: broker.DriverId{
				Namespace: storageNamespace,
				Name:      storageName,
				Type:      "SomeCluster",
			},
		}
		names = append(names, config)
	}
	return names, nil
}

func (r *Reconciler) getPrimitivesForPod(pod *corev1.Pod) ([]broker.PrimitiveConfig, error) {
	primitivesSet := make(map[broker.PrimitiveId]broker.PrimitiveConfig)

	primitiveTypeNames := make(map[schema.GroupVersionKind][]broker.PrimitiveConfig)
	for _, t := range r.primitiveTypes {
		primitiveNames, err := r.listPrimitives(t)
		if err != nil {
			return nil, err
		}
		kinds, _, err := r.scheme.ObjectKinds(t.object)
		if err != nil {
			return nil, err
		} else if len(kinds) > 0 {
			primitiveTypeNames[kinds[0]] = primitiveNames
		}
	}

	roleBindings := &rbacv1.RoleBindingList{}
	if err := r.client.List(context.TODO(), roleBindings, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return nil, err
	}

	clusterRoleKind := (&rbacv1.ClusterRole{}).Kind
	roleKind := (&rbacv1.Role{}).Kind
	for _, roleBinding := range roleBindings.Items {
		for _, subject := range roleBinding.Subjects {
			if subject.Kind == rbacv1.ServiceAccountKind &&
				((subject.Name == "default" && pod.Spec.ServiceAccountName == "") ||
					subject.Name == pod.Spec.ServiceAccountName) {
				if roleBinding.RoleRef.Kind == clusterRoleKind {
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
				} else if roleBinding.RoleRef.Kind == roleKind {
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
				if clusterRoleBinding.RoleRef.Kind == clusterRoleKind {
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
				} else if clusterRoleBinding.RoleRef.Kind == roleKind {
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

	primitives := make([]broker.PrimitiveConfig, 0, len(primitivesSet))
	for _, primitive := range primitivesSet {
		primitives = append(primitives, primitive)
	}
	return primitives, nil
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
				primitive = broker.PrimitiveConfig{
					ID: broker.PrimitiveId{
						Type:      primitiveType.Kind,
						Namespace: resource.ID.Namespace,
						Name:      resource.ID.Name,
					},
				}
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
