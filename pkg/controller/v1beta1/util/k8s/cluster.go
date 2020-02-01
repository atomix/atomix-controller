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
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/atomix/api/proto/atomix/controller"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	"github.com/gogo/protobuf/jsonpb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strconv"
	"strings"
)

const (
	apiPort      = 5678
	protocolPort = 5679
)

// getClusterResourceName returns the given resource name for the given cluster
func getClusterResourceName(cluster *v1beta1.Cluster, resource string) string {
	return fmt.Sprintf("%s-%s", cluster.Name, resource)
}

// GetClusterName returns the cluster name for the given cluster
func GetClusterName(database *v1beta1.Database, cluster int) string {
	return fmt.Sprintf("%s-%d", database.Name, cluster)
}

// GetClusterNamespacedName returns the NamespacedName for the given cluster
func GetClusterNamespacedName(database *v1beta1.Database, cluster int) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetClusterName(database, cluster),
		Namespace: database.Namespace,
	}
}

// NewCluster returns the configuration for the given cluster
func NewCluster(database *v1beta1.Database, cluster int) *v1beta1.Cluster {
	meta := database.Spec.Template.ObjectMeta
	meta.Name = GetClusterName(database, cluster)
	meta.Namespace = database.Namespace
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	for key, value := range newClusterLabels(database, cluster) {
		meta.Labels[key] = value
	}
	meta.Annotations = newClusterAnnotations(database, cluster)
	return &v1beta1.Cluster{
		ObjectMeta: meta,
		Spec:       database.Spec.Template.Spec,
	}
}

// GetClusterLabelsForDatabase returns the labels for the clusters in the given database
func GetClusterLabelsForDatabase(database *v1beta1.Database) map[string]string {
	return map[string]string{
		appKey:      atomixApp,
		typeKey:     clusterType,
		databaseKey: database.Name,
	}
}

// GetClusterLabels returns the labels for the given cluster
func GetClusterLabels(cluster *v1beta1.Cluster) map[string]string {
	labels := make(map[string]string)
	if value, ok := cluster.Labels[appKey]; ok {
		labels[appKey] = value
	}
	if value, ok := cluster.Labels[typeKey]; ok {
		labels[typeKey] = value
	}
	if value, ok := cluster.Labels[databaseKey]; ok {
		labels[databaseKey] = value
	}
	if value, ok := cluster.Labels[clusterKey]; ok {
		labels[clusterKey] = value
	}
	return labels
}

// newClusterLabels returns a new labels map containing the cluster app
func newClusterLabels(database *v1beta1.Database, cluster int) map[string]string {
	labels := GetClusterLabelsForDatabase(database)
	labels[clusterKey] = fmt.Sprint(cluster)
	return labels
}

// newClusterAnnotations returns annotations for the given cluster
func newClusterAnnotations(database *v1beta1.Database, cluster int) map[string]string {
	return map[string]string{
		controllerAnnotation: GetQualifiedControllerName(),
		typeAnnotation:       clusterType,
		databaseAnnotation:   database.Name,
		clusterAnnotation:    fmt.Sprint(cluster),
	}
}

// GetDatabaseFromClusterAnnotations returns the database name from the given cluster annotations
func GetDatabaseFromClusterAnnotations(cluster *v1beta1.Cluster) (string, error) {
	database, ok := cluster.Annotations[databaseAnnotation]
	if !ok {
		return "", errors.New("cluster missing database annotation")
	}
	return database, nil
}

// GetClusterIDFromClusterAnnotations returns the cluster ID from the given cluster annotations
func GetClusterIDFromClusterAnnotations(cluster *v1beta1.Cluster) (int32, error) {
	idstr, ok := cluster.Annotations[clusterAnnotation]
	if !ok {
		return 1, nil
	}

	id, err := strconv.ParseInt(idstr, 0, 32)
	if err != nil {
		return 0, err
	}
	return int32(id), nil
}

// GetClusterIDFromClusterName returns the cluster ID from the given cluster name
func GetClusterIDFromClusterName(name string) (int, error) {
	parts := strings.Split(name, "-")
	idstr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idstr, 0, 32)
	return int(id), err
}

