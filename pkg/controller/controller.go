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
	"fmt"
	api "github.com/atomix/api/proto/atomix/controller"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/atomix/kubernetes-controller/pkg/controller/database"
	"github.com/atomix/kubernetes-controller/pkg/controller/member"
	"github.com/atomix/kubernetes-controller/pkg/controller/membership"
	"github.com/atomix/kubernetes-controller/pkg/controller/membershipgroup"
	"github.com/atomix/kubernetes-controller/pkg/controller/partitiongroup"
	"github.com/atomix/kubernetes-controller/pkg/controller/partitiongroupmembership"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
)

var log = logf.Log.WithName("controller_atomix")

const numPartitions = 32

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


	if err = member.Add(mgr); err != nil {
		return err
	}
	if err = membershipgroup.Add(mgr); err != nil {
		return err
	}
	if err = membership.Add(mgr); err != nil {
		return err
	}
	if err = partitiongroup.Add(mgr); err != nil {
		return err
	}
	if err = partitiongroupmembership.Add(mgr); err != nil {
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
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	opts   []grpc.ServerOption
	mu     sync.Mutex
}

func (c *Controller) JoinCluster(request *api.JoinClusterRequest, stream api.ClusterService_JoinClusterServer) error {
	// Get the pod joining the cluster
	pod := &corev1.Pod{}
	name := types.NamespacedName{
		Namespace: request.Member.ID.Namespace,
		Name:      request.Member.ID.Name,
	}
	err := c.client.Get(stream.Context(), name, pod)
	if err != nil {
		return err
	}

	// Create the member
	member := &v1beta3.Member{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: request.Member.ID.Namespace,
			Name:      request.Member.ID.Name,
		},
		Service: request.Member.Host,
		Port:    intstr.FromInt(int(request.Member.Port)),
	}

	// Create the member
	err = c.client.Create(stream.Context(), member)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	// TODO: Handle membership events
	return nil
}

func (c *Controller) JoinPartitionGroup(request *api.JoinPartitionGroupRequest, stream api.PartitionGroupService_JoinPartitionGroupServer) error {
	if request.MemberID.Namespace != request.GroupID.Namespace {
		return errors.New("cannot join group in another namespace")
	}

	// Get the member joining the group
	member := &v1beta3.Member{}
	memberName := types.NamespacedName{
		Namespace: request.MemberID.Namespace,
		Name:      request.MemberID.Name,
	}
	err := c.client.Get(stream.Context(), memberName, member)
	if err != nil {
		return err
	}

	// Create the partition group if necessary
	partitionGroup := &v1beta3.PartitionGroup{}
	partitionGroupName := types.NamespacedName{
		Namespace: request.GroupID.Namespace,
		Name:      request.GroupID.Name,
	}
	err = c.client.Get(stream.Context(), partitionGroupName, partitionGroup)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		partitionGroup = &v1beta3.PartitionGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.GroupID.Namespace,
				Name:      request.GroupID.Name,
			},
		}
		err = c.client.Create(stream.Context(), partitionGroup)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return err
		}
		err = c.client.Get(stream.Context(), partitionGroupName, partitionGroup)
		if err != nil {
			return err
		}
	}

	// Create the partition group membership
	partitionGroupMembership := &v1beta3.PartitionGroupMembership{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: request.GroupID.Namespace,
			Name:      fmt.Sprintf("%s-%s", request.MemberID.Name, request.GroupID.Name),
		},
		Bind: v1beta3.PartitionGroupMembershipBinding{
			Member: request.MemberID.Name,
			Group:  request.GroupID.Name,
		},
	}

	// Create the partition group membership
	err = c.client.Create(stream.Context(), partitionGroupMembership)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	// TODO: Handle partition group events
	return nil
}

func (c *Controller) JoinMembershipGroup(request *api.JoinMembershipGroupRequest, stream api.MembershipGroupService_JoinMembershipGroupServer) error {
	if request.MemberID.Namespace != request.GroupID.Namespace {
		return errors.New("cannot join group in another namespace")
	}

	// Get the member joining the group
	member := &v1beta3.Member{}
	memberName := types.NamespacedName{
		Namespace: request.MemberID.Namespace,
		Name:      request.MemberID.Name,
	}
	err := c.client.Get(stream.Context(), memberName, member)
	if err != nil {
		return err
	}

	// Create the group if necessary
	membershipGroup := &v1beta3.MembershipGroup{}
	membershipGroupName := types.NamespacedName{
		Namespace: request.GroupID.Namespace,
		Name:      request.GroupID.Name,
	}
	err = c.client.Get(stream.Context(), membershipGroupName, membershipGroup)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		membershipGroup = &v1beta3.MembershipGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.GroupID.Namespace,
				Name:      request.GroupID.Name,
			},
		}
		err = c.client.Create(stream.Context(), membershipGroup)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return err
		}
		err = c.client.Get(stream.Context(), membershipGroupName, membershipGroup)
		if err != nil {
			return err
		}
	}

	// Create the group membership
	membership := &v1beta3.Membership{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: request.GroupID.Namespace,
			Name:      fmt.Sprintf("%s-%s", request.MemberID.Name, request.GroupID.Name),
		},
		Bind: v1beta3.MembershipBinding{
			Member: request.MemberID.Name,
			Group:  request.GroupID.Name,
		},
	}

	// Create the group membership
	err = c.client.Create(stream.Context(), membership)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}
	// TODO: Handle partition group events
	return nil
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
		api.RegisterClusterServiceServer(s, c)
		api.RegisterPartitionGroupServiceServer(s, c)
		api.RegisterMembershipGroupServiceServer(s, c)
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

var _ api.ControllerServiceServer = &Controller{}
var _ api.ClusterServiceServer = &Controller{}
var _ api.PartitionGroupServiceServer = &Controller{}
var _ api.MembershipGroupServiceServer = &Controller{}
