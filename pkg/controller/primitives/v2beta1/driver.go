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
	"encoding/json"
	"fmt"
	"github.com/atomix/api/go/atomix/management/broker"
	"github.com/atomix/kubernetes-controller/pkg/apis/storage/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strconv"
	"strings"
)

const baseDriverPort int32 = 55680

// DriverWebhook is a mutating webhook for injecting the broker container into pods
type DriverWebhook struct {
	client            client.Client
	scheme            *runtime.Scheme
	decoder           *admission.Decoder
	storageType       string
	storageAPIVersion string
	storageKind       string
	storageController StorageController
}

// InjectDecoder :
func (w *DriverWebhook) InjectDecoder(decoder *admission.Decoder) error {
	w.decoder = decoder
	return nil
}

// Handle :
func (w *DriverWebhook) Handle(ctx context.Context, request admission.Request) admission.Response {
	podNamespacedName := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	log.Infof("Received admission request for Pod '%s'", podNamespacedName)

	// Decode the pod
	pod := &corev1.Pod{}
	if err := w.decoder.Decode(request, pod); err != nil {
		log.Errorf("Could not decode Pod '%s'", podNamespacedName, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Determine whether the pod supports Atomix primitives by looking for the broker inject annotation
	injectBroker, ok := pod.Annotations[brokerInjectAnnotation]
	if !ok {
		log.Infof("Skipping storage driver injection for Pod '%s'", podNamespacedName)
		return admission.Allowed(fmt.Sprintf("'%s' annotation not found", brokerInjectAnnotation))
	}
	if inject, err := strconv.ParseBool(injectBroker); err != nil {
		log.Errorf("Could not read annotation '%s' for Pod '%s'", brokerInjectAnnotation, podNamespacedName, err)
		return admission.Allowed(fmt.Sprintf("'%s' annotation could not be parsed", brokerInjectAnnotation))
	} else if !inject {
		log.Infof("Skipping storage driver injection for Pod '%s'", podNamespacedName)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is false", brokerInjectAnnotation))
	}

	// If the broker inject annotation is present, determine whether the storage instances have already been injected
	storageStatusAnnotation := getStorageStatusAnnotation(w.storageType)
	storageStatus, ok := pod.Annotations[storageStatusAnnotation]
	injectedDrivers := make(map[string]bool)
	if ok {
		for _, injectedDriver := range strings.Split(storageStatus, ",") {
			if injectedDriver != "" {
				injectedDrivers[injectedDriver] = true
			}
		}
	}

	var storageNames []string

	// Determine whether the pod is annotated with a profile depending on this storage plugin
	injectProfile, ok := pod.Annotations[profileAnnotation]
	if ok && injectProfile != "" {
		profile := &v2beta1.StorageProfile{}
		profileNamespacedName := types.NamespacedName{
			Namespace: request.Namespace,
			Name:      injectProfile,
		}
		if err := w.client.Get(ctx, profileNamespacedName, profile); err != nil {
			if errors.IsNotFound(err) {
				log.Errorf("Could not find annotated StorageProfile '%s' for Pod '%s'", profileNamespacedName, podNamespacedName, err)
				return admission.Denied(fmt.Sprintf("annotated profile '%s' not found", profileNamespacedName))
			}
			log.Error(err)
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// Search the profile for storage instances matching this storage plugin
		for _, storage := range profile.Storage {
			if storage.APIVersion == w.storageAPIVersion && storage.Kind == w.storageKind {
				storageNames = append(storageNames, storage.Name)
			}
		}
	}

	// Determine whether the pod is annotated for storage injection
	injectStorageAnnotation := getStorageInjectAnnotation(w.storageType)
	injectStorage, ok := pod.Annotations[injectStorageAnnotation]
	if ok {
		if inject, err := strconv.ParseBool(injectStorage); err != nil {
			log.Errorf("Could not read annotation '%s' for Pod '%s'", injectStorageAnnotation, podNamespacedName, err)
		} else if inject {
			injectInstancesAnnotation := getStorageInstancesAnnotation(w.storageType)
			injectInstances, ok := pod.Annotations[injectInstancesAnnotation]
			if ok {
				for _, instance := range strings.Split(injectInstances, ",") {
					if instance != "" {
						storageNames = append(storageNames, instance)
					}
				}
			}
		}
	}

	// If there are no storage instances to inject, skip injection
	if len(storageNames) == 0 {
		log.Infof("Skipping storage driver injection for Pod '%s'", podNamespacedName)
		return admission.Allowed("no storage instances found")
	}

	// Add a readiness condition to the od status
	driverReadyCondition := getStorageReadyCondition(w.storageType)
	log.Infof("Adding readiness gate '%s' to Pod '%s'", driverReadyCondition, podNamespacedName)
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
		ConditionType: driverReadyCondition,
	})

	// Search the pod annotations for inject annotations for this driver type
	var port int32 = 55680
	for _, storageName := range storageNames {
		if injectedDrivers[storageName] {
			continue
		}

		driverNamespacedName := types.NamespacedName{
			Namespace: request.Namespace,
			Name:      storageName,
		}

		driver := broker.DriverId{
			Type:      w.storageType,
			Namespace: request.Namespace,
			Name:      storageName,
		}

		log.Infof("Adding storage driver '%s' to Pod '%s'", driverNamespacedName, podNamespacedName)

		// Add the port to the pod in a driver annotation
		portAnnotation := getDriverPortAnnotation(driver)
		pod.Annotations[portAnnotation] = fmt.Sprint(port)

		// Create a driver configuration
		config := broker.DriverConfig{
			ID:   driver,
			Host: "127.0.0.1",
			Port: port,
		}

		// Create a new container via the controller
		container, err := w.storageController.NewContainer(config)
		if err != nil {
			log.Errorf("Injection of storage driver '%s' failed for Pod '%s'", driverNamespacedName, podNamespacedName, err)
			return admission.Errored(http.StatusInternalServerError, err)
		}

		// Add the container to the pod spec
		pod.Spec.Containers = append(pod.Spec.Containers, container)

		// Add the driver status annotation
		driverStatusAnnotation := getDriverStatusAnnotation(driver)
		pod.Annotations[driverStatusAnnotation] = injectedStatus

		// Increment the driver port
		port++

		injectedDrivers[storageName] = true
	}

	// Add the injected status annotation
	injectedDriverNames := make([]string, 0, len(injectedDrivers))
	for injectedDriver := range injectedDrivers {
		injectedDriverNames = append(injectedDriverNames, injectedDriver)
	}
	pod.Annotations[storageStatusAnnotation] = strings.Join(injectedDriverNames, ",")

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Primitive injection failed for Pod '%s'", podNamespacedName, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

var _ admission.Handler = &DriverWebhook{}
