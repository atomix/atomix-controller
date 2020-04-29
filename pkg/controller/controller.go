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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sort"
	"sync"
)

var log = logf.Log.WithName("atomix_controller")

// AddController adds the Atomix controller to the k8s controller manager
func AddController(mgr manager.Manager) error {
	memberCh := make(chan types.NamespacedName)
	membershipGroupCh := make(chan types.NamespacedName)
	partitionGroupCh := make(chan types.NamespacedName)

	c := newController(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), memberCh, membershipGroupCh, partitionGroupCh)
	err := mgr.Add(c)
	if err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.Member{}, "scope", func(rawObj runtime.Object) []string {
		member := rawObj.(*v1beta3.Member)
		return []string{member.Scope}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.Membership{}, "bind.group", func(rawObj runtime.Object) []string {
		membership := rawObj.(*v1beta3.Membership)
		return []string{membership.Bind.Group}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.PartitionGroupMembership{}, "bind.group", func(rawObj runtime.Object) []string {
		membership := rawObj.(*v1beta3.PartitionGroupMembership)
		return []string{membership.Bind.Group}
	}); err != nil {
		return err
	}

	if err = database.Add(mgr); err != nil {
		return err
	}
	if err = member.Add(mgr, memberCh); err != nil {
		return err
	}
	if err = membershipgroup.Add(mgr, membershipGroupCh); err != nil {
		return err
	}
	if err = membership.Add(mgr, membershipGroupCh); err != nil {
		return err
	}
	if err = partitiongroup.Add(mgr, partitionGroupCh); err != nil {
		return err
	}
	if err = partitiongroupmembership.Add(mgr, partitionGroupCh); err != nil {
		return err
	}
	return nil
}

// newController creates a new controller server
func newController(
	client client.Client,
	scheme *runtime.Scheme,
	config *rest.Config,
	memberCh chan types.NamespacedName,
	membershipGroupCh chan types.NamespacedName,
	partitionGroupCh chan types.NamespacedName,
	opts ...grpc.ServerOption) *Controller {
	return &Controller{
		client:              client,
		scheme:              scheme,
		config:              config,
		memberIn:            memberCh,
		membersOut:          make(map[string]map[string]chan<- api.JoinClusterResponse),
		membershipGroupIn:   membershipGroupCh,
		membershipGroupsOut: make(map[string]map[string]chan<- api.JoinMembershipGroupResponse),
		partitionGroupIn:    partitionGroupCh,
		partitionGroupsOut:  make(map[string]map[string]chan<- api.JoinPartitionGroupResponse),
		opts:                opts,
	}
}

// Controller an implementation of the Atomix controller API
type Controller struct {
	client              client.Client
	scheme              *runtime.Scheme
	config              *rest.Config
	opts                []grpc.ServerOption
	memberIn            chan types.NamespacedName
	membersOut          map[string]map[string]chan<- api.JoinClusterResponse
	membershipGroupIn   chan types.NamespacedName
	membershipGroupsOut map[string]map[string]chan<- api.JoinMembershipGroupResponse
	partitionGroupIn    chan types.NamespacedName
	partitionGroupsOut  map[string]map[string]chan<- api.JoinPartitionGroupResponse
	mu                  sync.RWMutex
}

