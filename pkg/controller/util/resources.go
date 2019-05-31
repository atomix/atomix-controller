/*
 * Copyright 2019 Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	InitSuffix             = "init"
	ConfigSuffix           = "config"
)

const (
	InitScriptsVolume  = "init-scripts"
	UserConfigVolume   = "user-config"
	SystemConfigVolume = "system-config"
	DataVolume         = "data"
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

func GetControllerNamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      GetControllerName(),
		Namespace: GetControllerNamespace(),
	}
}

func GetControllerLabels() map[string]string {
	return map[string]string{
		AppKey:        AtomixApp,
		ControllerKey: GetControllerNamespacedName().String(),
	}
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
										string(partition),
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

func newInitContainers(size int32) []corev1.Container {
	return []corev1.Container{
		newInitContainer(size),
	}
}

func newInitContainer(size int32) corev1.Container {
	return corev1.Container{
		Name:  "configure",
		Image: "ubuntu:16.04",
		Env: []corev1.EnvVar{
			{
				Name:  "ATOMIX_REPLICAS",
				Value: fmt.Sprint(size),
			},
		},
		Command: []string{
			"bash",
			"-c",
			"/scripts/create_config.sh $ATOMIX_REPLICAS > /config/atomix.yaml",
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      InitScriptsVolume,
				MountPath: "/scripts",
			},
			{
				Name:      SystemConfigVolume,
				MountPath: "/config",
			},
		},
	}
}

func newPersistentContainers(version string, env []corev1.EnvVar, resources corev1.ResourceRequirements) []corev1.Container {
	return []corev1.Container{
		newPersistentContainer(version, env, resources),
	}
}

func newPersistentContainer(version string, env []corev1.EnvVar, resources corev1.ResourceRequirements) corev1.Container {
	args := []string{
		"--config",
		"/etc/atomix/system/atomix.yaml",
	}
	return newContainer(fmt.Sprintf("atomix/atomix-server:%s", version), args, env, resources, newPersistentVolumeMounts())
}

func newEphemeralContainers(version string, env []corev1.EnvVar, resources corev1.ResourceRequirements) []corev1.Container {
	return []corev1.Container{
		newEphemeralContainer(version, env, resources),
	}
}

func newEphemeralContainer(version string, env []corev1.EnvVar, resources corev1.ResourceRequirements) corev1.Container {
	args := []string{
		"--config",
		"/etc/atomix/system/atomix.yaml",
	}
	return newContainer(fmt.Sprintf("atomix/atomix-server:%s", version), args, env, resources, newEphemeralVolumeMounts())
}

func newContainer(image string, args []string, env []corev1.EnvVar, resources corev1.ResourceRequirements, volumeMounts []corev1.VolumeMount) corev1.Container {
	return corev1.Container{
		Name:            "atomix",
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		Env:             env,
		Resources:       resources,
		Ports: []corev1.ContainerPort{
			{
				Name:          "api",
				ContainerPort: 5678,
			},
		},
		Args: args,
		ReadinessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.IntOrString{Type: intstr.Int, IntVal: 5678},
				},
			},
			InitialDelaySeconds: 60,
			TimeoutSeconds:      10,
			FailureThreshold:    6,
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
		newSystemConfigVolumeMount(),
	}
}

func newEphemeralVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		newSystemConfigVolumeMount(),
	}
}

func newDataVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      DataVolume,
		MountPath: "/var/lib/atomix",
	}
}

func newSystemConfigVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      SystemConfigVolume,
		MountPath: "/etc/atomix/system",
	}
}

func newInitScriptsVolume(name string) corev1.Volume {
	defaultMode := int32(0744)
	return corev1.Volume{
		Name: InitScriptsVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
				DefaultMode: &defaultMode,
			},
		},
	}
}

func newUserConfigVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: UserConfigVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		},
	}
}

func newSystemConfigVolume() corev1.Volume {
	return corev1.Volume{
		Name: SystemConfigVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func newPersistentVolumeClaims(className *string, size string) ([]corev1.PersistentVolumeClaim, error) {
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
