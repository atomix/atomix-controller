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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// GetPartitionName returns the partition name for the given cluster
func GetPartitionName(cluster *v1beta1.Cluster, partition int32) string {
	return fmt.Sprintf("%s-%d", cluster.Name, partition)
}

// GetPartitionNamespacedName returns the NamespacedName for the given partition
func GetPartitionNamespacedName(cluster *v1beta1.Cluster, partition int32) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionName(cluster, partition),
		Namespace: cluster.Namespace,
	}
}

// GetPartitionLabelsForDatabase returns the labels for the partitions in the given database
func GetPartitionLabelsForDatabase(database *v1beta1.Database) map[string]string {
	return map[string]string{
		appKey:      atomixApp,
		typeKey:     partitionType,
		databaseKey: database.Name,
	}
}

// GetPartitionLabelsForCluster returns the labels for the partitions in the given cluster
func GetPartitionLabelsForCluster(cluster *v1beta1.Cluster) map[string]string {
	return map[string]string{
		appKey:      atomixApp,
		typeKey:     partitionType,
		databaseKey: cluster.Labels[databaseKey],
		clusterKey:  cluster.Name,
	}
}

// newPartitionLabels returns a new labels map containing the partition metadata
func newPartitionLabels(cluster *v1beta1.Cluster, partition int32) map[string]string {
	labels := GetPartitionLabelsForCluster(cluster)
	labels[partitionKey] = fmt.Sprint(partition)
	return labels
}

// NewPartition returns the configuration for the given partition
func NewPartition(cluster *v1beta1.Cluster, partition int32) *v1beta1.Partition {
	return &v1beta1.Partition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionName(cluster, partition),
			Namespace: cluster.Namespace,
			Labels:    newPartitionLabels(cluster, partition),
		},
		Spec: v1beta1.PartitionSpec{
			PartitionID: partition,
			Host:        cluster.Name,
			Port:        5678,
		},
	}
}
