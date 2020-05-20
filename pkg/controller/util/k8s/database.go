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
	databaseapi "github.com/atomix/api/proto/atomix/database"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
)

// GetDatabaseNamespace returns the Database namespace for the given database ID
func GetDatabaseNamespace(id databaseapi.DatabaseId) string {
	if id.Namespace != "" {
		return id.Namespace
	}
	return defaultNamespace
}

// NewPartitionProto returns the partition proto message for the given Partition
func NewPartitionProto(p *v1beta3.Partition) *databaseapi.Partition {
	host := fmt.Sprintf("%s.%s.svc.cluster.local", p.Spec.ServiceName, p.Namespace)
	return &databaseapi.Partition{
		PartitionID: databaseapi.PartitionId{
			Partition: p.Spec.PartitionID,
		},
		Endpoints: []databaseapi.PartitionEndpoint{
			{
				Host: host,
				Port: partitionPort,
			},
		},
	}
}

// NewDatabaseProto returns a Database proto message for the given Database
func NewDatabaseProto(database *v1beta3.Database) *databaseapi.Database {
	return &databaseapi.Database{
		ID: databaseapi.DatabaseId{
			Name:      database.Name,
			Namespace: database.Namespace,
		},
	}
}
