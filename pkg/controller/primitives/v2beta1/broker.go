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
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strconv"
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
		Handler: &BrokerWebhook{
			client: mgr.GetClient(),
		},
	})
	return nil
}

// BrokerWebhook is a mutating webhook for injecting the broker container into pods
type BrokerWebhook struct {
	client  client.Client
	decoder *admission.Decoder
}

// InjectDecoder :
func (i *BrokerWebhook) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

// Handle :
func (i *BrokerWebhook) Handle(ctx context.Context, request admission.Request) admission.Response {
	podNamespacedName := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}
	log.Infof("Received admission request for Pod '%s'", podNamespacedName)

	// Decode the pod
	pod := &corev1.Pod{}
	if err := i.decoder.Decode(request, pod); err != nil {
		log.Errorf("Could not decode Pod '%s'", podNamespacedName, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	injectBroker, ok := pod.Annotations[brokerInjectAnnotation]
	if !ok {
		log.Infof("Skipping broker injection for Pod '%s'", podNamespacedName)
		return admission.Allowed(fmt.Sprintf("'%s' annotation not found", brokerInjectAnnotation))
	}
	if inject, err := strconv.ParseBool(injectBroker); err != nil {
		log.Errorf("Broker injection failed for Pod '%s'", podNamespacedName, err)
		return admission.Allowed(fmt.Sprintf("'%s' annotation could not be parsed", brokerInjectAnnotation))
	} else if !inject {
		log.Infof("Skipping broker injection for Pod '%s'", podNamespacedName)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is false", brokerInjectAnnotation))
	}

	injectedBroker, ok := pod.Annotations[brokerInjectStatusAnnotation]
	if ok && injectedBroker == injectedStatus {
		log.Infof("Skipping broker injection for Pod '%s'", podNamespacedName)
		return admission.Allowed(fmt.Sprintf("'%s' annotation is '%s'", brokerInjectStatusAnnotation, injectedBroker))
	}

	container := corev1.Container{
		Name:            "atomix-broker",
		Image:           getDefaultBrokerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
	}
	pod.Spec.Containers = append(pod.Spec.Containers, container)
	pod.Annotations[brokerInjectStatusAnnotation] = injectedStatus

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Broker injection failed for Pod '%s'", podNamespacedName, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

var _ admission.Handler = &BrokerWebhook{}
