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

package v1beta1

// SetDatabaseDefaults sets the defaults for the given Database
func SetDatabaseDefaults(database *Database) {
	if database.Spec.Clusters == 0 {
		database.Spec.Clusters = 1
	}
	if database.Spec.Partitions == 0 {
		database.Spec.Partitions = 1
	}
}

// SetClusterDefaults sets the default values for the given Cluster
func SetClusterDefaults(cluster *Cluster) {
	if cluster.Spec.Backend.Replicas == 0 {
		cluster.Spec.Backend.Replicas = 1
	}
}
