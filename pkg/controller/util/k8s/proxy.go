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
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetProxyLabels returns the labels for the given partition
func GetProxyLabels(partition *v1alpha1.Partition) map[string]string {
	labels := make(map[string]string)
	if value, ok := partition.Labels[appKey]; ok {
		labels[appKey] = value
	}
	if value, ok := partition.Labels[groupKey]; ok {
		labels[groupKey] = value
	}
	if value, ok := partition.Labels[partitionKey]; ok {
		labels[partitionKey] = value
	}
	labels[typeKey] = proxyType
	return labels
}

// GetProxyConfigMapName returns the ConfigMap name for the given proxy and partition
func GetProxyConfigMapName(proxy string, partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, proxy)
}

// GetProxyDeploymentName returns the StatefulSet name for the given partition
func GetProxyDeploymentName(proxy string, partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, proxy)
}

// NewProxyConfigMap returns a new ConfigMap for initializing a proxy
func NewProxyConfigMap(proxy string, partition *v1alpha1.Partition, config map[string]interface{}) (*corev1.ConfigMap, error) {
	partitionConfig, err := newNodeConfigString(partition)
	if err != nil {
		return nil, err
	}

	protocolConfig, err := newProtocolConfigString(config)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetProxyConfigMapName(proxy, partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
		},
		Data: map[string]string{
			partitionConfigFile: partitionConfig,
			protocolConfigFile:  protocolConfig,
		},
	}, nil
}

// NewProxyService returns a new service for a partition proxy
func NewProxyService(partition *v1alpha1.Partition) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionServiceName(partition),
			Namespace: partition.Namespace,
			Labels:    GetProxyLabels(partition),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: 5678,
				},
			},
			Selector: GetProxyLabels(partition),
		},
	}
}

// NewProxyDeployment returns a new Deployment for a partition proxy
func NewProxyDeployment(proxy string, image string, pullPolicy corev1.PullPolicy, partition *v1alpha1.Partition) (*appsv1.Deployment, error) {
	var one int32 = 1
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetProxyDeploymentName(proxy, partition),
			Namespace: partition.Namespace,
			Labels:    GetProxyLabels(partition),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: GetProxyLabels(partition),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: GetProxyLabels(partition),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						newContainer(image, pullPolicy, partition.Spec.Env, partition.Spec.Resources, []corev1.VolumeMount{newConfigVolumeMount()}, 5678),
					},
					Volumes: []corev1.Volume{
						newConfigVolume(GetProxyConfigMapName(proxy, partition)),
					},
				},
			},
		},
	}, nil
}
