// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	"context"
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	sidecarv2beta1 "github.com/atomix/atomix-controller/pkg/apis/sidecar/v2beta1"
	"github.com/atomix/atomix-controller/pkg/controller/util/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const primitiveFinalizer = "proxy"

func addPrimitiveController(mgr manager.Manager) error {
	// Create a new controller
	options := controller.Options{
		Reconciler: &PrimitiveReconciler{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
			config: mgr.GetConfig(),
		},
	}
	controller, err := controller.New("primitive-controller", mgr, options)
	if err != nil {
		return err
	}

	// Watch for changes to Primitives
	err = controller.Watch(&source.Kind{Type: &corev2beta1.Primitive{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

// PrimitiveReconciler is a Reconciler for Primitives
type PrimitiveReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reconciles Primitive resources
func (r *PrimitiveReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Primitive '%s'", request.NamespacedName)
	primitive := &corev2beta1.Primitive{}
	err := r.client.Get(context.TODO(), request.NamespacedName, primitive)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err)
		return reconcile.Result{}, err
	}

	if primitive.DeletionTimestamp == nil {
		if !k8s.HasFinalizer(primitive.Finalizers, primitiveFinalizer) {
			log.Infof("Adding finalizer to Primitive %s", request.NamespacedName)
			primitive.Finalizers = k8s.AddFinalizer(primitive.Finalizers, primitiveFinalizer)
			if err := r.client.Update(context.TODO(), primitive); err != nil {
				log.Error(err)
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
	} else {
		if !k8s.HasFinalizer(primitive.Finalizers, primitiveFinalizer) {
			return reconcile.Result{}, nil
		}

		log.Infof("Deleting Proxy's for Primitive %s", request.NamespacedName)
		options := &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				Namespace: primitive.Namespace,
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"primitive": string(primitive.UID),
				}),
			},
		}
		if err := r.client.DeleteAllOf(context.TODO(), &sidecarv2beta1.Proxy{}, options); err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return reconcile.Result{}, err
			}
		}

		log.Infof("Removing finalizer from Primitive %s", request.NamespacedName)
		primitive.Finalizers = k8s.RemoveFinalizer(primitive.Finalizers, primitiveFinalizer)
		if err := r.client.Update(context.TODO(), primitive); err != nil {
			log.Error(err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}