func (c *Controller) JoinCluster(request *api.JoinClusterRequest, stream api.ClusterService_JoinClusterServer) error {
	log.Info("Received JoinClusterRequest", "Request", request)
	log.Info("Joining Member to cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)

	ch := make(chan api.JoinClusterResponse)
	c.mu.Lock()
	membersOut, ok := c.membersOut[request.GroupID.String()]
	if !ok {
		membersOut = make(map[string]chan<- api.JoinClusterResponse)
		c.membersOut[request.GroupID.String()] = membersOut
	}
	membersOut[request.Member.ID.String()] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		membersOut, ok := c.membersOut[request.GroupID.String()]
		if ok {
			delete(membersOut, request.Member.ID.String())
			if len(membersOut) == 0 {
				delete(c.membersOut, request.GroupID.String())
			}
		}
		c.mu.Unlock()
	}()

	// Get the pod joining the cluster
	pod := &corev1.Pod{}
	name := types.NamespacedName{
		Namespace: request.Member.ID.Namespace,
		Name:      request.Member.Host,
	}
	err := c.client.Get(stream.Context(), name, pod)
	if err != nil {
		log.Error(err, "Failed to join Member to cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
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
		Scope:   request.GroupID.Name,
	}

	// Create the member
	err = c.client.Create(stream.Context(), member)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		log.Error(err, "Failed to join Member to cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
		return err
	}

	go func() {
		<-stream.Context().Done()
		log.Info("Leaving Member from cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
		member := &v1beta3.Member{}
		err := c.client.Get(context.TODO(), name, member)
		if err != nil && !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to leave Member from cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
		} else {
			err = c.client.Delete(context.TODO(), member)
			if err != nil && !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to leave Member from cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			}
		}
		close(ch)
	}()

	hasResponse := false
	var lastResponse api.JoinClusterResponse
	for response := range ch {
		if !hasResponse || response.String() != lastResponse.String() {
			log.Info("Sending JoinClusterResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the membership response stream")
			}
			hasResponse = true
			lastResponse = response
		}
	}
	return nil
}

func (c *Controller) JoinPartitionGroup(request *api.JoinPartitionGroupRequest, stream api.PartitionGroupService_JoinPartitionGroupServer) error {
	if request.MemberID.Namespace != request.GroupID.Namespace {
		return errors.New("cannot join group in another namespace")
	}

	log.Info("Received JoinPartitionGroupRequest", "Request", request)
	log.Info("Joining Member to PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)

	ch := make(chan api.JoinPartitionGroupResponse)
	c.mu.Lock()
	partitionGroupsOut, ok := c.partitionGroupsOut[request.GroupID.String()]
	if !ok {
		partitionGroupsOut = make(map[string]chan<- api.JoinPartitionGroupResponse)
		c.partitionGroupsOut[request.GroupID.String()] = partitionGroupsOut
	}
	partitionGroupsOut[request.MemberID.String()] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		partitionGroupsOut, ok := c.partitionGroupsOut[request.GroupID.String()]
		if ok {
			delete(partitionGroupsOut, request.MemberID.String())
			if len(partitionGroupsOut) == 0 {
				delete(c.partitionGroupsOut, request.GroupID.String())
			}
		}
		c.mu.Unlock()
	}()

	// Get the member joining the group
	member := &v1beta3.Member{}
	memberName := types.NamespacedName{
		Namespace: request.MemberID.Namespace,
		Name:      request.MemberID.Name,
	}
	err := c.client.Get(stream.Context(), memberName, member)
	if err != nil {
		log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)
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
			log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)
			return err
		}
		partitionGroup = &v1beta3.PartitionGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: request.GroupID.Namespace,
				Name:      request.GroupID.Name,
			},
			Spec: v1beta3.PartitionGroupSpec{
				Partitions:        int32(request.Partitions),
				ReplicationFactor: int32(request.ReplicationFactor),
			},
		}
		err = c.client.Create(stream.Context(), partitionGroup)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)
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
		log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)
		return err
	}

	go func() {
		<-stream.Context().Done()
		partitionGroupMembership := &v1beta3.PartitionGroupMembership{}
		name := types.NamespacedName{
			Namespace: request.GroupID.Namespace,
			Name:      fmt.Sprintf("%s-%s", request.MemberID.Name, request.GroupID.Name),
		}
		err := c.client.Get(context.TODO(), name, partitionGroupMembership)
		if err != nil && !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to leave Member from PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)
		} else {
			err = c.client.Delete(context.TODO(), partitionGroupMembership)
			if err != nil && !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to leave Member from PartitionGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "PartitionGroup", request.GroupID.Name)
			}
		}
		close(ch)
	}()

	hasResponse := false
	var lastResponse api.JoinPartitionGroupResponse
	for response := range ch {
		// Wait until the member has been added to all partitions
		if !hasResponse {
			skip := false
			for _, partition := range response.Group.Partitions {
				if len(partition.Members) == 0 {
					skip = true
				}
			}
			if skip {
				continue
			}
		}
		if !hasResponse || response.String() != lastResponse.String() {
			log.Info("Sending JoinPartitionGroupResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the partition group response stream")
			}
			hasResponse = true
			lastResponse = response
		}
	}
	return nil
}

