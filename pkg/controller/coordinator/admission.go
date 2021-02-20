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

package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/atomix/go-framework/pkg/atomix/logging"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strconv"
)

var log = logging.GetLogger("atomix", "controller", "coordinator")

const (
	CoordinatorInjectAnnotation       = "storage.atomix.io/coordinator-inject"
	CoordinatorInjectStatusAnnotation = "storage.atomix.io/coordinator-inject-status"
	coordinatorInjectedStatus         = "injected"
)

const (
	defaultCoordinatorImageEnv = "DEFAULT_COORDINATOR_IMAGE"
	defaultCoordinatorImage    = "atomix/kubernetes-coordinator:latest"
)

func getDefaultCoordinatorImage() string {
	image := os.Getenv(defaultCoordinatorImageEnv)
	if image == "" {
		image = defaultCoordinatorImage
	}
	return image
}

// RegisterWebhooks registes admission webhooks on the given manager
func RegisterWebhooks(mgr manager.Manager) error {
	mgr.GetWebhookServer().Register(k8s.GetWebhookPath(), &webhook.Admission{
		Handler: &CoordinatorInjector{
			client: mgr.GetClient(),
		},
	})
	return nil
}

// CoordinatorInjector is a mutating webhook for injecting the coordinator container into pods
type CoordinatorInjector struct {
	client  client.Client
	decoder *admission.Decoder
}

// InjectDecoder :
func (i *CoordinatorInjector) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

// Handle :
func (i *CoordinatorInjector) Handle(ctx context.Context, request admission.Request) admission.Response {
	log.Infof("Received admission request for Pod '%s/%s'", request.Name, request.Namespace)

	// Decode the pod
	pod := &corev1.Pod{}
	if err := i.decoder.Decode(request, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	injectCoordinator, ok := pod.Annotations[CoordinatorInjectAnnotation]
	if !ok {
		log.Debugf("Skipping coordinator injection for Pod '%s/%s': '%s' annotation not found", pod.Name, pod.Namespace, CoordinatorInjectAnnotation)
		return admission.Allowed(fmt.Sprintf("'%s' annotation not found", CoordinatorInjectAnnotation))
	}
	if inject, err := strconv.ParseBool(injectCoordinator); err != nil {
		log.Debugf("Skipping coordinator injection for Pod '%s/%s': '%s' annotation could not be parsed (%v)", pod.Name, pod.Namespace, CoordinatorInjectAnnotation, err)
		return admission.Allowed(fmt.Sprintf("'%s' annotation could not be parsed", CoordinatorInjectAnnotation))
	} else if !inject {
		log.Debugf("Skipping coordinator injection for Pod '%s/%s': '%s' is false", pod.Name, pod.Namespace, CoordinatorInjectAnnotation)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is false", CoordinatorInjectAnnotation))
	}

	injectedCoordinator, ok := pod.Annotations[CoordinatorInjectStatusAnnotation]
	if ok && injectedCoordinator == coordinatorInjectedStatus {
		log.Debugf("Skipping coordinator injection for Pod '%s/%s': '%s' is '%s'", pod.Name, pod.Namespace, CoordinatorInjectStatusAnnotation, injectedCoordinator)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is '%s'", CoordinatorInjectStatusAnnotation, injectedCoordinator))
	}

	container := corev1.Container{
		Name:  "atomix-coordinator",
		Image: getDefaultCoordinatorImage(),
	}
	pod.Spec.Containers = append(pod.Spec.Containers, container)
	pod.Annotations[CoordinatorInjectStatusAnnotation] = coordinatorInjectedStatus

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Failed to inject models into Pod '%s/%s': %s", pod.Name, pod.Namespace, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

var _ admission.Handler = &CoordinatorInjector{}
