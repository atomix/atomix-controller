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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"os"
)

const (
	appKey       = "app"
	atomixApp    = "atomix"
	typeKey      = "type"
	groupKey     = "group"
	partitionKey = "partition"
)

const (
	controllerAnnotation = "k8s.atomix.io/controller"
	typeAnnotation       = "k8s.atomix.io/type"
	groupAnnotation      = "k8s.atomix.io/group"
	partitionAnnotation  = "k8s.atomix.io/partition"
)

const (
	partitionType = "partition"
)

const (
	headlessServiceSuffix  = "hs"
	disruptionBudgetSuffix = "pdb"
	configSuffix           = "config"
)

const (
	configPath          = "/etc/atomix"
	partitionConfigFile = "partition.json"
	protocolConfigFile  = "protocol.json"
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

// getControllerServiceDNSName returns the fully qualified DNS address for the controller
func getControllerServiceDNSName() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", GetControllerName(), GetControllerNamespace())
}

// newAffinity returns a new affinity policy for the given partition
func newAffinity(group string, partition int) *corev1.Affinity {
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
										partitionType,
									},
								},
								{
									Key:      groupKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										group,
									},
								},
								{
									Key:      partitionKey,
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

// newPersistentContainers returns the containers for a node
func newPersistentContainers(image string, pullPolicy corev1.PullPolicy, env []corev1.EnvVar, resources corev1.ResourceRequirements) []corev1.Container {
	return []corev1.Container{
		newPersistentContainer(image, pullPolicy, env, resources),
	}
}

// newPersistentContainer returns a container for a node
func newPersistentContainer(image string, pullPolicy corev1.PullPolicy, env []corev1.EnvVar, resources corev1.ResourceRequirements) corev1.Container {
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
		fmt.Sprintf("%s/%s", configPath, partitionConfigFile),
		fmt.Sprintf("%s/%s", configPath, protocolConfigFile),
	}
	return newContainer(image, args, env, resources, newPersistentVolumeMounts())
}

// newContainer returns the container for a node
func newContainer(image string, args []string, env []corev1.EnvVar, resources corev1.ResourceRequirements, volumeMounts []corev1.VolumeMount) corev1.Container {
	return corev1.Container{
		Name:            "atomix",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
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
					Port: intstr.IntOrString{Type: intstr.Int, IntVal: 5678},
				},
			},
			InitialDelaySeconds: 60,
			TimeoutSeconds:      10,
		},
		VolumeMounts: volumeMounts,
	}
}

// newPersistentVolumeMounts returns the persistent volume mounts for a node
func newPersistentVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		newDataVolumeMount(),
		newConfigVolumeMount(),
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

// newVolumes returns the volumes for a pod
func newVolumes(configName string, storageClass *string) []corev1.Volume {
	if storageClass == nil {
		return []corev1.Volume{
			newConfigVolume(configName),
			newDataVolume(),
		}
	}
	return []corev1.Volume{
		newConfigVolume(configName),
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

// newPersistentVolumeClaims returns the persistent volume claims for a pod
func newPersistentVolumeClaims(className *string, size string) ([]corev1.PersistentVolumeClaim, error) {
	if className == nil {
		return []corev1.PersistentVolumeClaim{}, nil
	}

	claim, err := newPersistentVolumeClaim(className, size)
	if err != nil {
		return nil, err
	}
	return []corev1.PersistentVolumeClaim{
		claim,
	}, nil
}

// newPersistentVolumeClaim returns the persistent volume claim for a pod
func newPersistentVolumeClaim(className *string, size string) (corev1.PersistentVolumeClaim, error) {
	quantity, err := resource.ParseQuantity(size)
	if err != nil {
		return corev1.PersistentVolumeClaim{}, err
	}
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: dataVolume,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: className,
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
		},
	}, nil
}