func (c *Controller) JoinMembershipGroup(request *api.JoinMembershipGroupRequest, stream api.MembershipGroupService_JoinMembershipGroupServer) error {
	if request.MemberID.Namespace != request.GroupID.Namespace {
		return errors.New("cannot join group in another namespace")
	}

	log.Info("Received JoinMembershipGroupRequest", "Request", request)
	log.Info("Joining Member to MembershipGroup", "Namespace", request.MemberID.Namespace, "Name", request.MemberID.Name)

	ch := make(chan api.JoinMembershipGroupResponse)
	c.mu.Lock()
	membershipGroupsOut, ok := c.membershipGroupsOut[request.GroupID.String()]
	if !ok {
		membershipGroupsOut = make(map[string]chan<- api.JoinMembershipGroupResponse)
		c.membershipGroupsOut[request.GroupID.String()] = membershipGroupsOut
	}
	membershipGroupsOut[request.MemberID.String()] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		membershipGroupsOut, ok := c.membershipGroupsOut[request.GroupID.String()]
		if ok {
			delete(membershipGroupsOut, request.MemberID.String())
			if len(membershipGroupsOut) == 0 {
				delete(c.membershipGroupsOut, request.GroupID.String())
			}
		}
		c.mu.Unlock()
	}()

	// Get the member joining the group
	member := &v1beta3.Member{}
	memberName := types.NamespacedName{
		Namespace: request.MemberID.Namespace,
		Name:      request.MemberID.Name,
	}
	err := c.client.Get(stream.Context(), memberName, member)
	if err != nil {
		log.Error(err, "Failed to join Member to MembershipGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "MembershipGroup", request.GroupID.Name)
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
			log.Error(err, "Failed to join Member to MembershipGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "MembershipGroup", request.GroupID.Name)
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
			log.Error(err, "Failed to join Member to MembershipGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "MembershipGroup", request.GroupID.Name)
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
		log.Error(err, "Failed to join Member to MembershipGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "MembershipGroup", request.GroupID.Name)
		return err
	}

	go func() {
		<-stream.Context().Done()
		membership := &v1beta3.Membership{}
		name := types.NamespacedName{
			Namespace: request.GroupID.Namespace,
			Name:      fmt.Sprintf("%s-%s", request.MemberID.Name, request.GroupID.Name),
		}
		err := c.client.Get(context.TODO(), name, membership)
		if err != nil && !k8serrors.IsNotFound(err) {
			log.Error(err, "Failed to leave Member from MembershipGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "MembershipGroup", request.GroupID.Name)
		} else {
			err = c.client.Delete(context.TODO(), membership)
			if err != nil && !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to leave Member from MembershipGroup", "Namespace", request.MemberID.Namespace, "Member", request.MemberID.Name, "MembershipGroup", request.GroupID.Name)
			}
		}
		close(ch)
	}()

	hasResponse := false
	var lastResponse api.JoinMembershipGroupResponse
	for response := range ch {
		if !hasResponse || response.String() != lastResponse.String() {
			log.Info("Sending JoinMembershipGroupResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the membership group response stream")
			}
			hasResponse = true
			lastResponse = response
		}
	}
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

	go c.processMembers(stop)
	go c.processMembershipGroups(stop)
	go c.processPartitionGroups(stop)

	select {
	case e := <-errs:
		return e
	case <-stop:
		log.Info("Stopping controller server")
		s.Stop()
		return nil
	}
}

func (c *Controller) processMembers(stop <-chan struct{}) {
	go func() {
		<-stop
		close(c.memberIn)
	}()
	for name := range c.memberIn {
		memberList := &v1beta3.MemberList{}
		memberListFields := map[string]string{
			"scope": name.Name,
		}
		memberListOpts := &client.ListOptions{
			Namespace:     name.Namespace,
			FieldSelector: fields.SelectorFromSet(memberListFields),
		}
		err := c.client.List(context.TODO(), memberList, memberListOpts)
		if err != nil {
			log.Error(err, "Failed to process member update", "Namespace", name.Namespace, "Name", name.Name)
			continue
		}

		members := make([]api.Member, 0, len(memberList.Items))
		for _, member := range memberList.Items {
			if member.DeletionTimestamp == nil {
				members = append(members, api.Member{
					ID: api.MemberId{
						Name:      member.Name,
						Namespace: member.Namespace,
					},
					Host: member.Service,
					Port: member.Port.IntVal,
				})
			}
		}
		sort.Slice(members, func(i, j int) bool {
			return members[i].ID.Name < members[j].ID.Name
		})

		response := api.JoinClusterResponse{
			Membership: api.Membership{
				Members: members,
			},
			GroupID: api.MembershipGroupId{
				Namespace: name.Namespace,
				Name:      name.Name,
			},
		}

		group, ok := c.membersOut[response.GroupID.String()]
		if ok {
			for _, ch := range group {
				ch <- response
			}
		}
	}
}

func (c *Controller) processMembershipGroups(stop <-chan struct{}) {
	go func() {
		<-stop
		close(c.memberIn)
	}()
	for name := range c.membershipGroupIn {
		membershipGroup := &v1beta3.MembershipGroup{}
		err := c.client.Get(context.TODO(), name, membershipGroup)
		if err != nil {
			log.Error(err, "Failed to process membership group update", "Namespace", name.Namespace, "Name", name.Name)
			continue
		}

		membershipList := &v1beta3.MembershipList{}
		membershipListFields := map[string]string{
			"bind.group": membershipGroup.Name,
		}
		membershipListOpts := &client.ListOptions{
			Namespace:     membershipGroup.Namespace,
			FieldSelector: fields.SelectorFromSet(membershipListFields),
		}
		err = c.client.List(context.TODO(), membershipList, membershipListOpts)
		if err != nil {
			log.Error(err, "Failed to process membership group update", "Namespace", name.Namespace, "Name", name.Name)
			continue
		}

		members := make([]api.Member, 0, len(membershipList.Items))
		for _, membership := range membershipList.Items {
			if membership.DeletionTimestamp != nil {
				continue
			}
			member := &v1beta3.Member{}
			memberName := types.NamespacedName{
				Namespace: membership.Namespace,
				Name:      membership.Bind.Member,
			}
			err := c.client.Get(context.TODO(), memberName, member)
			if err != nil {
				continue
			}
			if member.DeletionTimestamp == nil {
				members = append(members, api.Member{
					ID: api.MemberId{
						Name:      member.Name,
						Namespace: member.Namespace,
					},
					Host: member.Service,
					Port: member.Port.IntVal,
				})
			}
		}
		sort.Slice(members, func(i, j int) bool {
			return members[i].ID.Name < members[j].ID.Name
		})

		leadership := &v1beta3.Leadership{}
		leadershipName := types.NamespacedName{
			Namespace: membershipGroup.Namespace,
			Name:      membershipGroup.Name,
		}

		var term api.TermID
		var leader *api.MemberId
		err = c.client.Get(context.TODO(), leadershipName, leadership)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to process membership group update", "Namespace", name.Namespace, "Name", name.Name)
				continue
			}
			term = api.TermID(leadership.Term)
			leader = &api.MemberId{
				Namespace: membershipGroup.Namespace,
				Name:      leadership.Leader,
			}
		}

		response := api.JoinMembershipGroupResponse{
			Group: api.MembershipGroup{
				ID: api.MembershipGroupId{
					Namespace: membershipGroup.Namespace,
					Name:      membershipGroup.Name,
				},
				Term:    term,
				Leader:  leader,
				Members: members,
			},
		}

		group, ok := c.membershipGroupsOut[response.Group.ID.String()]
		if ok {
			for _, ch := range group {
				ch <- response
			}
		}
	}
}

