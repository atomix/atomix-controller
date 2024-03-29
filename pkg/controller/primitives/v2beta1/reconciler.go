// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	"context"
	v2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PrimitiveReconciler is a reconciler for Primitive resources
type PrimitiveReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	kind   schema.GroupVersionKind
}

// Reconcile reads that state of the cluster for a primitive object and makes changes based on the pod's annotations
func (r *PrimitiveReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling %s '%s'", r.kind.Kind, request.NamespacedName)

	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(r.kind)
	err := r.client.Get(context.TODO(), request.NamespacedName, object)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Errorf("Reconciling %s '%s' failed", r.kind.Kind, request.NamespacedName, err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	primitive := &v2beta1.Primitive{}
	err = r.client.Get(context.TODO(), request.NamespacedName, primitive)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Errorf("Reconciling %s '%s' failed", r.kind.Kind, request.NamespacedName, err)
			return reconcile.Result{}, err
		}

		log.Infof("Creating Primitive '%s'", request.NamespacedName)

		var reference corev1.ObjectReference
		if name, ok, err := unstructured.NestedString(object.UnstructuredContent(), "spec", "store", "name"); ok {
			reference.Name = name
		} else if err != nil {
			log.Errorf("Reconciling %s '%s' failed", r.kind.Kind, request.NamespacedName, err)
			return reconcile.Result{}, err
		}
		if namespace, ok, err := unstructured.NestedString(object.UnstructuredContent(), "spec", "store", "namespace"); ok {
			reference.Namespace = namespace
		} else if err != nil {
			log.Errorf("Reconciling %s '%s' failed", r.kind.Kind, request.NamespacedName, err)
			return reconcile.Result{}, err
		}

		config := make(map[string]runtime.RawExtension)
		if configs, ok, err := unstructured.NestedMap(object.UnstructuredContent(), "spec", "store", "config"); err != nil {
			log.Errorf("Reconciling %s '%s' failed", r.kind.Kind, request.NamespacedName, err)
			return reconcile.Result{}, err
		} else if ok {
			for key, value := range configs {
				bytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &unstructured.Unstructured{Object: value.(map[string]interface{})})
				if err != nil {
					log.Errorf("Reconciling %s '%s' failed", r.kind.Kind, request.NamespacedName, err)
					return reconcile.Result{}, err
				}
				config[key] = runtime.RawExtension{
					Raw: bytes,
				}
			}
		}

		primitive = &v2beta1.Primitive{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   request.Namespace,
				Name:        request.Name,
				Labels:      object.GetLabels(),
				Annotations: object.GetAnnotations(),
			},
			Spec: v2beta1.PrimitiveSpec{
				Type: r.kind.Kind,
				Store: v2beta1.PrimitiveStore{
					ObjectReference: reference,
					Config:          config,
				},
			},
		}
		if err := controllerutil.SetControllerReference(object, primitive, r.scheme); err != nil {
			log.Errorf("Creating Primitive '%s' failed", request.NamespacedName, err)
			return reconcile.Result{}, err
		}
		if err := r.client.Create(context.TODO(), primitive); err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				log.Errorf("Creating Primitive '%s' failed", request.NamespacedName, err)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

var _ reconcile.Reconciler = &PrimitiveReconciler{}
