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

package member

import (
	"context"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("member_controller")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, eventCh chan<- types.NamespacedName) error {
	r := &Reconciler{
		client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		config:  mgr.GetConfig(),
		eventCh: eventCh,
	}

	// Create a new controller
	c, err := controller.New("member-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Member
	err = c.Watch(&source.Kind{Type: &v1beta3.Member{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles a PartitionGroup object
type Reconciler struct {
	client  client.Client
	scheme  *runtime.Scheme
	config  *rest.Config
	eventCh chan<- types.NamespacedName
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Member")

	// Fetch the Member instance
	member := &v1beta3.Member{}
	err := r.client.Get(context.TODO(), request.NamespacedName, member)
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

	go func() {
		r.eventCh <- types.NamespacedName{
			Namespace: member.Namespace,
			Name:      member.Scope,
		}
	}()

	if member.DeletionTimestamp == nil {
		pod := &corev1.Pod{}
		podName := types.NamespacedName{
			Namespace: member.Namespace,
			Name:      member.Service,
		}
		err = r.client.Get(context.TODO(), podName, pod)
		if err != nil {
			if !errors.IsNotFound(err) {
				return reconcile.Result{}, err
			}
			err = r.client.Delete(context.TODO(), member)
			return reconcile.Result{}, err
		}

		if len(member.OwnerReferences) == 0 {
			owner := metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       pod.ObjectMeta.Name,
				UID:        pod.ObjectMeta.UID,
			}
			member.ObjectMeta.OwnerReferences = []metav1.OwnerReference{owner}
			err = r.client.Update(context.TODO(), member)
			return reconcile.Result{}, err
		}
		addFinalizer := true
		for _, finalizer := range member.Finalizers {
			if finalizer == "member-controller" {
				addFinalizer = false
				break
			}
		}
		if addFinalizer {
			member.Finalizers = append(member.Finalizers, "member-controller")
			err = r.client.Update(context.TODO(), member)
			return reconcile.Result{}, err
		}
	} else {
		finalize := false
		for _, finalizer := range member.Finalizers {
			if finalizer == "member-controller" {
				finalize = true
				break
			}
		}
		if finalize {
			finalizers := make([]string, 0, len(member.Finalizers)-1)
			for _, finalizer := range member.Finalizers {
				if finalizer != "member-controller" {
					finalizers = append(finalizers, finalizer)
				}
			}
			member.Finalizers = finalizers
			err = r.client.Update(context.TODO(), member)
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
