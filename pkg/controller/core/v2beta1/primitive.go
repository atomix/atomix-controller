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
	primitiveapi "github.com/atomix/api/go/atomix/primitive"
	"github.com/atomix/kubernetes-controller/pkg/apis/core/v2beta1"
	"google.golang.org/grpc"
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
	primitiveConditionFormat = "%s.primitive.atomix.io/%s"
	primitiveReadyCondition  = "ready"
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

	err = r.prepareStatus(pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.reconcilePrimitives(pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.updateStatus(pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *PrimitiveReconciler) prepareStatus(pod *corev1.Pod) error {
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

func (r *PrimitiveReconciler) updateStatus(pod *corev1.Pod) error {
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

func (r *PrimitiveReconciler) reconcilePrimitives(pod *corev1.Pod) error {
	primitives := &v2beta1.PrimitiveList{}
	err := r.client.List(context.TODO(), primitives, &client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		return err
	}
	for _, primitive := range primitives.Items {
		err := r.reconcilePrimitive(pod, primitive)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PrimitiveReconciler) reconcilePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive) error {
	protocol := &v2beta1.Protocol{}
	protocolName := types.NamespacedName{
		Namespace: primitive.Namespace,
		Name:      primitive.Spec.Protocol,
	}
	err := r.client.Get(context.TODO(), protocolName, protocol)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	protocolCond := NewProtocolConditions(protocol.Name, pod.Status.Conditions)
	if protocolCond.GetReady() != corev1.ConditionTrue {
		return nil
	}

	primitiveCond := NewPrimitiveConditions(primitive.Name, pod.Status.Conditions)

	// If the primitive status is Unknown, add the status to the pod
	if primitiveCond.GetReady() == corev1.ConditionUnknown {
		pod.Status.Conditions = primitiveCond.SetReady(corev1.ConditionFalse)
		return r.client.Status().Update(context.TODO(), pod)
	}

	// Determine the primitive permissions from RBAC
	read, write, err := r.getPermissions(pod, primitive)
	if err != nil {
		return err
	}

	address := fmt.Sprintf("%s:%d", pod.Status.PodIP, protocolCond.GetPort())
	log.Info("Dial %s", address)
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Warn("Dial %s failed", address, err)
		return err
	}
	defer conn.Close()
	client := driverapi.NewAgentClient(conn)

	// If the agent status is False, start the agent and update the pod status
	if primitiveCond.GetReady() == corev1.ConditionFalse {
		request := &driverapi.CreateProxyRequest{
			ProxyID: driverapi.ProxyId{
				PrimitiveId: primitiveapi.PrimitiveId{
					Namespace: primitive.Namespace,
					Name:      primitive.Name,
				},
			},
			Options: driverapi.ProxyOptions{
				Read:  read,
				Write: write,
			},
		}
		_, err := client.CreateProxy(context.TODO(), request)
		if err != nil {
			return err
		}
		pod.Status.Conditions = primitiveCond.SetReady(corev1.ConditionTrue)
		err = r.client.Status().Update(context.TODO(), pod)
		if err != nil {
			return err
		}
	} else {
		// TODO: Handle primitive deletes
	}
	return nil
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
func NewPrimitiveConditions(primitive string, conditions []corev1.PodCondition) PrimitiveConditions {
	return PrimitiveConditions{
		Primitive:  primitive,
		Conditions: conditions,
	}
}

// PrimitiveConditions provides utility functions for driver conditions
type PrimitiveConditions struct {
	Primitive  string
	Conditions []corev1.PodCondition
}

func (d PrimitiveConditions) getConditionType(name string) corev1.PodConditionType {
	return corev1.PodConditionType(fmt.Sprintf(primitiveConditionFormat, d.Primitive, name))
}

func (d PrimitiveConditions) getConditionStatus(name string) corev1.ConditionStatus {
	for _, condition := range d.Conditions {
		if condition.Type == d.getConditionType(name) {
			return condition.Status
		}
	}
	return corev1.ConditionUnknown
}

func (d PrimitiveConditions) setCondition(condition corev1.PodCondition) []corev1.PodCondition {
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

func (d PrimitiveConditions) GetReady() corev1.ConditionStatus {
	return d.getConditionStatus(primitiveReadyCondition)
}

func (d PrimitiveConditions) SetReady(status corev1.ConditionStatus) []corev1.PodCondition {
	return d.setCondition(corev1.PodCondition{
		Type:               d.getConditionType(primitiveReadyCondition),
		Status:             status,
		LastTransitionTime: metav1.Now(),
	})
}
