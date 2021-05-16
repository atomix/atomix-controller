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
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	sidecarv2beta1 "github.com/atomix/atomix-controller/pkg/apis/sidecar/v2beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
)

const (
	baseProtocolPort = 55680
)

const (
	verbRead  = "read"
	verbWrite = "write"
)

func addPodController(mgr manager.Manager) error {
	r := &PodReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("pod-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Pods
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Agents
	err = c.Watch(&source.Kind{Type: &sidecarv2beta1.Agent{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &corev1.Pod{},
		IsController: true,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Proxy's
	err = c.Watch(&source.Kind{Type: &sidecarv2beta1.Proxy{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &corev1.Pod{},
		IsController: true,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Stores
	err = c.Watch(&source.Kind{Type: &v2beta1.Store{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newStorePodMapper(mgr),
	})
	if err != nil {
		return err
	}

	// Watch for changes to Primitives
	err = c.Watch(&source.Kind{Type: &v2beta1.Primitive{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPrimitivePodMapper(mgr),
	})
	if err != nil {
		return err
	}
	return nil
}

// PodReconciler is a Reconciler for Pod resources
type PodReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

func (r *PodReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Pod '%s'", request.NamespacedName)
	pod := &corev1.Pod{}
	err := r.client.Get(context.TODO(), request.NamespacedName, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if ok, err := r.reconcilePrimitives(pod); err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *PodReconciler) reconcilePrimitives(pod *corev1.Pod) (bool, error) {
	primitives := &v2beta1.PrimitiveList{}
	options := &client.ListOptions{
		Namespace: pod.Namespace,
	}
	err := r.client.List(context.TODO(), primitives, options)
	if err != nil {
		log.Error(err)
		return false, err
	}

	ready := true
	sortedPrimitives := make([]v2beta1.Primitive, len(primitives.Items))
	for i, primitive := range primitives.Items {
		sortedPrimitives[i] = primitive
	}
	sort.Slice(sortedPrimitives, func(i, j int) bool {
		return sortedPrimitives[i].Name < sortedPrimitives[j].Name
	})
	for _, primitive := range sortedPrimitives {
		if ok, err := r.reconcilePrimitive(pod, primitive, &ready); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}

	if ready {
		return r.setAtomixCondition(pod, corev1.ConditionTrue, "", "")
	}
	return false, nil
}

func (r *PodReconciler) reconcilePrimitive(pod *corev1.Pod, primitive v2beta1.Primitive, ready *bool) (bool, error) {
	podName := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Name,
	}
	primitiveName := types.NamespacedName{
		Namespace: primitive.Namespace,
		Name:      primitive.Name,
	}

	log.Infof("Reconciling Primitive %s for Pod %s", primitiveName, podName)
	read, write, err := r.getPermissions(pod, primitive)
	if err != nil {
		log.Error(err)
		return false, err
	}

	proxyName := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      fmt.Sprintf("%s.%s", pod.Name, primitive.UID),
	}
	proxy := &sidecarv2beta1.Proxy{}
	if err := r.client.Get(context.TODO(), proxyName, proxy); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return false, err
		}

		if !read && !write {
			log.Warnf("Pod %s does not have permissions to access Primitive %s", podName, primitiveName)
			return false, nil
		}

		storeName := types.NamespacedName{
			Namespace: primitive.Spec.Store.Namespace,
			Name:      primitive.Spec.Store.Name,
		}
		if storeName.Namespace == "" {
			storeName.Namespace = primitive.Namespace
		}
		store := &v2beta1.Store{}
		if err := r.client.Get(context.TODO(), storeName, store); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return false, err
			}
			return false, nil
		}

		if supported, err := r.isProtocolSupported(pod, *store); err != nil {
			log.Error(err)
			return false, err
		} else if !supported {
			log.Warnf("Pod %s does not have the appropriate drivers for Primitive %s installed", podName, primitiveName)
			return false, nil
		}

		agentName := types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      fmt.Sprintf("%s.%s", pod.Name, store.UID),
		}
		agent := &sidecarv2beta1.Agent{}
		if err := r.client.Get(context.TODO(), agentName, agent); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return false, err
			}

			port, err := r.getPort(pod)
			if err != nil {
				log.Error(err)
				return false, err
			}

			podRef, err := reference.GetReference(r.scheme, pod)
			if err != nil {
				log.Error(err)
				return false, err
			}

			storeRef, err := reference.GetReference(r.scheme, store)
			if err != nil {
				log.Error(err)
				return false, err
			}

			agent = &sidecarv2beta1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentName.Namespace,
					Name:      agentName.Name,
					Labels: map[string]string{
						"pod":   string(podRef.UID),
						"store": string(storeRef.UID),
					},
				},
				Spec: sidecarv2beta1.AgentSpec{
					Port:  int32(port),
					Pod:   *podRef,
					Store: *storeRef,
				},
			}
			if err := controllerutil.SetControllerReference(pod, agent, r.scheme); err != nil {
				log.Error(err)
				return false, err
			}
			log.Infof("Creating Agent %s for Pod %s", agentName, podName)
			if err := r.client.Create(context.TODO(), agent); err != nil {
				if !k8serrors.IsAlreadyExists(err) {
					log.Error(err)
					return false, err
				}
				return false, nil
			}
			return true, nil
		}

		if *ready && !agent.Status.Ready {
			if ok, err := r.setAtomixCondition(pod, corev1.ConditionFalse, "WaitingForAgent", fmt.Sprintf("Waiting for %s agent", agentName)); err != nil {
				return false, err
			} else if ok {
				return true, nil
			}
			*ready = false
		}

		podRef, err := reference.GetReference(r.scheme, pod)
		if err != nil {
			log.Error(err)
			return false, err
		}

		primitiveRef, err := reference.GetReference(r.scheme, &primitive)
		if err != nil {
			log.Error(err)
			return false, err
		}

		agentRef, err := reference.GetReference(r.scheme, agent)
		if err != nil {
			log.Error(err)
			return false, err
		}

		proxy = &sidecarv2beta1.Proxy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: proxyName.Namespace,
				Name:      proxyName.Name,
				Labels: map[string]string{
					"pod":       string(podRef.UID),
					"primitive": string(primitiveRef.UID),
					"agent":     string(agentRef.UID),
				},
			},
			Spec: sidecarv2beta1.ProxySpec{
				Pod:       *podRef,
				Primitive: *primitiveRef,
				Agent:     *agentRef,
				Permissions: sidecarv2beta1.ProxyPermissions{
					Read:  read,
					Write: write,
				},
			},
		}
		if err := controllerutil.SetControllerReference(pod, proxy, r.scheme); err != nil {
			log.Error(err)
			return false, err
		}
		log.Infof("Creating Proxy %s for Pod %s", proxyName, podName)
		if err := r.client.Create(context.TODO(), proxy); err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				log.Error(err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	if *ready && !proxy.Status.Ready {
		if ok, err := r.setAtomixCondition(pod, corev1.ConditionFalse, "WaitingForProxy", fmt.Sprintf("Waiting for %s proxy", proxyName)); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
		*ready = false
	}

	agentName := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      proxy.Spec.Agent.Name,
	}
	agent := &sidecarv2beta1.Agent{}
	if err := r.client.Get(context.TODO(), agentName, agent); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return false, err
		}
		log.Infof("Deleting Proxy %s for Pod %s", proxyName, podName)
		if err := r.client.Delete(context.TODO(), proxy); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	if proxy.Spec.Agent.UID != agent.UID {
		log.Infof("Deleting Proxy %s for Pod %s", proxyName, podName)
		if err := r.client.Delete(context.TODO(), proxy); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}

	if proxy.Spec.Permissions.Read != read || proxy.Spec.Permissions.Write != write {
		log.Infof("Deleting Proxy %s for Pod %s", proxyName, podName)
		if err := r.client.Delete(context.TODO(), proxy); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

func (r *PodReconciler) setAtomixCondition(pod *corev1.Pod, status corev1.ConditionStatus, reason string, message string) (bool, error) {
	for i, condition := range pod.Status.Conditions {
		if condition.Type == atomixReadyCondition {
			if condition.Status == status && condition.Reason == reason && condition.Message == message {
				return false, nil
			}
			log.Infof("Updating Pod %s condition: status=%s, reason=%s, message=%s",
				types.NamespacedName{pod.Namespace, pod.Name}, status, reason, message)
			if condition.Status != status {
				condition.LastTransitionTime = metav1.Now()
			}
			condition.Status = status
			condition.Reason = reason
			condition.Message = message
			pod.Status.Conditions[i] = condition
			if err := r.client.Status().Update(context.TODO(), pod); err != nil {
				log.Error(err)
				return false, err
			}
			return true, nil
		}
	}

	log.Infof("Initializing Pod %s condition: status=%s, reason=%s, message=%s",
		types.NamespacedName{pod.Namespace, pod.Name}, status, reason, message)
	pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{
		Type:               atomixReadyCondition,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	if err := r.client.Status().Update(context.TODO(), pod); err != nil {
		return false, err
	}
	return true, nil
}

func (r *PodReconciler) getPort(pod *corev1.Pod) (int, error) {
	agents := &sidecarv2beta1.AgentList{}
	options := &client.ListOptions{
		Namespace: pod.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"pod": string(pod.UID),
		}),
	}
	if err := r.client.List(context.TODO(), agents, options); err != nil {
		return 0, err
	}

	port := baseProtocolPort
	for _, agent := range agents.Items {
		if int(agent.Spec.Port) > port {
			port = int(agent.Spec.Port) + 1
		}
	}
	return port, nil
}

