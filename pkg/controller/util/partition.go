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
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	return &v1alpha1.Partition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionName(group, partition),
			Namespace: group.Namespace,
			Labels:    newPartitionLabels(group, partition),
		},
		Spec: v1alpha1.PartitionSpec{
			Controller: group.Spec.Controller,
			Version:    group.Spec.Version,
			Size:       int32(group.Spec.PartitionSize),
		},
	}
}

// newPartitionLabels returns a new labels map containing the partition app
func newPartitionLabels(group *v1alpha1.PartitionGroup, partition int) map[string]string {
	return map[string]string{
		AppKey:        AtomixApp,
		ControllerKey: GetControllerNamespacedName().String(),
		TypeKey:       PartitionType,
		GroupKey:      group.Name,
		PartitionKey:  string(partition),
	}
}

func GetPartitionServiceName(group *v1alpha1.Partition) string {
	return group.Name
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

func GetPartitionSystemConfigMapName(partition *v1alpha1.Partition) string {
	return getPartitionResourceName(partition, ConfigSuffix)
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

// getPartitionControllerServiceName returns the DNS service for the partition controller
func getPartitionControllerServiceName(partition *v1alpha1.Partition) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", partition.Spec.Controller.Name, partition.Spec.Controller.Namespace)
}

// newRaftInitConfigMapScript returns a new script for generating a Raft configuration
func newRaftInitConfigMapScript(partition *v1alpha1.Partition) string {
	return fmt.Sprintf(`
#!/usr/bin/env bash
DOMAIN=$(hostname -d)
REPLICAS=$1
function create_config() {
    echo "controller:"
    echo "  service: %s"
    echo "dns:"
    echo "  service: %s"
    echo "node:"
    echo "  id: $NAME-$ORD"
    echo "  host: $NAME-$ORD.$DOMAIN"
    echo "  port: 5678"
    echo "protocol:"
    echo "  type: raft"
    echo "  config:"
    echo "    memberId: $NAME-$ORD"
    echo "    members:"
    for (( i=0; i<$REPLICAS; i++ ))
    do
        echo "      - $NAME-$((i))"
    done
}
if [[ $HOST =~ (.*)-([0-9]+)$ ]]; then
    NAME=${BASH_REMATCH[1]}
    ORD=${BASH_REMATCH[2]}
else
    echo "Failed to parse name and ordinal of Pod"
    exit 1
fi
create_config`, getPartitionControllerServiceName(partition), GetPartitionServiceName(partition))
}

// newPrimaryBackupInitConfigMapScript returns a new script for generating a Raft configuration
func newPrimaryBackupInitConfigMapScript(partition *v1alpha1.Partition) string {
	return fmt.Sprintf(`
#!/usr/bin/env bash
DOMAIN=$(hostname -d)
function create_config() {
    echo "controller:"
    echo "  service: %s"
    echo "dns:"
    echo "  service: %s"
    echo "node:"
    echo "  id: $NAME-$ORD"
    echo "  host: $NAME-$ORD.$DOMAIN"
    echo "  port: 5678"
    echo "protocol:"
    echo "  type: primary-backup"
}
if [[ $HOST =~ (.*)-([0-9]+)$ ]]; then
    NAME=${BASH_REMATCH[1]}
    ORD=${BASH_REMATCH[2]}
else
    echo "Failed to parse name and ordinal of Pod"
    exit 1
fi
create_config`, getPartitionControllerServiceName(partition), GetPartitionServiceName(partition))
}

// newLogInitConfigMapScript returns a new script for generating a Raft configuration
func newLogInitConfigMapScript(partition *v1alpha1.Partition) string {
	return fmt.Sprintf(`
#!/usr/bin/env bash
DOMAIN=$(hostname -d)
function create_config() {
    echo "controller:"
    echo "  service: %s"
    echo "dns:"
    echo "  service: %s"
    echo "node:"
    echo "  id: $NAME-$ORD"
    echo "  host: $NAME-$ORD.$DOMAIN"
    echo "  port: 5678"
    echo "protocol:"
    echo "  type: log"
}
if [[ $HOST =~ (.*)-([0-9]+)$ ]]; then
    NAME=${BASH_REMATCH[1]}
    ORD=${BASH_REMATCH[2]}
else
    echo "Failed to parse name and ordinal of Pod"
    exit 1
fi
create_config`, getPartitionControllerServiceName(partition), GetPartitionServiceName(partition))
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
					Name: "node",
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
			Name:      GetPartitionServiceName(partition),
			Namespace: partition.Namespace,
			Labels:    partition.Labels,
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "node",
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
					Volumes: []corev1.Volume{
						newInitScriptsVolume(GetPartitionInitConfigMapName(partition)),
						newUserConfigVolume(GetPartitionSystemConfigMapName(partition)),
						newSystemConfigVolume(),
					},
				},
			},
		},
	}, nil
}

// newPersistentPartitionStatefulSet returns a new StatefulSet for a persistent partition group
func newPersistentPartitionStatefulSet(partition *v1alpha1.Partition, storage *v1alpha1.Storage) (*appsv1.StatefulSet, error) {
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
					Affinity:       newAffinity(GetPartitionStatefulSetName(partition)),
					InitContainers: newInitContainers(partition.Spec.Size),
					Containers:     newPersistentContainers(partition.Spec.Version, partition.Spec.Env, partition.Spec.Resources),
					Volumes: []corev1.Volume{
						newInitScriptsVolume(GetPartitionInitConfigMapName(partition)),
						newUserConfigVolume(GetPartitionSystemConfigMapName(partition)),
						newSystemConfigVolume(),
					},
				},
			},
			VolumeClaimTemplates: claims,
		},
	}, err
}
