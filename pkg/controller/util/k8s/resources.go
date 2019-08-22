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
	AppKey        = "app"
	AtomixApp     = "atomix"
	ControllerKey = "controller"
	TypeKey       = "type"
	GroupKey      = "group"
	PartitionKey  = "partition"
)

const (
	ControllerAnnotation = "k8s.atomix.io/controller"
	TypeAnnotation       = "k8s.atomix.io/type"
	GroupAnnotation      = "k8s.atomix.io/group"
	PartitionAnnotation  = "k8s.atomix.io/partition"
)

const (
	GroupType     = "group"
	PartitionType = "partition"
)

const (
	ServiceSuffix          = "service"
	HeadlessServiceSuffix  = "hs"
	DisruptionBudgetSuffix = "pdb"
	ConfigSuffix           = "config"
)

const (
	ConfigPath          = "/etc/atomix"
	PartitionConfigFile = "partition.json"
	ProtocolConfigFile  = "protocol.json"
)

const (
	InitScriptsVolume = "init-scripts"
	EnvVolume         = "env"
	ConfigVolume      = "config"
	DataVolume        = "data"
)

const (
	ControllerNameVar      = "CONTROLLER_NAME"
	ControllerNamespaceVar = "CONTROLLER_NAMESPACE"
)

const (
	DefaultNamespace = "default"
)

func GetControllerName() string {
	return os.Getenv(ControllerNameVar)
}

func GetControllerNamespace() string {
	return os.Getenv(ControllerNamespaceVar)
}

func GetControllerNameString() string {
	return fmt.Sprintf("%s.%s", GetControllerNamespace(), GetControllerName())
}

func GetControllerLabels() map[string]string {
	return map[string]string{
		AppKey:        AtomixApp,
		ControllerKey: GetControllerNameString(),
	}
}

// GetServiceDnsName returns the DNS hostname for a service
func GetServiceDnsName(service *corev1.Service) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
}

func getControllerServiceDnsName() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", GetControllerName(), GetControllerNamespace())
}

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
									Key:      AppKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										AtomixApp,
									},
								},
								{
									Key:      TypeKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										PartitionType,
									},
								},
								{
									Key:      GroupKey,
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										group,
									},
								},
								{
									Key:      PartitionKey,
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

func newPersistentContainers(image string, env []corev1.EnvVar, resources corev1.ResourceRequirements) []corev1.Container {
	return []corev1.Container{
		newPersistentContainer(image, env, resources),
	}
}

func newPersistentContainer(image string, env []corev1.EnvVar, resources corev1.ResourceRequirements) corev1.Container {
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
		fmt.Sprintf("%s/%s", ConfigPath, PartitionConfigFile),
		fmt.Sprintf("%s/%s", ConfigPath, ProtocolConfigFile),
	}
	return newContainer(image, args, env, resources, newPersistentVolumeMounts())
}

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

func newPersistentVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		newDataVolumeMount(),
		newConfigVolumeMount(),
	}
}

func newDataVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      DataVolume,
		MountPath: "/var/lib/atomix",
	}
}

func newConfigVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      ConfigVolume,
		MountPath: ConfigPath,
	}
}

func newVolumes(configName string, storageClass *string) []corev1.Volume {
	if storageClass == nil {
		return []corev1.Volume{
			newConfigVolume(configName),
			newDataVolume(),
		}
	} else {
		return []corev1.Volume{
			newConfigVolume(configName),
		}
	}
}

func newConfigVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: ConfigVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		},
	}
}

func newDataVolume() corev1.Volume {
	return corev1.Volume{
		Name: DataVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

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

func newPersistentVolumeClaim(className *string, size string) (corev1.PersistentVolumeClaim, error) {
	quantity, err := resource.ParseQuantity(size)
	if err != nil {
		return corev1.PersistentVolumeClaim{}, err
	}
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: DataVolume,
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