func (r *PodReconciler) isProtocolSupported(pod *corev1.Pod, store v2beta1.Store) (bool, error) {
	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, store.Spec.Protocol.Raw)
	if err != nil {
		log.Error(err)
		return false, err
	}

	gvc := object.GetObjectKind().GroupVersionKind()

	plugins := &v2beta1.StoragePluginList{}
	err = r.client.List(context.TODO(), plugins)
	if err != nil {
		log.Error(err)
		return false, err
	}

	for _, plugin := range plugins.Items {
		if plugin.Spec.Group == gvc.Group && plugin.Spec.Kind == gvc.Kind {
			for _, version := range plugin.Spec.Versions {
				if version.Name == gvc.Version {
					statusAnnotation := fmt.Sprintf("%s.%s/status", version.Name, plugin.Name)
					return pod.Annotations[statusAnnotation] == injectedStatus, nil
				}
			}
			log.Errorf("Could not find plugin for %s", gvc)
			return false, nil
		}
	}
	log.Errorf("Could not find plugin for %s", gvc)
	return false, nil
}

func (r *PodReconciler) getPermissions(pod *corev1.Pod, primitive v2beta1.Primitive) (read bool, write bool, err error) {
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
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return false, false, err
		}
		return false, false, nil
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
			if subject.Kind == rbacv1.ServiceAccountKind && subject.Name == pod.Spec.ServiceAccountName {
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

var _ reconcile.Reconciler = &PodReconciler{}

func newStorePodMapper(mgr manager.Manager) handler.Mapper {
	return &storePodMapper{
		client: mgr.GetClient(),
	}
}

type storePodMapper struct {
	client client.Client
}

func (m *storePodMapper) Map(object handler.MapObject) []reconcile.Request {
	pods := &corev1.PodList{}
	if err := m.client.List(context.TODO(), pods, &client.ListOptions{}); err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(pods.Items))
	for _, pod := range pods.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
		})
	}
	return requests
}

var _ handler.Mapper = &storePodMapper{}

func newPrimitivePodMapper(mgr manager.Manager) handler.Mapper {
	return &primitivePodMapper{
		client: mgr.GetClient(),
	}
}

type primitivePodMapper struct {
	client client.Client
}

func (m *primitivePodMapper) Map(object handler.MapObject) []reconcile.Request {
	pods := &corev1.PodList{}
	if err := m.client.List(context.TODO(), pods, &client.ListOptions{Namespace: object.Meta.GetNamespace()}); err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(pods.Items))
	for _, pod := range pods.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
		})
	}
	return requests
}

var _ handler.Mapper = &primitivePodMapper{}
