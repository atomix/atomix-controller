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
	"errors"
	"fmt"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strconv"
)

func getPartitionResourceName(partition *v1alpha1.Partition, resource string) string {
	return fmt.Sprintf("%s-%s", partition.Name, resource)
}

func GetPartitionName(group *v1alpha1.PartitionGroup, partition int) string {
	return fmt.Sprintf("%s-%d", group.Name, partition)
}

func GetPartitionNamespacedName(group *v1alpha1.PartitionGroup, partition int) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionName(group, partition),
		Namespace: group.Namespace,
	}
}

func NewPartition(group *v1alpha1.PartitionGroup, partition int) *v1alpha1.Partition {
	spec := v1alpha1.PartitionSpec{
		Version:   group.Spec.Version,
		Size:      int32(group.Spec.PartitionSize),
		Env:       group.Spec.Env,
		Resources: group.Spec.Resources,
	}

	if group.Spec.Raft != nil {
		spec.Raft = group.Spec.Raft
	} else if group.Spec.PrimaryBackup != nil {
		spec.PrimaryBackup = group.Spec.PrimaryBackup
	} else if group.Spec.Log != nil {
		spec.Log = group.Spec.Log
	}

	return &v1alpha1.Partition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetPartitionName(group, partition),
			Namespace:   group.Namespace,
			Labels:      newPartitionLabels(group, partition),
			Annotations: newPartitionAnnotations(group, partition),
		},
		Spec: spec,
	}
}

func GetPartitionLabels(group *v1alpha1.PartitionGroup) map[string]string {
	return map[string]string{
		AppKey:       AtomixApp,
		TypeKey:      PartitionType,
		GroupKey:     group.Name,
	}
}

// newPartitionLabels returns a new labels map containing the partition app
func newPartitionLabels(group *v1alpha1.PartitionGroup, partition int) map[string]string {
	return map[string]string{
		AppKey:       AtomixApp,
		TypeKey:      PartitionType,
		GroupKey:     group.Name,
		PartitionKey: fmt.Sprint(partition),
	}
}

// newPartitionAnnotations returns annotations for the given partition
func newPartitionAnnotations(group *v1alpha1.PartitionGroup, partition int) map[string]string {
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

func GetPartitionServiceName(partition *v1alpha1.Partition) string {
	return partition.Name
}

func GetPartitionHeadlessServiceName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, HeadlessServiceSuffix)
}

func GetPartitionDisruptionBudgetName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, DisruptionBudgetSuffix)
}

func GetPartitionInitConfigMapName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, InitSuffix)
}

func GetPartitionStatefulSetName(partition *v1alpha1.Partition) string {
	return partition.Name
}

// NewPartitionInitConfigMap returns a new ConfigMap for initializing Atomix clusters
func NewPartitionInitConfigMap(partition *v1alpha1.Partition) *corev1.ConfigMap {
	script := ""
	if partition.Spec.Raft != nil {
		script = newRaftInitConfigMapScript(partition)
	} else if partition.Spec.PrimaryBackup != nil {
		script = newPrimaryBackupInitConfigMapScript(partition)
	} else if partition.Spec.Log != nil {
		script = newLogInitConfigMapScript(partition)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionInitConfigMapName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
		},
		Data: map[string]string{
			"create_config.sh": script,
		},
	}
}

// newRaftInitConfigMapScript returns a new script for generating a Raft configuration
func newRaftInitConfigMapScript(partition *v1alpha1.Partition) string {
	id, _ := getPartitionIdFromAnnotation(partition)
	group, _ := getPartitionGroupFromAnnotation(partition)
	return fmt.Sprintf(`
#!/usr/bin/env bash
HOST=$(hostname -s)
DOMAIN=$(hostname -d)
REPLICAS=$1
function create_config() {
    echo "partitionId: %d"
    echo "partitionGroup:"
    echo "  name: %s"
    echo "  namespace: %s"
    echo "controller:"
    echo "  id: %s"
    echo "  host: %s"
    echo "  port: 5679"
    echo "node:"
    echo "  id: $NAME-$ORD"
    echo "  host: $NAME-$ORD.$DOMAIN"
    echo "  port: 5678"
    echo "protocol:"
    echo "  type: raft"
    echo "  members:"
    for (( i=0; i<$REPLICAS; i++ ))
    do
        echo "    - id: $NAME-$((i))"
        echo "      host: $NAME-$((i)).$DOMAIN"
        echo "      port: 5678"
    done
    echo "  storage:"
    echo "    directory: /var/lib/atomix"
}
if [[ $HOST =~ (.*)-([0-9]+)$ ]]; then
    NAME=${BASH_REMATCH[1]}
    ORD=${BASH_REMATCH[2]}
else
    echo "Failed to parse name and ordinal of Pod"
    exit 1
fi
create_config`, id, group, partition.Namespace, GetControllerName(), getControllerServiceDnsName())
}

