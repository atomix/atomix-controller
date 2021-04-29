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
	brokerapi "github.com/atomix/atomix-api/go/atomix/management/broker"
	driverapi "github.com/atomix/atomix-api/go/atomix/management/driver"
	primitiveapi "github.com/atomix/atomix-api/go/atomix/primitive"
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"strings"
)

const (
	verbRead  = "read"
	verbWrite = "write"
)

func addPrimitiveController(mgr manager.Manager) error {
	r := &PrimitiveReconciler{
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
		ToRequests: newPodMapper(mgr),
	})
	return nil
}

// PrimitiveReconciler is a Reconciler for Primitive resources
type PrimitiveReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

func (r *PrimitiveReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

func (r *PrimitiveReconciler) prepareStatus(pod *corev1.Pod) (bool, error) {
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

func (r *PrimitiveReconciler) updateStatus(pod *corev1.Pod) (bool, error) {
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

func (r *PrimitiveReconciler) reconcilePrimitives(pod *corev1.Pod) (bool, error) {
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

func (r *PrimitiveReconciler) reconcilePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive) (bool, error) {
	log := newPrimitiveLogger(*pod, primitive)
	name := types.NamespacedName{
		Namespace: primitive.Spec.Store.Namespace,
		Name:      primitive.Spec.Store.Name,
	}
	if name.Namespace != "" {
		name.Namespace = primitive.Namespace
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

func (r *PrimitiveReconciler) updatePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
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

func (r *PrimitiveReconciler) createProxy(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
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

func (r *PrimitiveReconciler) registerPrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
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
	if name.Namespace != "" {
		name.Namespace = primitive.Namespace
	}
	conditions := NewProtocolConditions(name, pod.Status.Conditions)
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
			Port: int32(conditions.GetPort()),
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

func (r *PrimitiveReconciler) deletePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
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

func (r *PrimitiveReconciler) deleteProxy(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
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

func (r *PrimitiveReconciler) unregisterPrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, log logging.Logger) (bool, error) {
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

func (r *PrimitiveReconciler) connectAgent(pod *corev1.Pod, primitive v2beta1.Primitive) (*grpc.ClientConn, error) {
	name := types.NamespacedName{
		Namespace: primitive.Spec.Store.Namespace,
		Name:      primitive.Spec.Store.Name,
	}
	if name.Namespace != "" {
		name.Namespace = primitive.Namespace
	}
	conditions := NewProtocolConditions(name, pod.Status.Conditions)
	port := conditions.GetPort()
	address := fmt.Sprintf("%s:%d", pod.Status.PodIP, port)
	return grpc.Dial(address, grpc.WithInsecure())
}

func (r *PrimitiveReconciler) connectBroker(pod *corev1.Pod) (*grpc.ClientConn, error) {
	address := fmt.Sprintf("%s:5678", pod.Status.PodIP)
	return grpc.Dial(address, grpc.WithInsecure())
}

func (r *PrimitiveReconciler) getPermissions(pod *corev1.Pod, primitive v2beta1.Primitive) (read bool, write bool, err error) {
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

var _ reconcile.Reconciler = &PrimitiveReconciler{}

func isPrimitiveReadyCondition(condition corev1.PodConditionType) bool {
	return strings.HasSuffix(string(condition), ".primitive.atomix.io/ready")
}

// NewPrimitiveConditions returns new conditions helper for the given driver with the given conditions
func NewPrimitiveConditions(primitive string, conditions []corev1.PodCondition) *PrimitiveConditions {
	return &PrimitiveConditions{
		Primitive:  primitive,
		Conditions: conditions,
	}
}

// PrimitiveConditions provides utility functions for driver conditions
type PrimitiveConditions struct {
	Primitive  string
	Conditions []corev1.PodCondition
}

func (d *PrimitiveConditions) getReadyType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("primitives.atomix.io/%s", d.Primitive))
}

func (d *PrimitiveConditions) getReadAccessType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.primitives.atomix.io/read-access", d.Primitive))
}

func (d *PrimitiveConditions) getWriteAccessType() corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf("%s.primitives.atomix.io/write-access", d.Primitive))
}

func (d *PrimitiveConditions) getConditionStatus(conditionType corev1.PodConditionType) corev1.ConditionStatus {
	for _, condition := range d.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionUnknown
}

func (d *PrimitiveConditions) setCondition(condition corev1.PodCondition) []corev1.PodCondition {
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

func (d *PrimitiveConditions) GetReady() corev1.ConditionStatus {
	return d.getConditionStatus(d.getReadyType())
}

func (d *PrimitiveConditions) SetReady(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getReadyType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d *PrimitiveConditions) GetReadAccess() corev1.ConditionStatus {
	return d.getConditionStatus(d.getReadAccessType())
}

func (d *PrimitiveConditions) SetReadAccess(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getReadAccessType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func (d *PrimitiveConditions) GetWriteAccess() corev1.ConditionStatus {
	return d.getConditionStatus(d.getWriteAccessType())
}

func (d *PrimitiveConditions) SetWriteAccess(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getWriteAccessType(),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}

func newPodLogger(pod corev1.Pod) logging.Logger {
	fields := []logging.Field{
		logging.String("pod", types.NamespacedName{pod.Namespace, pod.Name}.String()),
	}
	return log.WithFields(fields...)
}

func newPrimitiveLogger(pod corev1.Pod, primitive v2beta1.Primitive) logging.Logger {
	fields := []logging.Field{
		logging.String("pod", types.NamespacedName{pod.Namespace, pod.Name}.String()),
		logging.String("primitive", types.NamespacedName{primitive.Namespace, primitive.Name}.String()),
		logging.String("type", primitive.OwnerReferences[0].Kind),
		logging.String("protocol", types.NamespacedName{primitive.Spec.Store.Namespace, primitive.Spec.Store.Name}.String()),
	}
	return log.WithFields(fields...)
}
