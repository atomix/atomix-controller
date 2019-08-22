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
	"errors"
	"fmt"
	api "github.com/atomix/atomix-api/proto/atomix/controller"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/protocol"
	"github.com/golang/protobuf/jsonpb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strconv"
	"strings"
)

func getPartitionResourceName(partition *v1alpha1.Partition, resource string) string {
	return fmt.Sprintf("%s-%s", partition.Name, resource)
}

func GetPartitionName(set *v1alpha1.PartitionSet, partition int) string {
	return fmt.Sprintf("%s-%d", set.Name, partition)
}

func GetPartitionNamespacedName(set *v1alpha1.PartitionSet, partition int) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionName(set, partition),
		Namespace: set.Namespace,
	}
}

func NewPartition(set *v1alpha1.PartitionSet, partition int) *v1alpha1.Partition {
	meta := set.Spec.Template.ObjectMeta
	meta.Name = GetPartitionName(set, partition)
	meta.Namespace = set.Namespace
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	for key, value := range newPartitionLabels(set, partition) {
		meta.Labels[key] = value
	}
	meta.Annotations = newPartitionAnnotations(set, partition)
	return &v1alpha1.Partition{
		ObjectMeta: meta,
		Spec:       set.Spec.Template.Spec,
	}
}

func GetPartitionSetPartitionLabels(set *v1alpha1.PartitionSet) map[string]string {
	return map[string]string{
		AppKey:   AtomixApp,
		TypeKey:  PartitionType,
		GroupKey: set.Name,
	}
}

func GetPartitionLabels(partition *v1alpha1.Partition) map[string]string {
	labels := make(map[string]string)
	if value, ok := partition.Labels[AppKey]; ok {
		labels[AppKey] = value
	}
	if value, ok := partition.Labels[TypeKey]; ok {
		labels[TypeKey] = value
	}
	if value, ok := partition.Labels[GroupKey]; ok {
		labels[GroupKey] = value
	}
	if value, ok := partition.Labels[PartitionKey]; ok {
		labels[PartitionKey] = value
	}
	return labels
}

// newPartitionLabels returns a new labels map containing the partition app
func newPartitionLabels(set *v1alpha1.PartitionSet, partition int) map[string]string {
	labels := GetPartitionSetPartitionLabels(set)
	labels[PartitionKey] = fmt.Sprint(partition)
	return labels
}

// newPartitionAnnotations returns annotations for the given partition
func newPartitionAnnotations(group *v1alpha1.PartitionSet, partition int) map[string]string {
	return map[string]string{
		ControllerAnnotation: GetControllerNameString(),
		TypeAnnotation:       PartitionType,
		GroupAnnotation:      group.Name,
		PartitionAnnotation:  fmt.Sprint(partition),
	}
}

func getPartitionGroupFromAnnotation(partition *v1alpha1.Partition) (string, error) {
	group, ok := partition.Annotations[GroupAnnotation]
	if !ok {
		return "", errors.New("partition missing group annotation")
	}
	return group, nil
}

func getPartitionIdFromAnnotation(partition *v1alpha1.Partition) (int, error) {
	idstr, ok := partition.Annotations[PartitionAnnotation]
	if !ok {
		return 0, errors.New("partition missing partition ID annotation")
	}

	id, err := strconv.ParseInt(idstr, 0, 32)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func GetPartitionIdFromPartitionName(name string) (int, error) {
	parts := strings.Split(name, "-")
	idstr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idstr, 0, 32)
	return int(id), err
}

func GetPartitionServiceName(partition *v1alpha1.Partition) string {
	return partition.Name
}

func getPodName(partition *v1alpha1.Partition, pod int) string {
	return fmt.Sprintf("%s-%d", partition.Name, pod)
}

func getPodDnsName(partition *v1alpha1.Partition, pod int) string {
	return fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", partition.Name, pod, GetPartitionHeadlessServiceName(partition), partition.Namespace)
}

func GetPartitionServiceNamespacedName(partition *v1alpha1.Partition) types.NamespacedName {
	return types.NamespacedName{
		Name:      partition.Name,
		Namespace: partition.Namespace,
	}
}

func GetPartitionHeadlessServiceName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, HeadlessServiceSuffix)
}

func GetPartitionDisruptionBudgetName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, DisruptionBudgetSuffix)
}

func GetPartitionConfigMapName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, ConfigSuffix)
}

func GetPartitionStatefulSetName(partition *v1alpha1.Partition) string {
	return partition.Name
}

// NewPartitionConfigMap returns a new ConfigMap for initializing Atomix clusters
func NewPartitionConfigMap(partition *v1alpha1.Partition, protocols *protocol.ProtocolManager) (*corev1.ConfigMap, error) {
	partitionConfig, err := toNodeConfig(partition)
	if err != nil {
		return nil, err
	}

	protocolConfig, err := toProtocolConfig(partition, protocols)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionConfigMapName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
		},
		Data: map[string]string{
			PartitionConfigFile: partitionConfig,
			ProtocolConfigFile:  protocolConfig,
		},
	}, nil
}