// newPrimaryBackupInitConfigMapScript returns a new script for generating a Raft configuration
func newPrimaryBackupInitConfigMapScript(partition *v1alpha1.Partition) string {
	id, _ := getPartitionIdFromAnnotation(partition)
	group, _ := getPartitionGroupFromAnnotation(partition)
	return fmt.Sprintf(`
#!/usr/bin/env bash
HOST=$(hostname -s)
DOMAIN=$(hostname -d)
function create_config() {
    echo "partitionId: %d"
    echo "partitionGroup:"
    echo "  name: %s"
    echo "  namespace: %s"
    echo "controller:"
    echo "  id: %s"
    echo "  host: %s"
    echo "  port: 5679"
    echo "node:"
    echo "  id: $NAME-$ORD"
    echo "  host: $NAME-$ORD.$DOMAIN"
    echo "  port: 5678"
    echo "protocol:"
    echo "  type: primaryBackup"
}
if [[ $HOST =~ (.*)-([0-9]+)$ ]]; then
    NAME=${BASH_REMATCH[1]}
    ORD=${BASH_REMATCH[2]}
else
    echo "Failed to parse name and ordinal of Pod"
    exit 1
fi
create_config`, id, group, partition.Namespace, GetControllerName(), getControllerServiceDnsName())
}

// newLogInitConfigMapScript returns a new script for generating a Raft configuration
func newLogInitConfigMapScript(partition *v1alpha1.Partition) string {
	id, _ := getPartitionIdFromAnnotation(partition)
	group, _ := getPartitionGroupFromAnnotation(partition)
	return fmt.Sprintf(`
#!/usr/bin/env bash
HOST=$(hostname -s)
DOMAIN=$(hostname -d)
function create_config() {
    echo "partitionId: %d"
    echo "partitionGroup:"
    echo "  name: %s"
    echo "  namespace: %s"
    echo "controller:"
    echo "  id: %s"
    echo "  host: %s"
    echo "  port: 5679"
    echo "node:"
    echo "  id: $NAME-$ORD"
    echo "  host: $NAME-$ORD.$DOMAIN"
    echo "  port: 5678"
    echo "protocol:"
    echo "  type: log"
    echo "  storage:"
    echo "    directory: /var/lib/atomix"
}
if [[ $HOST =~ (.*)-([0-9]+)$ ]]; then
    NAME=${BASH_REMATCH[1]}
    ORD=${BASH_REMATCH[2]}
else
    echo "Failed to parse name and ordinal of Pod"
    exit 1
fi
create_config`, id, group, partition.Namespace, GetControllerName(), getControllerServiceDnsName())
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

// NewPartitionService returns a new service for a partition group
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
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Selector:                 partition.Labels,
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
			},
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Selector:                 partition.Labels,
		},
	}
}

// NewPartitionConfigMap returns a new StatefulSet for a partition group
func NewPartitionStatefulSet(partition *v1alpha1.Partition) (*appsv1.StatefulSet, error) {
	if partition.Spec.Raft != nil {
		return newPersistentPartitionStatefulSet(partition, &partition.Spec.Raft.Storage)
	} else if partition.Spec.Log != nil {
		return newPersistentPartitionStatefulSet(partition, &partition.Spec.Log.Storage)
	} else if partition.Spec.PrimaryBackup != nil {
		return newEphemeralPartitionStatefulSet(partition)
	}
	return nil, nil
}

// newEphemeralPartitionStatefulSet returns a new StatefulSet for a persistent partition group
func newEphemeralPartitionStatefulSet(partition *v1alpha1.Partition) (*appsv1.StatefulSet, error) {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionStatefulSetName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: GetPartitionServiceName(partition),
			Replicas:    &partition.Spec.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: partition.Labels,
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
					InitContainers: newInitContainers(partition.Spec.Size),
					Containers:     newEphemeralContainers(partition.Spec.Version, partition.Spec.Env, partition.Spec.Resources),
					Volumes:        newVolumes(GetPartitionInitConfigMapName(partition), nil),
				},
			},
		},
	}, nil
}

// newPersistentPartitionStatefulSet returns a new StatefulSet for a persistent partition group
func newPersistentPartitionStatefulSet(partition *v1alpha1.Partition, storage *v1alpha1.Storage) (*appsv1.StatefulSet, error) {
	var affinity *corev1.Affinity
	group, err := getPartitionGroupFromAnnotation(partition)
	id, err := getPartitionIdFromAnnotation(partition)
	if group != "" && id != 0 {
		affinity = newAffinity(group, id)
	}

	claims, err := newPersistentVolumeClaims(storage.ClassName, storage.Size)
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
			ServiceName: GetPartitionServiceName(partition),
			Replicas:    &partition.Spec.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: partition.Labels,
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
					Affinity:       affinity,
					InitContainers: newInitContainers(partition.Spec.Size),
					Containers:     newPersistentContainers(partition.Spec.Version, partition.Spec.Env, partition.Spec.Resources),
					Volumes:        newVolumes(GetPartitionInitConfigMapName(partition), storage.ClassName),
				},
			},
			VolumeClaimTemplates: claims,
		},
	}, err
}
