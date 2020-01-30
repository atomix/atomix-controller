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
	api "github.com/atomix/api/proto/atomix/controller"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

// GetDatabaseName returns the Database name for the given database ID
func GetDatabaseName(id *api.DatabaseId) string {
	return id.Name
}

// GetDatabaseNamespace returns the Database namespace for the given database ID
func GetDatabaseNamespace(id *api.DatabaseId) string {
	if id.Namespace != "" {
		return id.Namespace
	}
	return defaultNamespace
}

// GetDatabaseNamespacedName returns the NamespacedName for the given database ID
func GetDatabaseNamespacedName(id *api.DatabaseId) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetDatabaseName(id),
		Namespace: GetDatabaseNamespace(id),
	}
}

// GetDatabaseServiceName returns the service name for a database
func GetDatabaseServiceName(database *v1beta1.Database) string {
	return database.Name
}

// GetDatabaseServiceNamespacedName returns the namespaced service name for a database
func GetDatabaseServiceNamespacedName(database *v1beta1.Database) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetDatabaseServiceName(database),
		Namespace: database.Namespace,
	}
}

// NewPartitionProto returns the partition proto message for the given Partition
func NewPartitionProto(p *v1beta1.Partition) (*api.Partition, error) {
	return &api.Partition{
		PartitionID: int32(p.Spec.PartitionID),
		Endpoints: []*api.PartitionEndpoint{
			{
				Host: p.Spec.Host,
				Port: p.Spec.Port,
			},
		},
	}, nil
}

// NewDatabaseProto returns a Database proto message for the given Database
func NewDatabaseProto(database *v1beta1.Database) *api.Database {
	return &api.Database{
		ID: &api.DatabaseId{
			Name:      database.Name,
			Namespace: database.Namespace,
		},
	}
}