// GetClusterServiceName returns the given cluster's service name
func GetClusterServiceName(cluster *v1beta1.Cluster) string {
	return cluster.Name
}

// getPodName returns the name of the pod for the given pod ID
func getPodName(cluster *v1beta1.Cluster, pod int) string {
	return fmt.Sprintf("%s-%d", cluster.Name, pod)
}

// getPodDNSName returns the fully qualified DNS name for the given pod ID
func getPodDNSName(cluster *v1beta1.Cluster, pod int) string {
	return fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", cluster.Name, pod, GetClusterHeadlessServiceName(cluster), cluster.Namespace)
}

// GetClusterServiceNamespacedName returns the given cluster's NamespacedName
func GetClusterServiceNamespacedName(cluster *v1beta1.Cluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}
}

// GetClusterHeadlessServiceName returns the headless service name for the given cluster
func GetClusterHeadlessServiceName(cluster *v1beta1.Cluster) string {
	return getClusterResourceName(cluster, headlessServiceSuffix)
}

// GetClusterDisruptionBudgetName returns the pod disruption budget name for the given cluster
func GetClusterDisruptionBudgetName(cluster *v1beta1.Cluster) string {
	return getClusterResourceName(cluster, disruptionBudgetSuffix)
}

// GetClusterConfigMapName returns the ConfigMap name for the given cluster
func GetClusterConfigMapName(cluster *v1beta1.Cluster) string {
	return getClusterResourceName(cluster, configSuffix)
}

// GetClusterStatefulSetName returns the StatefulSet name for the given cluster
func GetClusterStatefulSetName(cluster *v1beta1.Cluster) string {
	return cluster.Name
}

// NewClusterConfigMap returns a new ConfigMap for initializing Atomix clusters
func NewClusterConfigMap(cluster *v1beta1.Cluster, config interface{}) (*corev1.ConfigMap, error) {
	clusterConfig, err := newNodeConfigString(cluster)
	if err != nil {
		return nil, err
	}

	protocolConfig, err := newProtocolConfigString(config)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterConfigMapName(cluster),
			Namespace: cluster.Namespace,
			Labels:    cluster.Labels,
		},
		Data: map[string]string{
			clusterConfigFile:  clusterConfig,
			protocolConfigFile: protocolConfig,
		},
	}, nil
}

// newNodeConfigString creates a node configuration string for the given cluster
func newNodeConfigString(cluster *v1beta1.Cluster) (string, error) {
	clusterID, err := GetClusterIDFromClusterAnnotations(cluster)
	if err != nil {
		return "", err
	}

	clusterDatabase, err := GetDatabaseFromClusterAnnotations(cluster)
	if err != nil {
		return "", err
	}

	members := make([]*api.MemberConfig, cluster.Spec.Backend.Replicas)
	for i := 0; i < int(cluster.Spec.Backend.Replicas); i++ {
		members[i] = &api.MemberConfig{
			ID:           getPodName(cluster, i),
			Host:         getPodDNSName(cluster, i),
			ProtocolPort: protocolPort,
			APIPort:      apiPort,
		}
	}

	partitions := make([]*api.PartitionId, 0, cluster.Spec.Partitions)
	for partitionID := (cluster.Spec.Partitions * (clusterID - 1)) + 1; partitionID <= cluster.Spec.Partitions*clusterID; partitionID++ {
		partition := &api.PartitionId{
			Partition: partitionID,
			Group: &api.PartitionGroupId{
				Name:      clusterDatabase,
				Namespace: cluster.Namespace,
			},
			Cluster: &api.ClusterId{
				ID: int32(clusterID),
				DatabaseID: &api.DatabaseId{
					Name:      clusterDatabase,
					Namespace: cluster.Namespace,
				},
			},
		}
		partitions = append(partitions, partition)
	}

	config := &api.ClusterConfig{
		Members:    members,
		Partitions: partitions,
	}

	marshaller := jsonpb.Marshaler{}
	return marshaller.MarshalToString(config)
}

