// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	"context"
	"fmt"
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	profileInjectPath = "/inject-profile"
)

const (
	storageProfileAnnotation       = "storage.atomix.io/profile"
	storageProfileStatusAnnotation = "profile.storage.atomix.io/status"
)

func addProfileController(mgr manager.Manager) error {
	mgr.GetWebhookServer().Register(profileInjectPath, &webhook.Admission{
		Handler: &ProfileInjector{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
		},
	})
	return nil
}

// ProfileInjector is a mutating webhook that injects profile drivers into pods
type ProfileInjector struct {
	client  client.Client
	scheme  *runtime.Scheme
	decoder *admission.Decoder
}

// InjectDecoder :
func (i *ProfileInjector) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

// Handle :
func (i *ProfileInjector) Handle(ctx context.Context, request admission.Request) admission.Response {
	namespacedName := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	log.Infof("Received admission request for Pod '%s'", namespacedName)

	// Decode the pod
	pod := &corev1.Pod{}
	if err := i.decoder.Decode(request, pod); err != nil {
		log.Errorf("Could not decode Pod '%s'", namespacedName, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	profileInjectStatus := pod.Annotations[storageProfileStatusAnnotation]
	if profileInjectStatus == injectedStatus {
		return admission.Allowed("Profile drivers already injected")
	}

	profileName, ok := pod.Annotations[storageProfileAnnotation]
	if !ok {
		return admission.Allowed("No profile to inject")
	}

	profile := &v2beta1.StorageProfile{}
	profileNamespacedName := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      profileName,
	}
	if err := i.client.Get(ctx, profileNamespacedName, profile); err != nil {
		if errors.IsNotFound(err) {
			return admission.Denied(fmt.Sprintf("StorageProfile %s not found", profileNamespacedName))
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}

	for _, driver := range profile.Spec.Drivers {
		driverInjectAnnotation := fmt.Sprintf("%s/inject", driver)
		if _, ok := pod.Annotations[driverInjectAnnotation]; !ok {
			pod.Annotations[driverInjectAnnotation] = fmt.Sprint(true)
		}
	}

	pod.Annotations[storageProfileStatusAnnotation] = injectedStatus

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Profile injection failed for Pod '%s'", namespacedName, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

var _ admission.Handler = &ProfileInjector{}
