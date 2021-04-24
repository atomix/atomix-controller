// Copyright 2020-present Open Networking Foundation.
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
	"github.com/atomix/kubernetes-controller/pkg/apis/core/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strconv"
	"strings"
)

const (
	driverTypeEnv      = "ATOMIX_DRIVER_TYPE"
	driverNodeEnv      = "ATOMIX_DRIVER_NODE"
	driverNamespaceEnv = "ATOMIX_DRIVER_NAMESPACE"
	driverNameEnv      = "ATOMIX_DRIVER_NAME"
	driverPortEnv      = "ATOMIX_DRIVER_PORT"
)

const (
	driverInjectPath = "/inject-drivers"
)

func addDriverController(mgr manager.Manager) error {
	mgr.GetWebhookServer().Register(driverInjectPath, &webhook.Admission{
		Handler: &DriverInjector{
			client: mgr.GetClient(),
			scheme: mgr.GetScheme(),
		},
	})
	return nil
}

type DriverInjector struct {
	client  client.Client
	scheme  *runtime.Scheme
	decoder *admission.Decoder
}

func (i *DriverInjector) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

func (i *DriverInjector) Handle(ctx context.Context, request admission.Request) admission.Response {
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

	pluginList := &v2beta1.StoragePluginList{}
	if err := i.client.List(ctx, pluginList); err != nil {
		log.Error("Could not list StoragePlugins", err)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	plugins := make(map[string]v2beta1.StoragePlugin)
	for _, plugin := range pluginList.Items {
		plugins[plugin.Name] = plugin
	}

	for annotation, value := range pod.Annotations {
		parts := strings.Split(annotation, "/")
		if len(parts) != 2 {
			continue
		}

		domain, path := parts[0], parts[1]
		if path != "inject" {
			continue
		}

		if value != fmt.Sprint(true) {
			continue
		}

		plugin, ok := plugins[domain]
		if !ok {
			continue
		}

		for _, version := range plugin.Spec.Versions {
			driverQualifiedName := fmt.Sprintf("%s.%s", version.Name, plugin.Name)
			pluginName := plugin.Name[len(plugin.Name)-len(plugin.Spec.Group):]
			statusAnnotation := fmt.Sprintf("%s/status", driverQualifiedName)
			statusValue := pod.Annotations[statusAnnotation]
			if statusValue == injectedStatus {
				continue
			}

			port := 5680
			for a, v := range pod.Annotations {
				parts := strings.Split(a, "/")
				if len(parts) != 2 {
					continue
				}

				domain, path := parts[0], parts[1]
				if path != "port" {
					continue
				}

				_, ok := plugins[domain]
				if !ok {
					continue
				}

				i, err := strconv.Atoi(v)
				if err != nil {
					log.Errorf("Could not decode port annotation '%s'", a, err)
					return admission.Errored(http.StatusInternalServerError, err)
				} else if i > port {
					port = i + 1
				}
			}

			container := corev1.Container{
				Name:            strings.ReplaceAll(fmt.Sprintf("driver-%s-%s", pluginName, version.Name), ".", "-"),
				Image:           version.Driver,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Ports: []corev1.ContainerPort{
					{
						Name:          "driver",
						ContainerPort: int32(port),
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  driverTypeEnv,
						Value: driverQualifiedName,
					},
					{
						Name:  driverNamespaceEnv,
						Value: namespacedName.Namespace,
					},
					{
						Name:  driverNameEnv,
						Value: namespacedName.Name,
					},
					{
						Name:  driverPortEnv,
						Value: fmt.Sprint(port),
					},
					{
						Name: driverNodeEnv,
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "spec.nodeName",
							},
						},
					},
				},
			}
			pod.Spec.Containers = append(pod.Spec.Containers, container)

			pod.Annotations[statusAnnotation] = injectedStatus
			portAnnotation := fmt.Sprintf("%s/port", driverQualifiedName)
			pod.Annotations[portAnnotation] = fmt.Sprint(port)

			// Marshal the pod and return a patch response
			marshaledPod, err := json.Marshal(pod)
			if err != nil {
				log.Errorf("Driver injection failed for Pod '%s'", namespacedName, err)
				return admission.Errored(http.StatusInternalServerError, err)
			}
			return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
		}
	}
	return admission.Allowed("No drivers to inject")
}

var _ admission.Handler = &DriverInjector{}
