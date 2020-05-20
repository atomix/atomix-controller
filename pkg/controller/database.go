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

package controller

import (
	"context"
	"errors"
	databaseapi "github.com/atomix/api/proto/atomix/database"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	"google.golang.org/grpc"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func registerDatabaseServiceServer(s *grpc.Server, srv databaseapi.DatabaseServiceServer) {
	databaseapi.RegisterDatabaseServiceServer(s, srv)
}

// GetDatabase get a database managed by the controller
func (c *Controller) GetDatabase(ctx context.Context, request *databaseapi.GetDatabaseRequest) (*databaseapi.GetDatabaseResponse, error) {
	database := &v1beta3.Database{}
	name := types.NamespacedName{
		Namespace: k8s.GetDatabaseNamespace(request.ID),
		Name:      request.ID.Name,
	}
	err := c.client.Get(ctx, name, database)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.New("database not found")
		}
		return nil, err
	}

	pbdatabase := k8s.NewDatabaseProto(database)
	options := &client.ListOptions{
		Namespace:     k8s.GetDatabaseNamespace(request.ID),
		LabelSelector: labels.SelectorFromSet(k8s.GetPartitionLabelsForDatabase(database)),
	}
	partitions := &v1beta3.PartitionList{}
	err = c.client.List(context.TODO(), partitions, options)
	if err != nil {
		return nil, err
	}

	if len(partitions.Items) != int(database.Spec.Partitions) {
		return nil, errors.New("database not found")
	}

	pbpartitions := make([]databaseapi.Partition, 0, len(partitions.Items))
	for _, partition := range partitions.Items {
		pbpartitions = append(pbpartitions, *k8s.NewPartitionProto(&partition))
	}
	pbdatabase.Partitions = pbpartitions
	return &databaseapi.GetDatabaseResponse{
		Database: pbdatabase,
	}, nil
}

// GetDatabases get a list of databases managed by the controller
func (c *Controller) GetDatabases(ctx context.Context, request *databaseapi.GetDatabasesRequest) (*databaseapi.GetDatabasesResponse, error) {
	databases := &v1beta3.DatabaseList{}

	opts := &client.ListOptions{
		Namespace: k8s.GetDatabaseNamespace(databaseapi.DatabaseId{Namespace: request.Namespace}),
	}

	if err := c.client.List(ctx, databases, opts); err != nil {
		return nil, err
	}

	pbdatabases := make([]databaseapi.Database, 0, len(databases.Items))
	for _, database := range databases.Items {
		pbdatabase := k8s.NewDatabaseProto(&database)

		options := &client.ListOptions{
			Namespace:     k8s.GetDatabaseNamespace(databaseapi.DatabaseId{Namespace: request.Namespace}),
			LabelSelector: labels.SelectorFromSet(k8s.GetPartitionLabelsForDatabase(&database)),
		}
		partitions := &v1beta3.PartitionList{}
		err := c.client.List(context.TODO(), partitions, options)
		if err != nil {
			return nil, err
		}

		if len(partitions.Items) != int(database.Spec.Partitions) {
			continue
		}

		pbpartitions := make([]databaseapi.Partition, 0, len(partitions.Items))
		for _, partition := range partitions.Items {
			pbpartitions = append(pbpartitions, *k8s.NewPartitionProto(&partition))
		}
		pbdatabase.Partitions = pbpartitions
		pbdatabases = append(pbdatabases, *pbdatabase)
	}

	return &databaseapi.GetDatabasesResponse{
		Databases: pbdatabases,
	}, nil
}
