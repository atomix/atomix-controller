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
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta2"
	"k8s.io/apimachinery/pkg/types"
	"strconv"
)

// GetClusterName returns the cluster name for the given cluster
func GetClusterName(database *v1beta2.Database, cluster int) string {
	return fmt.Sprintf("%s-%d", database.Name, cluster)
}

// GetClusterNamespacedName returns the NamespacedName for the given cluster
func GetClusterNamespacedName(database *v1beta2.Database, cluster int) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetClusterName(database, cluster),
		Namespace: database.Namespace,
	}
}

// NewCluster returns the configuration for the given cluster
func NewCluster(database *v1beta2.Database, cluster int) *v1beta2.Cluster {
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
	return &v1beta2.Cluster{
		ObjectMeta: meta,
		Spec:       database.Spec.Template.Spec,
	}
}

// GetClusterLabelsForDatabase returns the labels for the clusters in the given database
func GetClusterLabelsForDatabase(database *v1beta2.Database) map[string]string {
	return map[string]string{
		appKey:      atomixApp,
		typeKey:     clusterType,
		databaseKey: database.Name,
	}
}

// newClusterLabels returns a new labels map containing the cluster app
func newClusterLabels(database *v1beta2.Database, cluster int) map[string]string {
	labels := GetClusterLabelsForDatabase(database)
	labels[clusterKey] = fmt.Sprint(cluster)
	return labels
}

// newClusterAnnotations returns annotations for the given cluster
func newClusterAnnotations(database *v1beta2.Database, cluster int) map[string]string {
	return map[string]string{
		controllerAnnotation: GetQualifiedControllerName(),
		typeAnnotation:       clusterType,
		databaseAnnotation:   database.Name,
		clusterAnnotation:    fmt.Sprint(cluster),
	}
}

// GetClusterIDFromClusterAnnotations returns the cluster ID from the given cluster annotations
func GetClusterIDFromClusterAnnotations(cluster *v1beta2.Cluster) (int32, error) {
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
