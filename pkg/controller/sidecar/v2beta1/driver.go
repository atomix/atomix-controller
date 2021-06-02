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
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
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

// DriverInjector is a mutating webhook that injects driver containers into pods
type DriverInjector struct {
	client  client.Client
	scheme  *runtime.Scheme
	decoder *admission.Decoder
}

// InjectDecoder :
func (i *DriverInjector) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}

// Handle :
func (i *DriverInjector) Handle(ctx context.Context, request admission.Request) admission.Response {
	// Decode the pod
	pod := &corev1.Pod{}
	if err := i.decoder.Decode(request, pod); err != nil {
		log.Error("Could not decode Pod", err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	namespacedName := types.NamespacedName{
		Namespace: request.Namespace,
		Name:      pod.Name,
	}
	log.Infof("Received admission request for Pod '%s'", namespacedName)

	pluginList := &v2beta1.StoragePluginList{}
	if err := i.client.List(ctx, pluginList); err != nil {
		log.Error("Could not list StoragePlugins", err)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	plugins := make(map[string]v2beta1.StoragePlugin)
	for _, plugin := range pluginList.Items {
		plugins[plugin.Name] = plugin
	}

	injected := false
	for annotation, value := range pod.Annotations {
		parts := strings.Split(annotation, "/")
		if len(parts) != 2 {
			continue
		}

		domain, path := parts[0], parts[1]
		if path != "inject" {
			continue
		}

		if pod.Annotations[getPluginStatusAnnotation(domain)] == injectedStatus {
			continue
		}

		if value != fmt.Sprint(true) {
			continue
		}

		isDriver := func(pluginName, driverName string) bool {
			if plugin, ok := plugins[pluginName]; ok {
				for _, driver := range plugin.Spec.Drivers {
					if driver.Version == driverName {
						return true
					}
				}
				for _, version := range plugin.Spec.DeprecatedVersions {
					if version.Name == driverName {
						return true
					}
				}
			}
			return false
		}

		if plugin, ok := plugins[domain]; ok {
			for _, driver := range plugin.Spec.Drivers {
				port, err := i.getNextPort(pod, plugin.Name, driver.Version, isDriver)
				if err != nil {
					return admission.Errored(http.StatusBadRequest, err)
				}

				ok, err := i.injectDriver(namespacedName, pod, plugin.Name, driver.Version, driver.Image, port)
				if err != nil {
					return admission.Errored(http.StatusInternalServerError, err)
				} else if ok {
					injected = true
				}
			}
			for _, version := range plugin.Spec.DeprecatedVersions {
				port, err := i.getNextPort(pod, plugin.Name, version.Name, isDriver)
				if err != nil {
					return admission.Errored(http.StatusBadRequest, err)
				}

				ok, err := i.injectDriver(namespacedName, pod, plugin.Name, version.Name, version.Driver.Image, port)
				if err != nil {
					return admission.Errored(http.StatusInternalServerError, err)
				} else if ok {
					injected = true
				}
			}
		}
		pod.Annotations[getPluginStatusAnnotation(domain)] = injectedStatus
	}

	if !injected {
		return admission.Allowed("No drivers to inject")
	}

	// Marshal the pod and return a patch response
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		log.Errorf("Driver injection failed for Pod '%s'", namespacedName, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(request.Object.Raw, marshaledPod)
}

func (i *DriverInjector) getNextPort(pod *corev1.Pod, plugin, driver string, isDriver func(plugin, name string) bool) (int, error) {
	if portValue, ok := pod.Annotations[getDriverPortAnnotation(plugin, driver)]; ok {
		return strconv.Atoi(portValue)
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

		if i := strings.Index(domain, "."); i != -1 {
			portDriver, portPlugin := domain[:i], domain[i+1:]
			if !isDriver(portPlugin, portDriver) {
				continue
			}
		}

		i, err := strconv.Atoi(v)
		if err != nil {
			log.Errorf("Could not decode port annotation '%s'", a, err)
			return 0, err
		} else if i > port {
			port = i + 1
		}
	}
	return port, nil
}

func (i *DriverInjector) injectDriver(name types.NamespacedName, pod *corev1.Pod, plugin, driver, image string, port int) (bool, error) {
	driverQualifiedName := fmt.Sprintf("%s.%s", driver, plugin)
	pluginName := plugin[:strings.Index(plugin, ".")]
	statusValue := pod.Annotations[getDriverStatusAnnotation(plugin, driver)]
	if statusValue == injectedStatus {
		return false, nil
	}

	container := corev1.Container{
		Name:            strings.ReplaceAll(fmt.Sprintf("driver-%s-%s", pluginName, driver), ".", "-"),
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args: []string{
			fmt.Sprintf(":%d", port),
		},
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
				Value: name.Namespace,
			},
			{
				Name:  driverNameEnv,
				Value: name.Name,
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

	pod.Annotations[getDriverStatusAnnotation(plugin, driver)] = injectedStatus
	pod.Annotations[getDriverPortAnnotation(plugin, driver)] = fmt.Sprint(port)
	return true, nil
}

func getPluginStatusAnnotation(plugin string) string {
	return fmt.Sprintf("%s/status", plugin)
}

func getDriverStatusAnnotation(plugin, driver string) string {
	return fmt.Sprintf("%s/status", getDriverDomain(plugin, driver))
}

func getDriverPortAnnotation(plugin, driver string) string {
	return fmt.Sprintf("%s/port", getDriverDomain(plugin, driver))
}

func getDriverDomain(plugin, driver string) string {
	return fmt.Sprintf("%s.%s", driver, plugin)
}

var _ admission.Handler = &DriverInjector{}
