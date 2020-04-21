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
	api "github.com/atomix/api/proto/atomix/controller"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/atomix/kubernetes-controller/pkg/controller/database"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var log = logf.Log.WithName("controller_atomix")

// AddController adds the Atomix controller to the k8s controller manager
func AddController(mgr manager.Manager) error {
	c := newController(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig())
	err := mgr.Add(c)
	if err != nil {
		return err
	}

	if err = database.Add(mgr); err != nil {
		return err
	}
	return nil
}

// newController creates a new controller server
func newController(client client.Client, scheme *runtime.Scheme, config *rest.Config, opts ...grpc.ServerOption) *Controller {
	return &Controller{
		client: client,
		scheme: scheme,
		config: config,
		opts:   opts,
	}
}

// Controller an implementation of the Atomix controller API
type Controller struct {
	api.ControllerServiceServer
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	opts   []grpc.ServerOption
}

// GetDatabases get a list of databases managed by the controller
func (c *Controller) GetDatabases(ctx context.Context, request *api.GetDatabasesRequest) (*api.GetDatabasesResponse, error) {
	databases := &v1beta3.DatabaseList{}

	opts := &client.ListOptions{
		Namespace: k8s.GetDatabaseNamespace(request.ID),
	}

	if err := c.client.List(ctx, databases, opts); err != nil {
		return nil, err
	}

	if request.ID != nil && request.ID.Name != "" {
		for _, database := range databases.Items {
			if database.Name == request.ID.Name {
				pbdatabase := k8s.NewDatabaseProto(&database)
				options := &client.ListOptions{
					Namespace:     k8s.GetDatabaseNamespace(request.ID),
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

				pbpartitions := make([]*api.Partition, 0, len(partitions.Items))
				for _, partition := range partitions.Items {
					pbpartition, err := k8s.NewPartitionProto(&partition)
					if err != nil {
						return nil, err
					}
					pbpartitions = append(pbpartitions, pbpartition)
				}
				pbdatabase.Partitions = pbpartitions
				return &api.GetDatabasesResponse{
					Databases: []*api.Database{pbdatabase},
				}, nil
			}
		}
		return &api.GetDatabasesResponse{}, nil
	}

	pbdatabases := make([]*api.Database, 0, len(databases.Items))
	for _, database := range databases.Items {
		pbdatabase := k8s.NewDatabaseProto(&database)

		options := &client.ListOptions{
			Namespace:     k8s.GetDatabaseNamespace(request.ID),
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

		pbpartitions := make([]*api.Partition, 0, len(partitions.Items))
		for _, partition := range partitions.Items {
			pbpartition, err := k8s.NewPartitionProto(&partition)
			if err != nil {
				return nil, err
			}
			pbpartitions = append(pbpartitions, pbpartition)
		}
		pbdatabase.Partitions = pbpartitions
		pbdatabases = append(pbdatabases, pbdatabase)
	}

	return &api.GetDatabasesResponse{
		Databases: pbdatabases,
	}, nil
}

// Start starts the controller server
func (c *Controller) Start(stop <-chan struct{}) error {
	errs := make(chan error)

	log.Info("Starting controller server")
	lis, err := net.Listen("tcp", ":5679")
	if err != nil {
		return err
	}

	s := grpc.NewServer(c.opts...)
	go func() {
		api.RegisterControllerServiceServer(s, c)
		if err := s.Serve(lis); err != nil {
			errs <- err
		}
	}()

	select {
	case e := <-errs:
		return e
	case <-stop:
		log.Info("Stopping controller server")
		s.Stop()
		return nil
	}
}