func (c *Controller) processPartitionGroups(stop <-chan struct{}) {
	go func() {
		<-stop
		close(c.memberIn)
	}()
	for name := range c.partitionGroupIn {
		partitionGroup := &v1beta3.PartitionGroup{}
		err := c.client.Get(context.TODO(), name, partitionGroup)
		if err != nil {
			log.Error(err, "Failed to process partition group update", "Namespace", name.Namespace, "Name", name.Name)
			continue
		}

		partitions := int(partitionGroup.Spec.Partitions)
		membershipGroups := make([]api.MembershipGroup, 0)
		skip := false
		for partition := 1; partition <= partitions; partition++ {
			membershipGroup := &v1beta3.MembershipGroup{}
			membershipGroupName := types.NamespacedName{
				Namespace: partitionGroup.Namespace,
				Name:      fmt.Sprintf("%s-%d", partitionGroup.Name, partition),
			}
			err = c.client.Get(context.TODO(), membershipGroupName, membershipGroup)
			if err != nil {
				log.Error(err, "Failed to process partition group update", "Namespace", name.Namespace, "Name", name.Name)
				skip = true
				break
			}

			membershipList := &v1beta3.MembershipList{}
			membershipListFields := map[string]string{
				"bind.group": fmt.Sprintf("%s-%d", name.Name, partition),
			}
			membershipListOpts := &client.ListOptions{
				Namespace:     partitionGroup.Namespace,
				FieldSelector: fields.SelectorFromSet(membershipListFields),
			}
			err = c.client.List(context.TODO(), membershipList, membershipListOpts)
			if err != nil {
				log.Error(err, "Failed to process partition group update", "Namespace", name.Namespace, "Name", name.Name)
				skip = true
				break
			}

			members := make([]api.Member, 0, len(membershipList.Items))
			for _, membership := range membershipList.Items {
				if membership.DeletionTimestamp != nil {
					continue
				}
				member := &v1beta3.Member{}
				memberName := types.NamespacedName{
					Namespace: membership.Namespace,
					Name:      membership.Bind.Member,
				}
				err := c.client.Get(context.TODO(), memberName, member)
				if err != nil {
					continue
				}
				if member.DeletionTimestamp == nil {
					members = append(members, api.Member{
						ID: api.MemberId{
							Name:      member.Name,
							Namespace: member.Namespace,
						},
						Host: member.Service,
						Port: member.Port.IntVal,
					})
				}
			}
			sort.Slice(members, func(i, j int) bool {
				return members[i].ID.Name < members[j].ID.Name
			})

			leadership := &v1beta3.Leadership{}
			leadershipName := types.NamespacedName{
				Namespace: partitionGroup.Namespace,
				Name:      partitionGroup.Name,
			}

			var term api.TermID
			var leader *api.MemberId
			err = c.client.Get(context.TODO(), leadershipName, leadership)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					log.Error(err, "Failed to process partition group update", "Namespace", name.Namespace, "Name", name.Name)
					skip = true
					break
				}
				term = api.TermID(leadership.Term)
				leader = &api.MemberId{
					Namespace: partitionGroup.Namespace,
					Name:      leadership.Leader,
				}
			}

			membershipGroups = append(membershipGroups, api.MembershipGroup{
				ID: api.MembershipGroupId{
					Namespace: membershipGroup.Namespace,
					Name:      membershipGroup.Name,
				},
				Term:    term,
				Leader:  leader,
				Members: members,
			})
		}

		if skip {
			continue
		}

		sort.Slice(membershipGroups, func(i, j int) bool {
			return membershipGroups[i].ID.Name < membershipGroups[j].ID.Name
		})

		response := api.JoinPartitionGroupResponse{
			Group: api.PartitionGroup{
				ID: api.PartitionGroupId{
					Namespace: partitionGroup.Namespace,
					Name:      partitionGroup.Name,
				},
				Partitions: membershipGroups,
			},
		}

		group, ok := c.partitionGroupsOut[response.Group.ID.String()]
		if ok {
			for _, ch := range group {
				ch <- response
			}
		}
	}
}

var _ api.ControllerServiceServer = &Controller{}
var _ api.ClusterServiceServer = &Controller{}
var _ api.PartitionGroupServiceServer = &Controller{}
var _ api.MembershipGroupServiceServer = &Controller{}