func toNodeConfig(partition *v1alpha1.Partition) (string, error) {
	partitionId, err := getPartitionIdFromAnnotation(partition)
	if err != nil {
		return "", err
	}

	partitionGroup, err := getPartitionGroupFromAnnotation(partition)
	if err != nil {
		return "", err
	}

	nodes := make([]*api.NodeConfig, partition.Spec.Size)
	for i := 0; i < int(partition.Spec.Size); i++ {
		nodes[i] = &api.NodeConfig{
			ID:   getPodName(partition, i),
			Host: getPodDnsName(partition, i),
			Port: 5679,
		}
	}

	config := &api.PartitionConfig{
		Partition: &api.PartitionId{
			Partition: int32(partitionId),
			Group: &api.PartitionGroupId{
				Name:      partitionGroup,
				Namespace: partition.Namespace,
			},
		},
		Controller: &api.NodeConfig{
			ID:   GetControllerName(),
			Host: getControllerServiceDnsName(),
			Port: 5679,
		},
		Members: nodes,
	}

	marshaller := jsonpb.Marshaler{}
	return marshaller.MarshalToString(config)
}

func toProtocolConfig(partition *v1alpha1.Partition, protocols *protocol.ProtocolManager) (string, error) {
	protocol, err := protocols.GetProtocolByName(partition.Spec.Protocol)
	if err != nil {
		return "", err
	}
	bytes, err := protocol.YamlToJson([]byte(partition.Spec.Config))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// NewPartitionDisruptionBudget returns a new pod disruption budget for the partition group partition
func NewPartitionDisruptionBudget(partition *v1alpha1.Partition) *v1beta1.PodDisruptionBudget {
	minAvailable := intstr.FromInt(int(partition.Spec.Size)/2 + 1)
	return &v1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionDisruptionBudgetName(partition),
			Namespace: partition.Namespace,
		},
		Spec: v1beta1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
		},
	}
}

// GetPartitionPartitionGroupServiceName returns the partition group service name for a partition
func GetPartitionPartitionGroupServiceName(partition *v1alpha1.Partition) string {
	group, err := getPartitionGroupFromAnnotation(partition)
	if err != nil {
		return partition.Name[:strings.LastIndex(partition.Name, "-")]
	}
	return group
}

// GetPartitionPartitionGroupServiceNamespacedName returns the partition group service name for a partition
func GetPartitionPartitionGroupServiceNamespacedName(partition *v1alpha1.Partition) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionPartitionGroupServiceName(partition),
		Namespace: partition.Namespace,
	}
}

// GetPartitionPartitionGroupName returns the partition group name for a partition
func GetPartitionPartitionGroupName(partition *v1alpha1.Partition) string {
	group, err := getPartitionGroupFromAnnotation(partition)
	if err != nil {
		return partition.Name[:strings.LastIndex(partition.Name, "-")]
	}
	return group
}

// GetPartitionPartitionGroupNamespacedName returns the partition group service name for a partition
func GetPartitionPartitionGroupNamespacedName(partition *v1alpha1.Partition) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionPartitionGroupName(partition),
		Namespace: partition.Namespace,
	}
}

// NewPartitionService returns a new service for a partition
func NewPartitionService(partition *v1alpha1.Partition) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionServiceName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: 5678,
				},
			},
			Selector: GetPartitionLabels(partition),
		},
	}
}

// NewPartitionHeadlessService returns a new headless service for a partition group
func NewPartitionHeadlessService(partition *v1alpha1.Partition) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionHeadlessServiceName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: 5678,
				},
				{
					Name: "protocol",
					Port: 5679,
				},
			},
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Selector:                 GetPartitionLabels(partition),
		},
	}
}

// NewPartitionStatefulSet returns a new StatefulSet for a partition group
func NewPartitionStatefulSet(partition *v1alpha1.Partition) (*appsv1.StatefulSet, error) {
	var affinity *corev1.Affinity
	group, err := getPartitionGroupFromAnnotation(partition)
	id, err := getPartitionIdFromAnnotation(partition)
	if group != "" && id != 0 {
		affinity = newAffinity(group, id)
	}

	claims, err := newPersistentVolumeClaims(partition.Spec.StorageClass, partition.Spec.StorageSize)
	if err != nil {
		return nil, err
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionStatefulSetName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: GetPartitionHeadlessServiceName(partition),
			Replicas:    &partition.Spec.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: GetPartitionLabels(partition),
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: partition.Labels,
				},
				Spec: corev1.PodSpec{
					Affinity:   affinity,
					Containers: newPersistentContainers(partition.Spec.Image, partition.Spec.Env, partition.Spec.Resources),
					Volumes:    newVolumes(GetPartitionConfigMapName(partition), partition.Spec.StorageClass),
				},
			},
			VolumeClaimTemplates: claims,
		},
	}, err
}
