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
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getProxyName returns the name of the given cluster's proxy
func getProxyName(cluster *v1beta1.Cluster) string {
	return fmt.Sprintf("%s-proxy", cluster.Name)
}

// GetProxyLabels returns the labels for the given partition
func GetProxyLabels(cluster *v1beta1.Cluster) map[string]string {
	labels := make(map[string]string)
	if value, ok := cluster.Labels[appKey]; ok {
		labels[appKey] = value
	}
	if value, ok := cluster.Labels[databaseKey]; ok {
		labels[databaseKey] = value
	}
	if value, ok := cluster.Labels[clusterKey]; ok {
		labels[clusterKey] = value
	}
	labels[typeKey] = proxyType
	return labels
}

// GetProxyConfigMapName returns the ConfigMap name for the given proxy and partition
func GetProxyConfigMapName(cluster *v1beta1.Cluster) string {
	return getClusterResourceName(cluster, getProxyName(cluster))
}

// GetProxyDeploymentName returns the StatefulSet name for the given partition
func GetProxyDeploymentName(cluster *v1beta1.Cluster) string {
	return getClusterResourceName(cluster, getProxyName(cluster))
}

// NewProxyConfigMap returns a new ConfigMap for initializing a proxy
func NewProxyConfigMap(cluster *v1beta1.Cluster, config map[string]interface{}) (*corev1.ConfigMap, error) {
	partitionConfig, err := newNodeConfigString(cluster)
	if err != nil {
		return nil, err
	}

	protocolConfig, err := newProtocolConfigString(config)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetProxyConfigMapName(cluster),
			Namespace: cluster.Namespace,
			Labels:    cluster.Labels,
		},
		Data: map[string]string{
			clusterConfigFile:  partitionConfig,
			protocolConfigFile: protocolConfig,
		},
	}, nil
}

// NewProxyService returns a new service for a partition proxy
func NewProxyService(cluster *v1beta1.Cluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterServiceName(cluster),
			Namespace: cluster.Namespace,
			Labels:    GetProxyLabels(cluster),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: 5678,
				},
			},
			Selector: GetProxyLabels(cluster),
		},
	}
}

// NewProxyDeployment returns a new Deployment for a partition proxy
func NewProxyDeployment(image string, pullPolicy corev1.PullPolicy, cluster *v1beta1.Cluster) (*appsv1.Deployment, error) {
	var one int32 = 1
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetProxyDeploymentName(cluster),
			Namespace: cluster.Namespace,
			Labels:    GetProxyLabels(cluster),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: GetProxyLabels(cluster),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: GetProxyLabels(cluster),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						newContainer(image, pullPolicy, cluster.Spec.Proxy.Env, cluster.Spec.Proxy.Resources, []corev1.VolumeMount{newConfigVolumeMount()}, 5678),
					},
					Volumes: []corev1.Volume{
						newConfigVolume(GetProxyConfigMapName(cluster)),
					},
				},
			},
		},
	}, nil
}
