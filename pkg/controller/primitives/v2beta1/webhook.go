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

const (
	injectAnnotation        = "broker.atomix.io/inject"
	injectStatusAnnotation  = "broker.atomix.io/inject-status"
	containerNameAnnotation = "broker.atomix.io/container-name"
	injectedStatus          = "injected"
	defaultContainerName    = "atomix-broker"
)

const (
	defaultBrokerImageEnv = "DEFAULT_BROKER_IMAGE"
	defaultBrokerImage    = "atomix/kubernetes-broker:latest"
)

func getDefaultBrokerImage() string {
	image := os.Getenv(defaultBrokerImageEnv)
	if image == "" {
		image = defaultBrokerImage
	}
	return image
}

// RegisterWebhooks registers admission webhooks on the given manager
func RegisterWebhooks(mgr manager.Manager) error {
	mgr.GetWebhookServer().Register(k8s.GetWebhookPath(), &webhook.Admission{
		Handler: &BrokerInjector{
			client: mgr.GetClient(),
		},
	})
	return nil
}

// BrokerInjector is a mutating webhook for injecting the broker container into pods
type BrokerInjector struct {
	client  client.Client
	decoder *admission.Decoder
}

// InjectDecoder :
func (i *BrokerInjector) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

// Handle :
func (i *BrokerInjector) Handle(ctx context.Context, request admission.Request) admission.Response {
	log.Infof("Received admission request for Pod '%s/%s'", request.Name, request.Namespace)

	// Decode the pod
	pod := &corev1.Pod{}
	if err := i.decoder.Decode(request, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	injectBroker, ok := pod.Annotations[injectAnnotation]
	if !ok {
		log.Debugf("Skipping broker injection for Pod '%s/%s': '%s' annotation not found", pod.Name, pod.Namespace, injectAnnotation)
		return admission.Allowed(fmt.Sprintf("'%s' annotation not found", injectAnnotation))
	}
	if inject, err := strconv.ParseBool(injectBroker); err != nil {
		log.Debugf("Skipping broker injection for Pod '%s/%s': '%s' annotation could not be parsed (%v)", pod.Name, pod.Namespace, injectAnnotation, err)
		return admission.Allowed(fmt.Sprintf("'%s' annotation could not be parsed", injectAnnotation))
	} else if !inject {
		log.Debugf("Skipping broker injection for Pod '%s/%s': '%s' is false", pod.Name, pod.Namespace, injectAnnotation)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is false", injectAnnotation))
	}

	injectedBroker, ok := pod.Annotations[injectStatusAnnotation]
	if ok && injectedBroker == injectedStatus {
		log.Debugf("Skipping broker injection for Pod '%s/%s': '%s' is '%s'", pod.Name, pod.Namespace, injectStatusAnnotation, injectedBroker)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is '%s'", injectStatusAnnotation, injectedBroker))
	}

	container := corev1.Container{
		Name:            defaultContainerName,
		Image:           getDefaultBrokerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
	}
	pod.Spec.Containers = append(pod.Spec.Containers, container)
	pod.Annotations[injectStatusAnnotation] = injectedStatus
	pod.Annotations[containerNameAnnotation] = defaultContainerName

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Failed to inject models into Pod '%s/%s': %s", pod.Name, pod.Namespace, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

var _ admission.Handler = &BrokerInjector{}
