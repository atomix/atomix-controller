// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package v2beta1

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strconv"
)

const (
	brokerNodeEnv      = "ATOMIX_BROKER_NODE"
	brokerNamespaceEnv = "ATOMIX_BROKER_NAMESPACE"
	brokerNameEnv      = "ATOMIX_BROKER_NAME"
)

const (
	brokerInjectPath             = "/inject-broker"
	brokerInjectAnnotation       = "broker.atomix.io/inject"
	brokerInjectStatusAnnotation = "broker.atomix.io/status"
	atomixReadyCondition         = "AtomixReady"
	injectedStatus               = "injected"
)

const (
	defaultBrokerImageEnv = "DEFAULT_BROKER_IMAGE"
	defaultBrokerImage    = "atomix/atomix-broker:latest"
)

func getDefaultBrokerImage() string {
	image := os.Getenv(defaultBrokerImageEnv)
	if image == "" {
		image = defaultBrokerImage
	}
	return image
}

func addBrokerController(mgr manager.Manager) error {
	mgr.GetWebhookServer().Register(brokerInjectPath, &webhook.Admission{
		Handler: &BrokerInjector{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
		},
	})
	return nil
}

// BrokerInjector is a mutating webhook that injects the broker container into pods
type BrokerInjector struct {
	client  client.Client
	scheme  *runtime.Scheme
	decoder *admission.Decoder
}

// InjectDecoder :
func (i *BrokerInjector) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

// Handle :
func (i *BrokerInjector) Handle(ctx context.Context, request admission.Request) admission.Response {
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

	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name:            "atomix-broker",
		Image:           getDefaultBrokerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env: []corev1.EnvVar{
			{
				Name: brokerNamespaceEnv,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name: brokerNameEnv,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: brokerNodeEnv,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
	})
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
		ConditionType: atomixReadyCondition,
	})
	pod.Annotations[brokerInjectStatusAnnotation] = injectedStatus

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Broker injection failed for Pod '%s'", podNamespacedName, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

var _ admission.Handler = &BrokerInjector{}
