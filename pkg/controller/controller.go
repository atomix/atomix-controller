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

package controller

import (
	"context"
	"errors"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/partition"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/partitiongroup"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/util"
	"github.com/atomix/atomix-k8s-controller/proto/atomix/controller"
	partitionpb "github.com/atomix/atomix-k8s-controller/proto/atomix/partition"
	"google.golang.org/grpc"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller_atomix")

// AddController adds the Atomix contrller to the controller manager
func AddController(mgr manager.Manager) error {
	c := NewController(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig())
	err := mgr.Add(c)
	if err != nil {
		return err
	}

	if err = partition.Add(mgr); err != nil {
		return err
	}
	if err = partitiongroup.Add(mgr); err != nil {
		return err
	}
	return nil
}

// NewController creates a new controller server
func NewController(client client.Client, scheme *runtime.Scheme, config *rest.Config, opts ...grpc.ServerOption) *AtomixController {
	return &AtomixController{
		client: client,
		scheme: scheme,
		config: config,
		opts:   opts,
	}
}

// Controller server
type AtomixController struct {
	controller.ControllerServiceServer

	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	opts   []grpc.ServerOption
}

// CreatePartitionGroup creates a partition group via the k8s API
func (c *AtomixController) CreatePartitionGroup(ctx context.Context, r *controller.CreatePartitionGroupRequest) (*controller.CreatePartitionGroupResponse, error) {
	group := &v1alpha1.PartitionGroup{}
	name := util.GetPartitionGroupNamespacedName(r.Id)

	err := c.client.Get(ctx, name, group)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			group = util.NewPartitionGroup(r.Id, r.Spec)
		}
		return nil, err
	}
	return &controller.CreatePartitionGroupResponse{}, nil
}

// DeletePartitionGroup deletes a partition group via the k8s API
func (c *AtomixController) DeletePartitionGroup(ctx context.Context, r *controller.DeletePartitionGroupRequest) (*controller.DeletePartitionGroupResponse, error) {
	group := &v1alpha1.PartitionGroup{}
	name := util.GetPartitionGroupNamespacedName(r.Id)

	if err := c.client.Get(ctx, name, group); err != nil {
		return nil, err
	}

	if err := c.client.Delete(ctx, group); err != nil {
		return nil, err
	}
	return &controller.DeletePartitionGroupResponse{}, nil
}

// GetPartitionGroups returns a list of partition groups read from the k8s API
func (c *AtomixController) GetPartitionGroups(ctx context.Context, r *controller.GetPartitionGroupsRequest) (*controller.GetPartitionGroupsResponse, error) {
	if r.Id.Name != "" {
		group := &v1alpha1.PartitionGroup{}
		name := util.GetPartitionGroupNamespacedName(r.Id)
		err := c.client.Get(context.TODO(), name, group)
		if err != nil && !k8serrors.IsNotFound(err) {
			return nil, err
		}

		proto, err := util.NewPartitionGroupProto(group)
		if err != nil {
			return nil, err
		}

		return &controller.GetPartitionGroupsResponse{
			Groups: []*partitionpb.PartitionGroup{proto},
		}, nil
	} else {
		groups := &v1alpha1.PartitionGroupList{}

		opts := &client.ListOptions{
			Namespace:     util.GetPartitionGroupNamespace(r.Id),
			LabelSelector: labels.SelectorFromSet(util.GetControllerLabels()),
		}

		if err := c.client.List(ctx, opts, groups); err != nil {
			return nil, err
		}

		pbgroups := make([]*partitionpb.PartitionGroup, len(groups.Items))
		for _, group := range groups.Items {
			pbgroup, err := util.NewPartitionGroupProto(&group)
			if err != nil {
				return nil, err
			}
			pbgroups = append(pbgroups, pbgroup)
		}

		return &controller.GetPartitionGroupsResponse{
			Groups: pbgroups,
		}, nil
	}
}

// EnterElection is unimplemented
func (c *AtomixController) EnterElection(r *controller.PartitionElectionRequest, s controller.ControllerService_EnterElectionServer) error {
	return errors.New("Not implemented")
}

// Start starts the controller server
func (c *AtomixController) Start(stop <-chan struct{}) error {
	errs := make(chan error)

	lis, err := net.Listen("tcp", "0.0.0.0:5679")
	if err != nil {
		return err
	}

	s := grpc.NewServer(c.opts...)
	go func() {
		controller.RegisterControllerServiceServer(s, c)
		if err := s.Serve(lis); err != nil {
			errs <- err
		}
	}()

	go func() {
		<-stop
		s.Stop()
	}()
	return nil
}
