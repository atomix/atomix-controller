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
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	sidecarv2beta1 "github.com/atomix/atomix-controller/pkg/apis/sidecar/v2beta1"
	"github.com/atomix/atomix-controller/pkg/controller/util/k8s"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const proxyFinalizer = "proxy"

func addProxyController(mgr manager.Manager) error {
	r := &AgentReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("proxy-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Proxy's
	err = c.Watch(&source.Kind{Type: &sidecarv2beta1.Proxy{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Primitives
	err = c.Watch(&source.Kind{Type: &corev2beta1.Primitive{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newPrimitiveProxyMapper(mgr),
	})
	if err != nil {
		return err
	}

	// Watch for changes to ProtocolAgents
	err = c.Watch(&source.Kind{Type: &sidecarv2beta1.Agent{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newAgentProxyMapper(mgr),
	})
	if err != nil {
		return err
	}
	return nil
}

// ProxyReconciler is a Reconciler for Proxy resources
type ProxyReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

func (r *ProxyReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Proxy '%s'", request.NamespacedName)
	proxy := &sidecarv2beta1.Proxy{}
	err := r.client.Get(context.TODO(), request.NamespacedName, proxy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if proxy.DeletionTimestamp == nil {
		return r.reconcileCreate(proxy)
	} else {
		return r.reconcileDelete(proxy)
	}
}

func (r *ProxyReconciler) reconcileCreate(proxy *sidecarv2beta1.Proxy) (reconcile.Result, error) {
	if !k8s.HasFinalizer(proxy.Finalizers, proxyFinalizer) {
		return reconcile.Result{}, r.addFinalizer(proxy)
	}

	if proxy.Status.Ready {
		return reconcile.Result{}, nil
	}

	pod, err := r.getPod(proxy)
	if err != nil {
		return reconcile.Result{}, err
	} else if pod == nil {
		return reconcile.Result{}, r.client.Delete(context.TODO(), proxy)
	}

	primitive, err := r.getPrimitive(proxy)
	if err != nil {
		return reconcile.Result{}, err
	} else if primitive == nil {
		return reconcile.Result{}, r.client.Delete(context.TODO(), proxy)
	}

	agent, err := r.getAgent(proxy)
	if err != nil {
		return reconcile.Result{}, err
	} else if agent == nil {
		return reconcile.Result{}, r.client.Delete(context.TODO(), proxy)
	}

	if !agent.Status.Ready {
		return reconcile.Result{}, nil
	}

	log.Info("Connecting to agent")
	agentConn, err := grpc.Dial(fmt.Sprintf("%s:%d", pod.Status.PodIP, agent.Spec.Port), grpc.WithInsecure())
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	agentClient := driverapi.NewAgentClient(agentConn)

	log.Info("Creating primitive proxy")
	createProxyRequest := &driverapi.CreateProxyRequest{
		ProxyID: driverapi.ProxyId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
		Options: driverapi.ProxyOptions{
			Read:  proxy.Spec.Permissions.Read,
			Write: proxy.Spec.Permissions.Write,
		},
	}
	_, err = agentClient.CreateProxy(context.TODO(), createProxyRequest)
	if err != nil && status.Code(err) != codes.AlreadyExists {
		log.Error(err, "Creating primitive proxy")
		return reconcile.Result{}, err
	}

	log.Info("Connecting to broker")
	brokerConn, err := grpc.Dial(fmt.Sprintf("%s:5678", pod.Status.PodIP), grpc.WithInsecure())
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	brokerClient := brokerapi.NewBrokerClient(brokerConn)

	log.Info("Registering primitive with broker")
	registerPrimitiveRequest := &brokerapi.RegisterPrimitiveRequest{
		PrimitiveID: brokerapi.PrimitiveId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
		Address: brokerapi.PrimitiveAddress{
			Host: "127.0.0.1",
			Port: agent.Spec.Port,
		},
	}
	_, err = brokerClient.RegisterPrimitive(context.TODO(), registerPrimitiveRequest)
	if err != nil && status.Code(err) != codes.AlreadyExists {
		log.Error(err, "Registering primitive with broker")
		return reconcile.Result{}, err
	}

	proxy.Status.Ready = true
	if err := r.client.Status().Update(context.TODO(), proxy); err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ProxyReconciler) reconcileDelete(proxy *sidecarv2beta1.Proxy) (reconcile.Result, error) {
	if !k8s.HasFinalizer(proxy.Finalizers, proxyFinalizer) {
		return reconcile.Result{}, nil
	}

	pod, err := r.getPod(proxy)
	if err != nil {
		return reconcile.Result{}, err
	} else if pod == nil {
		return reconcile.Result{}, r.removeFinalizer(proxy)
	}

	primitive, err := r.getPrimitive(proxy)
	if err != nil {
		return reconcile.Result{}, err
	} else if primitive == nil {
		return reconcile.Result{}, r.removeFinalizer(proxy)
	}

	log.Info("Connecting to broker")
	brokerConn, err := grpc.Dial(fmt.Sprintf("%s:5678", pod.Status.PodIP), grpc.WithInsecure())
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	brokerClient := brokerapi.NewBrokerClient(brokerConn)

	log.Info("Unregistering primitive with broker")
	unregisterPrimitiveRequest := &brokerapi.UnregisterPrimitiveRequest{
		PrimitiveID: brokerapi.PrimitiveId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
	}
	_, err = brokerClient.UnregisterPrimitive(context.TODO(), unregisterPrimitiveRequest)
	if err != nil && status.Code(err) != codes.NotFound {
		log.Error(err, "Unregistering primitive with broker")
		return reconcile.Result{}, err
	}

	agent, err := r.getAgent(proxy)
	if err != nil {
		return reconcile.Result{}, err
	} else if agent == nil {
		return reconcile.Result{}, r.removeFinalizer(proxy)
	}

	log.Info("Connecting to agent")
	agentConn, err := grpc.Dial(fmt.Sprintf("%s:%d", pod.Status.PodIP, agent.Spec.Port), grpc.WithInsecure())
	if err != nil {
		log.Error(err)
		return reconcile.Result{}, err
	}
	agentClient := driverapi.NewAgentClient(agentConn)

	log.Info("Destroying primitive proxy")
	destroyProxyRequest := &driverapi.DestroyProxyRequest{
		ProxyID: driverapi.ProxyId{
			PrimitiveId: primitiveapi.PrimitiveId{
				Type:      primitive.Spec.Type,
				Namespace: primitive.Namespace,
				Name:      primitive.Name,
			},
		},
	}
	_, err = agentClient.DestroyProxy(context.TODO(), destroyProxyRequest)
	if err != nil && status.Code(err) != codes.NotFound {
		log.Error(err, "Destroying primitive proxy")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, r.removeFinalizer(proxy)
}

func (r *ProxyReconciler) addFinalizer(proxy *sidecarv2beta1.Proxy) error {
	proxy.Finalizers = k8s.AddFinalizer(proxy.Finalizers, proxyFinalizer)
	if err := r.client.Update(context.TODO(), proxy); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (r *ProxyReconciler) removeFinalizer(proxy *sidecarv2beta1.Proxy) error {
	proxy.Finalizers = k8s.RemoveFinalizer(proxy.Finalizers, proxyFinalizer)
	if err := r.client.Update(context.TODO(), proxy); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

func (r *ProxyReconciler) getPod(proxy *sidecarv2beta1.Proxy) (*corev1.Pod, error) {
	podName := types.NamespacedName{
		Namespace: proxy.Spec.Pod.Namespace,
		Name:      proxy.Spec.Pod.Name,
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

func (r *ProxyReconciler) getPrimitive(proxy *sidecarv2beta1.Proxy) (*corev2beta1.Primitive, error) {
	primitiveName := types.NamespacedName{
		Namespace: proxy.Spec.Primitive.Namespace,
		Name:      proxy.Spec.Primitive.Name,
	}
	primitive := &corev2beta1.Primitive{}
	if err := r.client.Get(context.TODO(), primitiveName, primitive); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return nil, err
		}
		return nil, nil
	}
	return primitive, nil
}

func (r *ProxyReconciler) getAgent(proxy *sidecarv2beta1.Proxy) (*sidecarv2beta1.Agent, error) {
	agentName := types.NamespacedName{
		Namespace: proxy.Spec.Agent.Namespace,
		Name:      proxy.Spec.Agent.Name,
	}
	agent := &sidecarv2beta1.Agent{}
	if err := r.client.Get(context.TODO(), agentName, agent); err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return nil, err
		}
		return nil, nil
	}
	return agent, nil
}

var _ reconcile.Reconciler = &ProxyReconciler{}

func newPrimitiveProxyMapper(mgr manager.Manager) handler.Mapper {
	return &primitiveProxyMapper{
		client: mgr.GetClient(),
	}
}

type primitiveProxyMapper struct {
	client client.Client
}

func (m *primitiveProxyMapper) Map(object handler.MapObject) []reconcile.Request {
	primitive := object.Object.(*corev2beta1.Primitive)
	proxies := &sidecarv2beta1.ProxyList{}
	options := &client.ListOptions{
		Namespace:     primitive.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.primitive.uid", string(primitive.UID)),
	}
	if err := m.client.List(context.TODO(), proxies, options); err != nil {
		log.Error(err)
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, 0, len(proxies.Items))
	for _, proxy := range proxies.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: proxy.Namespace,
				Name:      proxy.Name,
			},
		})
	}
	return requests
}

var _ handler.Mapper = &primitiveProxyMapper{}

func newAgentProxyMapper(mgr manager.Manager) handler.Mapper {
	return &agentProxyMapper{
		client: mgr.GetClient(),
	}
}

type agentProxyMapper struct {
	client client.Client
}

func (m *agentProxyMapper) Map(object handler.MapObject) []reconcile.Request {
	agent := object.Object.(*sidecarv2beta1.Agent)
	proxies := &sidecarv2beta1.ProxyList{}
	options := &client.ListOptions{
		Namespace:     agent.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.agent.uid", string(agent.UID)),
	}
	if err := m.client.List(context.TODO(), proxies, options); err != nil {
		log.Error(err)
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, 0, len(proxies.Items))
	for _, proxy := range proxies.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: proxy.Namespace,
				Name:      proxy.Name,
			},
		})
	}
	return requests
}

var _ handler.Mapper = &agentProxyMapper{}
