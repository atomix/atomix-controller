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

package v1beta3

import (
	"fmt"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strconv"
)

const partitionPort = 5678

// GetPartitionName returns the partition name for the given partition
func GetPartitionName(database *v1beta3.Database, partition int) string {
	return fmt.Sprintf("%s-%d", database.Name, partition)
}

// GetPartitionServiceName returns the service name for the given partition
func GetPartitionServiceName(database *v1beta3.Database, partition int) string {
	return GetPartitionName(database, partition)
}

// GetPartitionNamespacedName returns the NamespacedName for the given partition
func GetPartitionNamespacedName(database *v1beta3.Database, partition int) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionName(database, partition),
		Namespace: database.Namespace,
	}
}

// NewPartition returns the configuration for the given partition
func NewPartition(database *v1beta3.Database, partition int) *v1beta3.Partition {
	return &v1beta3.Partition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetPartitionName(database, partition),
			Namespace:   database.Namespace,
			Labels:      newPartitionLabels(database, partition),
			Annotations: newPartitionAnnotations(database, partition),
		},
		Spec: v1beta3.PartitionSpec{
			PartitionID: int32(partition),
			ServiceName: GetPartitionServiceName(database, partition),
		},
	}
}

// GetPartitionLabelsForDatabase returns the labels for the partitions in the given database
func GetPartitionLabelsForDatabase(database *v1beta3.Database) map[string]string {
	return map[string]string{
		appKey:      atomixApp,
		typeKey:     partitionType,
		databaseKey: database.Name,
	}
}

// newPartitionLabels returns a new labels map containing the partition app
func newPartitionLabels(database *v1beta3.Database, partition int) map[string]string {
	labels := GetPartitionLabelsForDatabase(database)
	labels[partitionKey] = fmt.Sprint(partition)
	return labels
}

// newPartitionAnnotations returns annotations for the given partition
func newPartitionAnnotations(database *v1beta3.Database, partition int) map[string]string {
	return map[string]string{
		controllerAnnotation: GetQualifiedControllerName(),
		typeAnnotation:       partitionType,
		databaseAnnotation:   database.Name,
		partitionAnnotation:  fmt.Sprint(partition),
	}
}

// GetPartitionIDFromPartitionAnnotations returns the partition ID from the given partition annotations
func GetPartitionIDFromPartitionAnnotations(partition *v1beta3.Partition) (int32, error) {
	idstr, ok := partition.Annotations[partitionAnnotation]
	if !ok {
		return 1, nil
	}

	id, err := strconv.ParseInt(idstr, 0, 32)
	if err != nil {
		return 0, err
	}
	return int32(id), nil
}
