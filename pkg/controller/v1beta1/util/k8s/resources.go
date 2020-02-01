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

package k8s

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"os"
)

const (
	appKey       = "app"
	atomixApp    = "atomix"
	typeKey      = "type"
	databaseKey  = "database"
	clusterKey   = "cluster"
	partitionKey = "partition"
)

const (
	controllerAnnotation = "cloud.atomix.io/controller"
	typeAnnotation       = "cloud.atomix.io/type"
	databaseAnnotation   = "cloud.atomix.io/group"
	clusterAnnotation    = "cloud.atomix.io/cluster"
)

const (
	clusterType   = "cluster"
	proxyType     = "proxy"
	partitionType = "partition"
)

const (
	headlessServiceSuffix  = "hs"
	disruptionBudgetSuffix = "pdb"
	configSuffix           = "config"
)

const (
	configPath         = "/etc/atomix"
	clusterConfigFile  = "cluster.json"
	protocolConfigFile = "protocol.json"
)

const (
	configVolume = "config"
	dataVolume   = "data"
)

const (
	controllerNameVar      = "CONTROLLER_NAME"
	controllerNamespaceVar = "CONTROLLER_NAMESPACE"
)

const (
	defaultNamespace = "default"
)

// GetControllerName gets the name of the current controller from the environment
func GetControllerName() string {
	return os.Getenv(controllerNameVar)
}

// GetControllerNamespace gets the controller's namespace from the environment
func GetControllerNamespace() string {
	return os.Getenv(controllerNamespaceVar)
}

// GetQualifiedControllerName returns the qualified controller name
func GetQualifiedControllerName() string {
	return fmt.Sprintf("%s.%s", GetControllerNamespace(), GetControllerName())
}

// newAffinity returns a new affinity policy for the given partition
func newAffinity(group string, partition int32) *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 1,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      appKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										atomixApp,
									},
								},
								{
									Key:      typeKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										clusterType,
									},
								},
								{
									Key:      databaseKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										group,
									},
								},
								{
									Key:      clusterKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										fmt.Sprint(partition),
									},
								},
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

// newContainer returns a container for a node
func newContainer(image string, pullPolicy corev1.PullPolicy, env []corev1.EnvVar, resources corev1.ResourceRequirements, volumeMounts []corev1.VolumeMount, probePort int32) corev1.Container {
	env = append(env, corev1.EnvVar{
		Name: "NODE_ID",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.name",
			},
		},
	})
	args := []string{
		"$(NODE_ID)",
		fmt.Sprintf("%s/%s", configPath, clusterConfigFile),
		fmt.Sprintf("%s/%s", configPath, protocolConfigFile),
	}
	return corev1.Container{
		Name:            "atomix",
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Env:             env,
		Resources:       resources,
		Ports: []corev1.ContainerPort{
			{
				Name:          "api",
				ContainerPort: 5678,
			},
			{
				Name:          "protocol",
				ContainerPort: 5679,
			},
		},
		Args: args,
		ReadinessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{"stat", "/tmp/atomix-ready"},
				},
			},
			InitialDelaySeconds: 5,
			TimeoutSeconds:      10,
			FailureThreshold:    12,
		},
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.IntOrString{Type: intstr.Int, IntVal: probePort},
				},
			},
			InitialDelaySeconds: 60,
			TimeoutSeconds:      10,
		},
		VolumeMounts: volumeMounts,
	}
}

// newDataVolumeMount returns a data volume mount for a pod
func newDataVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      dataVolume,
		MountPath: "/var/lib/atomix",
	}
}

// newConfigVolumeMount returns a configuration volume mount for a pod
func newConfigVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      configVolume,
		MountPath: configPath,
	}
}

// newConfigVolume returns the configuration volume for a pod
func newConfigVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: configVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		},
	}
}

// newDataVolume returns the data volume for a pod
func newDataVolume() corev1.Volume {
	return corev1.Volume{
		Name: dataVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}