// newProtocolConfigString creates a protocol configuration string for the given cluster and protocol
func newProtocolConfigString(config interface{}) (string, error) {
	bytes, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// NewClusterDisruptionBudget returns a new pod disruption budget for the cluster group cluster
func NewClusterDisruptionBudget(cluster *v1beta1.Cluster) *policyv1beta1.PodDisruptionBudget {
	minAvailable := intstr.FromInt(int(cluster.Spec.Backend.Replicas)/2 + 1)
	return &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterDisruptionBudgetName(cluster),
			Namespace: cluster.Namespace,
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
		},
	}
}

// GetClusterClusterGroupServiceName returns the cluster group service name for a cluster
func GetClusterClusterGroupServiceName(cluster *v1beta1.Cluster) string {
	group, err := GetDatabaseFromClusterAnnotations(cluster)
	if err != nil {
		return cluster.Name[:strings.LastIndex(cluster.Name, "-")]
	}
	return group
}

// GetClusterClusterGroupServiceNamespacedName returns the cluster group service name for a cluster
func GetClusterClusterGroupServiceNamespacedName(cluster *v1beta1.Cluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetClusterClusterGroupServiceName(cluster),
		Namespace: cluster.Namespace,
	}
}

// GetClusterClusterGroupName returns the cluster group name for a cluster
func GetClusterClusterGroupName(cluster *v1beta1.Cluster) string {
	group, err := GetDatabaseFromClusterAnnotations(cluster)
	if err != nil {
		return cluster.Name[:strings.LastIndex(cluster.Name, "-")]
	}
	return group
}

// GetClusterClusterGroupNamespacedName returns the cluster group service name for a cluster
func GetClusterClusterGroupNamespacedName(cluster *v1beta1.Cluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetClusterClusterGroupName(cluster),
		Namespace: cluster.Namespace,
	}
}

// NewClusterService returns a new service for a cluster
func NewClusterService(cluster *v1beta1.Cluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterServiceName(cluster),
			Namespace: cluster.Namespace,
			Labels:    cluster.Labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: apiPort,
				},
			},
			Selector: GetClusterLabels(cluster),
		},
	}
}

// NewClusterHeadlessService returns a new headless service for a cluster group
func NewClusterHeadlessService(cluster *v1beta1.Cluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterHeadlessServiceName(cluster),
			Namespace: cluster.Namespace,
			Labels:    cluster.Labels,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: apiPort,
				},
				{
					Name: "protocol",
					Port: protocolPort,
				},
			},
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Selector:                 GetClusterLabels(cluster),
		},
	}
}

// NewBackendStatefulSet returns a new StatefulSet for a cluster group
func NewBackendStatefulSet(cluster *v1beta1.Cluster, image string, pullPolicy corev1.PullPolicy, probePort int32) (*appsv1.StatefulSet, error) {
	var affinity *corev1.Affinity

	group, err := GetDatabaseFromClusterAnnotations(cluster)
	if err != nil {
		return nil, err
	}

	id, err := GetClusterIDFromClusterAnnotations(cluster)
	if err != nil {
		return nil, err
	}

	if group != "" && id != 0 {
		affinity = newAffinity(group, id)
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterStatefulSetName(cluster),
			Namespace: cluster.Namespace,
			Labels:    cluster.Labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: GetClusterHeadlessServiceName(cluster),
			Replicas:    &cluster.Spec.Backend.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: GetClusterLabels(cluster),
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: cluster.Labels,
				},
				Spec: corev1.PodSpec{
					Affinity: affinity,
					Containers: []corev1.Container{
						newContainer(image, pullPolicy, cluster.Spec.Backend.Env, cluster.Spec.Backend.Resources, []corev1.VolumeMount{
							newDataVolumeMount(),
							newConfigVolumeMount(),
						}, probePort),
					},
					Volumes: []corev1.Volume{
						newConfigVolume(GetClusterConfigMapName(cluster)),
						newDataVolume(),
					},
				},
			},
		},
	}, err
}
