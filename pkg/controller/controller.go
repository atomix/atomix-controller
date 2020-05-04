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
	"fmt"
	"github.com/atomix/api/proto/atomix/cluster"
	databaseapi "github.com/atomix/api/proto/atomix/database"
	gossipapi "github.com/atomix/api/proto/atomix/gossip"
	pbapi "github.com/atomix/api/proto/atomix/pb"
	primitiveapi "github.com/atomix/api/proto/atomix/primitive"
	protocolapi "github.com/atomix/api/proto/atomix/protocol"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/atomix/kubernetes-controller/pkg/controller/database"
	"github.com/atomix/kubernetes-controller/pkg/controller/member"
	"github.com/atomix/kubernetes-controller/pkg/controller/membershipgroup"
	"github.com/atomix/kubernetes-controller/pkg/controller/partitiongroup"
	"github.com/atomix/kubernetes-controller/pkg/controller/util/k8s"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	clusterResponseCh := make(chan cluster.JoinClusterResponse)
	gossipGroupResponseCh := make(chan gossipapi.JoinGossipGroupResponse)
	replicaGroupResponseCh := make(chan pbapi.JoinReplicaGroupResponse)

	c := newController(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), clusterResponseCh, gossipGroupResponseCh, replicaGroupResponseCh)
	err := mgr.Add(c)
	if err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.Member{}, "properties.namespace", func(rawObj runtime.Object) []string {
		member := rawObj.(*v1beta3.Member)
		return []string{member.Properties.Namespace}
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

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.Primitive{}, "properties.type", func(rawObj runtime.Object) []string {
		primitive := rawObj.(*v1beta3.Primitive)
		return []string{string(primitive.Properties.Type)}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.Primitive{}, "properties.protocol", func(rawObj runtime.Object) []string {
		primitive := rawObj.(*v1beta3.Primitive)
		return []string{primitive.Properties.Protocol}
	}); err != nil {
		return err
	}

	if err = database.Add(mgr); err != nil {
		return err
	}
	if err = member.Add(mgr, clusterResponseCh); err != nil {
		return err
	}
	if err = membershipgroup.Add(mgr, gossipGroupResponseCh); err != nil {
		return err
	}
	if err = partitiongroup.Add(mgr, replicaGroupResponseCh); err != nil {
		return err
	}
	return nil
}

// newController creates a new controller server
func newController(
	client client.Client,
	scheme *runtime.Scheme,
	config *rest.Config,
	clusterResponseCh chan cluster.JoinClusterResponse,
	gossipGroupResponseCh chan gossipapi.JoinGossipGroupResponse,
	replicaGroupResponseCh chan pbapi.JoinReplicaGroupResponse,
	opts ...grpc.ServerOption) *Controller {
	return &Controller{
		client:                   client,
		scheme:                   scheme,
		config:                   config,
		clusterResponseIn:        clusterResponseCh,
		clusterResponsesOut:      make(map[string]map[string]chan<- cluster.JoinClusterResponse),
		gossipGroupResponseIn:    gossipGroupResponseCh,
		gossipGroupResponsesOut:  make(map[string]map[string]chan<- gossipapi.JoinGossipGroupResponse),
		replicaGroupResponseIn:   replicaGroupResponseCh,
		replicaGroupResponsesOut: make(map[string]map[string]chan<- pbapi.JoinReplicaGroupResponse),
		opts:                     opts,
	}
}

// Controller an implementation of the Atomix controller API
type Controller struct {
	client                   client.Client
	scheme                   *runtime.Scheme
	config                   *rest.Config
	opts                     []grpc.ServerOption
	clusterResponseIn        chan cluster.JoinClusterResponse
	clusterResponsesOut      map[string]map[string]chan<- cluster.JoinClusterResponse
	gossipGroupResponseIn    chan gossipapi.JoinGossipGroupResponse
	gossipGroupResponsesOut  map[string]map[string]chan<- gossipapi.JoinGossipGroupResponse
	replicaGroupResponseIn   chan pbapi.JoinReplicaGroupResponse
	replicaGroupResponsesOut map[string]map[string]chan<- pbapi.JoinReplicaGroupResponse
	mu                       sync.RWMutex
}

func (c *Controller) JoinCluster(request *cluster.JoinClusterRequest, stream cluster.ClusterService_JoinClusterServer) error {
	log.Info("Received JoinClusterRequest", "Request", request)

	ch := make(chan cluster.JoinClusterResponse)
	key := uuid.New().String()
	c.mu.Lock()
	membersOut, ok := c.clusterResponsesOut[request.ClusterID.String()]
	if !ok {
		membersOut = make(map[string]chan<- cluster.JoinClusterResponse)
		c.clusterResponsesOut[request.ClusterID.String()] = membersOut
	}
	membersOut[key] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		membersOut, ok := c.clusterResponsesOut[request.ClusterID.String()]
		if ok {
			delete(membersOut, key)
			if len(membersOut) == 0 {
				delete(c.clusterResponsesOut, request.ClusterID.String())
			}
		}
		c.mu.Unlock()
	}()

	// If no member was added, send an initial response to acknowledge the stream
	var initialResponse *cluster.JoinClusterResponse
	if request.Member == nil {
		// Get the set of members in the member's scope
		memberList := &v1beta3.MemberList{}
		memberListFields := map[string]string{
			"properties.namespace": request.ClusterID.Name,
		}
		memberListOpts := &client.ListOptions{
			Namespace:     request.ClusterID.Namespace,
			FieldSelector: fields.SelectorFromSet(memberListFields),
		}
		err := c.client.List(context.TODO(), memberList, memberListOpts)
		if err != nil {
			return err
		}

		// Create a list of members that have not been deleted
		members := make([]cluster.Member, 0, len(memberList.Items))
		for _, member := range memberList.Items {
			if member.DeletionTimestamp == nil {
				members = append(members, cluster.Member{
					ID: cluster.MemberId{
						Name:      member.Properties.Name,
						Namespace: member.Properties.Namespace,
					},
					Host: member.Properties.Service,
					Port: member.Properties.Port.IntVal,
				})
			}
		}

		// Sort the membership to aid in deduplicating responses
		sort.Slice(members, func(i, j int) bool {
			return members[i].ID.Name < members[j].ID.Name
		})

		// Construct a membership response
		initialResponse = &cluster.JoinClusterResponse{
			ClusterID: cluster.ClusterId{
				Namespace: request.ClusterID.Namespace,
				Name:      request.ClusterID.Name,
			},
			Members: members,
		}

		// Send the initial response
		err = stream.Send(initialResponse)
		if err != nil {
			return err
		}
	} else {
		log.Info("Joining Member to cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
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

		owner := metav1.OwnerReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       pod.ObjectMeta.Name,
			UID:        pod.ObjectMeta.UID,
		}

		// Create the member
		member := &v1beta3.Member{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       request.Member.ID.Namespace,
				Name:            request.Member.ID.Name,
				OwnerReferences: []metav1.OwnerReference{owner},
				Finalizers:      []string{"event"},
			},
			Properties: v1beta3.MemberProperties{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: request.ClusterID.Name,
					Name:      request.Member.ID.Name,
				},
				Service: request.Member.Host,
				Port:    intstr.FromInt(int(request.Member.Port)),
			},
		}

		// Create the member
		err = c.client.Create(stream.Context(), member)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			log.Error(err, "Failed to join Member to cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			return err
		}
	}

	go func() {
		<-stream.Context().Done()
		if request.Member != nil {
			log.Info("Leaving Member from cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			member := &v1beta3.Member{}
			name := types.NamespacedName{
				Namespace: request.Member.ID.Namespace,
				Name:      request.Member.Host,
			}
			err := c.client.Get(context.TODO(), name, member)
			if err != nil && !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to leave Member from cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			} else {
				err = c.client.Delete(context.TODO(), member)
				if err != nil && !k8serrors.IsNotFound(err) {
					log.Error(err, "Failed to leave Member from cluster", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
				}
			}
		}
		close(ch)
	}()

	// Process response changes
	var lastResponse cluster.JoinClusterResponse
	if initialResponse != nil {
		lastResponse = *initialResponse
	}
	for response := range ch {
		if response.String() != lastResponse.String() {
			log.Info("Sending JoinClusterResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the membership response stream")
			}
			lastResponse = response
		}
	}
	return nil
}

func (c *Controller) JoinGossipGroup(request *gossipapi.JoinGossipGroupRequest, stream gossipapi.GossipService_JoinGossipGroupServer) error {
	log.Info("Received JoinMembershipGroupRequest", "Request", request)

	ch := make(chan gossipapi.JoinGossipGroupResponse)
	key := uuid.New().String()
	c.mu.Lock()
	membershipGroupsOut, ok := c.gossipGroupResponsesOut[request.GroupID.String()]
	if !ok {
		membershipGroupsOut = make(map[string]chan<- gossipapi.JoinGossipGroupResponse)
		c.gossipGroupResponsesOut[request.GroupID.String()] = membershipGroupsOut
	}
	membershipGroupsOut[key] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		membershipGroupsOut, ok := c.gossipGroupResponsesOut[request.GroupID.String()]
		if ok {
			delete(membershipGroupsOut, key)
			if len(membershipGroupsOut) == 0 {
				delete(c.gossipGroupResponsesOut, request.GroupID.String())
			}
		}
		c.mu.Unlock()
	}()

	// If no member was added, send an initial response to acknowledge the stream
	var initialResponse *gossipapi.JoinGossipGroupResponse
	if request.MemberID == nil {
		// Get the membership group
		membershipGroup := &v1beta3.MembershipGroup{}
		membershipGroupName := types.NamespacedName{
			Namespace: request.GroupID.Namespace,
			Name:      request.GroupID.Name,
		}
		err := c.client.Get(context.TODO(), membershipGroupName, membershipGroup)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		} else if err != nil {
			initialResponse = &gossipapi.JoinGossipGroupResponse{
				Group: gossipapi.GossipGroup{
					ID:      request.GroupID,
					Members: []gossipapi.Member{},
				},
			}
		} else {
			// Create the protocol if necessary
			protocol := &v1beta3.Protocol{}
			protocolName := types.NamespacedName{
				Namespace: request.GroupID.Namespace,
				Name:      request.GroupID.Name,
			}
			err := c.client.Get(context.TODO(), protocolName, protocol)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					return err
				}
				protocolOwner := metav1.OwnerReference{
					APIVersion: membershipGroup.APIVersion,
					Kind:       membershipGroup.Kind,
					Name:       membershipGroup.ObjectMeta.Name,
					UID:        membershipGroup.ObjectMeta.UID,
				}
				protocol = &v1beta3.Protocol{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       request.GroupID.Namespace,
						Name:            request.GroupID.Name,
						OwnerReferences: []metav1.OwnerReference{protocolOwner},
					},
					Type: v1beta3.ProtocolType{
						Gossip: &v1beta3.GossipProtocolType{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: request.GroupID.Namespace,
								Name:      request.GroupID.Name,
							},
						},
					},
				}
				err := c.client.Create(context.TODO(), protocol)
				if err != nil && !k8serrors.IsAlreadyExists(err) {
					return err
				}
			}

			// Get the list of members in the group
			membershipList := &v1beta3.MembershipList{}
			membershipListOpts := &client.ListOptions{
				Namespace: request.GroupID.Namespace,
				FieldSelector: fields.SelectorFromSet(map[string]string{
					"bind.group": request.GroupID.Name,
				}),
			}
			err = c.client.List(context.TODO(), membershipList, membershipListOpts)
			if err != nil {
				return err
			}

			// Construct response membership from the set of members that have not been deleted
			responseMembers := make([]gossipapi.Member, 0, len(membershipList.Items))
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
					responseMembers = append(responseMembers, gossipapi.Member{
						ID: gossipapi.MemberId{
							Name:      member.Properties.Name,
							Namespace: member.Properties.Namespace,
						},
						Host: member.Properties.Service,
						Port: member.Properties.Port.IntVal,
					})
				}
			}

			// Sort the membership to aid in deduplicating responses
			sort.Slice(responseMembers, func(i, j int) bool {
				return responseMembers[i].ID.Name < responseMembers[j].ID.Name
			})

			// Construct a membership response
			initialResponse = &gossipapi.JoinGossipGroupResponse{
				Group: gossipapi.GossipGroup{
					ID: gossipapi.GossipGroupId{
						Namespace: membershipGroup.Namespace,
						Name:      membershipGroup.Name,
					},
					Members: responseMembers,
				},
			}
		}

		// Send the initial response
		err = stream.Send(initialResponse)
		if err != nil {
			return err
		}
	} else {
		log.Info("Joining Member to MembershipGroup", "Namespace", request.MemberID.Namespace, "Name", request.MemberID.Name)
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

		owner := metav1.OwnerReference{
			APIVersion: member.APIVersion,
			Kind:       member.Kind,
			Name:       member.ObjectMeta.Name,
			UID:        member.ObjectMeta.UID,
		}

		// Create the group membership
		membership := &v1beta3.Membership{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       request.GroupID.Namespace,
				Name:            fmt.Sprintf("%s-%s", request.MemberID.Name, request.GroupID.Name),
				OwnerReferences: []metav1.OwnerReference{owner},
				Finalizers:      []string{"event"},
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
	}

	go func() {
		<-stream.Context().Done()
		if request.MemberID != nil {
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
		}
		close(ch)
	}()

	// Process response changes
	var lastResponse gossipapi.JoinGossipGroupResponse
	if initialResponse != nil {
		lastResponse = *initialResponse
	}
	for response := range ch {
		if response.String() != lastResponse.String() {
			log.Info("Sending JoinMembershipGroupResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the membership group response stream")
			}
			lastResponse = response
		}
	}
	return nil
}

func (c *Controller) JoinReplicaGroup(request *pbapi.JoinReplicaGroupRequest, stream pbapi.ReplicaGroupService_JoinReplicaGroupServer) error {
	log.Info("Received JoinPartitionGroupRequest", "Request", request)

	ch := make(chan pbapi.JoinReplicaGroupResponse)
	key := uuid.New().String()
	c.mu.Lock()
	partitionGroupsOut, ok := c.replicaGroupResponsesOut[request.GroupID.String()]
	if !ok {
		partitionGroupsOut = make(map[string]chan<- pbapi.JoinReplicaGroupResponse)
		c.replicaGroupResponsesOut[request.GroupID.String()] = partitionGroupsOut
	}
	partitionGroupsOut[key] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		partitionGroupsOut, ok := c.replicaGroupResponsesOut[request.GroupID.String()]
		if ok {
			delete(partitionGroupsOut, key)
			if len(partitionGroupsOut) == 0 {
				delete(c.replicaGroupResponsesOut, request.GroupID.String())
			}
		}
		c.mu.Unlock()
	}()

	// If no member was added, send an initial response to acknowledge the stream
	var initialResponse *pbapi.JoinReplicaGroupResponse
	if request.ReplicaID == nil {
		// Get the partition group
		partitionGroup := &v1beta3.PartitionGroup{}
		partitionGroupName := types.NamespacedName{
			Namespace: request.GroupID.Namespace,
			Name:      request.GroupID.Name,
		}
		err := c.client.Get(context.TODO(), partitionGroupName, partitionGroup)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		} else if err != nil {
			initialResponse = &pbapi.JoinReplicaGroupResponse{
				Group: pbapi.ReplicaGroup{
					ID:         request.GroupID,
					Partitions: []pbapi.Partition{},
				},
			}
		} else {
			// Create the protocol if necessary
			protocol := &v1beta3.Protocol{}
			protocolName := types.NamespacedName{
				Namespace: request.GroupID.Namespace,
				Name:      request.GroupID.Name,
			}
			err := c.client.Get(context.TODO(), protocolName, protocol)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					return err
				}
				protocolOwner := metav1.OwnerReference{
					APIVersion: partitionGroup.APIVersion,
					Kind:       partitionGroup.Kind,
					Name:       partitionGroup.ObjectMeta.Name,
					UID:        partitionGroup.ObjectMeta.UID,
				}
				protocol = &v1beta3.Protocol{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       request.GroupID.Namespace,
						Name:            request.GroupID.Name,
						OwnerReferences: []metav1.OwnerReference{protocolOwner},
					},
					Type: v1beta3.ProtocolType{
						PrimaryBackup: &v1beta3.PrimaryBackupProtocolType{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: request.GroupID.Namespace,
								Name:      request.GroupID.Name,
							},
						},
					},
				}
				err := c.client.Create(context.TODO(), protocol)
				if err != nil && !k8serrors.IsAlreadyExists(err) {
					return err
				}
			}

			// Get the list of partition group members
			partitionGroupMemberships := &v1beta3.PartitionGroupMembershipList{}
			partitionGroupMembershipFields := map[string]string{
				"bind.group": partitionGroup.Name,
			}
			listOpts := &client.ListOptions{Namespace: partitionGroup.Namespace, FieldSelector: fields.SelectorFromSet(partitionGroupMembershipFields)}
			err = c.client.List(context.TODO(), partitionGroupMemberships, listOpts)
			if err != nil {
				return err
			}

			// Construct a set of partition group members
			partitionGroupMembers := make(map[string]bool)
			for _, partitionGroupMembership := range partitionGroupMemberships.Items {
				partitionGroupMembers[partitionGroupMembership.Bind.Member] = true
			}

			numPartitions := int(partitionGroup.Spec.Partitions)
			partitions := make([]pbapi.Partition, 0)
			for partition := 1; partition <= numPartitions; partition++ {
				membershipGroup := &v1beta3.MembershipGroup{}
				membershipGroupName := types.NamespacedName{
					Namespace: partitionGroup.Namespace,
					Name:      fmt.Sprintf("%s-%d", partitionGroup.Name, partition),
				}
				err = c.client.Get(context.TODO(), membershipGroupName, membershipGroup)
				if err != nil {
					return err
				}

				// Get the list of members in the group
				membershipList := &v1beta3.MembershipList{}
				membershipListOpts := &client.ListOptions{
					Namespace: membershipGroup.Namespace,
					FieldSelector: fields.SelectorFromSet(map[string]string{
						"bind.group": membershipGroup.Name,
					}),
				}
				err = c.client.List(context.TODO(), membershipList, membershipListOpts)
				if err != nil {
					return err
				}

				// Create a set of active members
				members := make(map[string]bool)
				for _, membership := range membershipList.Items {
					if membership.DeletionTimestamp == nil {
						members[membership.Bind.Member] = true
					}
				}

				// Construct a response leader/term
				responseTerm := pbapi.Term(membershipGroup.Status.Term)
				var responseLeader *pbapi.ReplicaId
				if membershipGroup.Status.Leader != "" {
					leaderMember := &v1beta3.Member{}
					leaderMemberName := types.NamespacedName{
						Namespace: membershipGroup.Namespace,
						Name:      membershipGroup.Status.Leader,
					}
					err := c.client.Get(context.TODO(), leaderMemberName, leaderMember)
					if err == nil {
						responseLeader = &pbapi.ReplicaId{
							Namespace: leaderMember.Properties.Namespace,
							Name:      leaderMember.Properties.Name,
						}
					} else if !k8serrors.IsNotFound(err) {
						return err
					}
				}

				// Construct response membership from the set of members that have not been deleted
				responseReplicas := make([]pbapi.Replica, 0, len(membershipList.Items))
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
						responseReplicas = append(responseReplicas, pbapi.Replica{
							ID: pbapi.ReplicaId{
								Name:      member.Properties.Name,
								Namespace: member.Properties.Namespace,
							},
							Host: member.Properties.Service,
							Port: member.Properties.Port.IntVal,
						})
					}
				}

				// Sort the membership to aid in deduplicating responses
				sort.Slice(responseReplicas, func(i, j int) bool {
					return responseReplicas[i].ID.Name < responseReplicas[j].ID.Name
				})

				partitions = append(partitions, pbapi.Partition{
					ID: pbapi.PartitionId{
						Namespace: membershipGroup.Namespace,
						Name:      membershipGroup.Name,
						Index:     uint32(partition),
					},
					Term:     responseTerm,
					Leader:   responseLeader,
					Replicas: responseReplicas,
				})
			}

			// Sort the membership groups to aid in deduplicating responses
			sort.Slice(partitions, func(i, j int) bool {
				return partitions[i].ID.Name < partitions[j].ID.Name
			})

			// Construct a partition group response
			initialResponse = &pbapi.JoinReplicaGroupResponse{
				Group: pbapi.ReplicaGroup{
					ID: pbapi.ReplicaGroupId{
						Namespace: partitionGroup.Namespace,
						Name:      partitionGroup.Name,
					},
					Partitions: partitions,
				},
			}
		}

		// Send the initial response
		err = stream.Send(initialResponse)
		if err != nil {
			return err
		}
	} else {
		log.Info("Joining Member to PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
		// Get the member joining the group
		member := &v1beta3.Member{}
		memberName := types.NamespacedName{
			Namespace: request.ReplicaID.Namespace,
			Name:      request.ReplicaID.Name,
		}
		err := c.client.Get(stream.Context(), memberName, member)
		if err != nil {
			log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
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
				log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
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
				log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
				return err
			}
		}

		owner := metav1.OwnerReference{
			APIVersion: member.APIVersion,
			Kind:       member.Kind,
			Name:       member.ObjectMeta.Name,
			UID:        member.ObjectMeta.UID,
		}

		// Create the partition group membership
		partitionGroupMembership := &v1beta3.PartitionGroupMembership{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       request.GroupID.Namespace,
				Name:            fmt.Sprintf("%s-%s", request.ReplicaID.Name, request.GroupID.Name),
				OwnerReferences: []metav1.OwnerReference{owner},
				Finalizers:      []string{"event"},
			},
			Bind: v1beta3.PartitionGroupMembershipBinding{
				Member: request.ReplicaID.Name,
				Group:  request.GroupID.Name,
			},
		}

		// Create the partition group membership
		err = c.client.Create(stream.Context(), partitionGroupMembership)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			log.Error(err, "Failed to join Member to PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
			return err
		}
	}

	go func() {
		<-stream.Context().Done()
		if request.ReplicaID != nil {
			partitionGroupMembership := &v1beta3.PartitionGroupMembership{}
			name := types.NamespacedName{
				Namespace: request.GroupID.Namespace,
				Name:      fmt.Sprintf("%s-%s", request.ReplicaID.Name, request.GroupID.Name),
			}
			err := c.client.Get(context.TODO(), name, partitionGroupMembership)
			if err != nil && !k8serrors.IsNotFound(err) {
				log.Error(err, "Failed to leave Member from PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
			} else {
				err = c.client.Delete(context.TODO(), partitionGroupMembership)
				if err != nil && !k8serrors.IsNotFound(err) {
					log.Error(err, "Failed to leave Member from PartitionGroup", "Namespace", request.ReplicaID.Namespace, "Member", request.ReplicaID.Name, "PartitionGroup", request.GroupID.Name)
				}
			}
		}
		close(ch)
	}()

	// Process response changes
	var lastResponse pbapi.JoinReplicaGroupResponse
	if initialResponse != nil {
		lastResponse = *initialResponse
	}
	for response := range ch {
		if response.String() != lastResponse.String() {
			log.Info("Sending JoinPartitionGroupResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the partition group response stream")
			}
			lastResponse = response
		}
	}
	return nil
}

// GetDatabases get a list of databases managed by the controller
func (c *Controller) GetDatabases(ctx context.Context, request *databaseapi.GetDatabasesRequest) (*databaseapi.GetDatabasesResponse, error) {
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

				pbpartitions := make([]*databaseapi.Partition, 0, len(partitions.Items))
				for _, partition := range partitions.Items {
					pbpartition, err := k8s.NewPartitionProto(&partition)
					if err != nil {
						return nil, err
					}
					pbpartitions = append(pbpartitions, pbpartition)
				}
				pbdatabase.Partitions = pbpartitions
				return &databaseapi.GetDatabasesResponse{
					Databases: []*databaseapi.Database{pbdatabase},
				}, nil
			}
		}
		return &databaseapi.GetDatabasesResponse{}, nil
	}

	pbdatabases := make([]*databaseapi.Database, 0, len(databases.Items))
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

		pbpartitions := make([]*databaseapi.Partition, 0, len(partitions.Items))
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

	return &databaseapi.GetDatabasesResponse{
		Databases: pbdatabases,
	}, nil
}

func getPrimitiveNamespacedName(protocol protocolapi.ProtocolId, primitive primitiveapi.PrimitiveId) types.NamespacedName {
	return types.NamespacedName{
		Namespace: getPrimitiveNamespace(protocol, primitive),
		Name:      getPrimitiveName(protocol, primitive),
	}
}

func getPrimitiveNamespace(protocol protocolapi.ProtocolId, primitive primitiveapi.PrimitiveId) string {
	return protocol.Namespace
}

func getPrimitiveName(protocol protocolapi.ProtocolId, primitive primitiveapi.PrimitiveId) string {
	return fmt.Sprintf("%s-%s-%s", protocol.Name, primitive.Namespace, primitive.Name)
}

func (c *Controller) CreatePrimitive(ctx context.Context, request *primitiveapi.CreatePrimitiveRequest) (*primitiveapi.CreatePrimitiveResponse, error) {
	protocol := &v1beta3.Protocol{}
	protocolName := types.NamespacedName{
		Namespace: request.Protocol.Namespace,
		Name:      request.Protocol.Name,
	}
	err := c.client.Get(ctx, protocolName, protocol)
	if err != nil {
		return nil, err
	}

	primitive := &v1beta3.Primitive{}
	primitiveName := getPrimitiveNamespacedName(request.Protocol, request.Primitive)
	err = c.client.Get(ctx, primitiveName, primitive)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}

		primitiveOwner := metav1.OwnerReference{
			APIVersion: protocol.APIVersion,
			Kind:       protocol.Kind,
			Name:       protocol.ObjectMeta.Name,
			UID:        protocol.ObjectMeta.UID,
		}

		primitive = &v1beta3.Primitive{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       getPrimitiveNamespace(request.Protocol, request.Primitive),
				Name:            getPrimitiveName(request.Protocol, request.Primitive),
				OwnerReferences: []metav1.OwnerReference{primitiveOwner},
			},
			Properties: v1beta3.PrimitiveProperties{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: request.Primitive.Namespace,
					Name:      request.Primitive.Name,
				},
				Type:     v1beta3.GetPrimitiveType(request.Type),
				Protocol: protocol.Name,
			},
		}
		err = c.client.Create(ctx, primitive)
		if err != nil {
			return nil, err
		}
	}
	return &primitiveapi.CreatePrimitiveResponse{
		Primitive: primitiveapi.PrimitiveMetadata{
			Protocol: protocolapi.ProtocolId{
				Namespace: primitive.Namespace,
				Name:      primitive.Properties.Protocol,
			},
			Primitive: primitiveapi.PrimitiveId{
				Namespace: primitive.Properties.Namespace,
				Name:      primitive.Properties.Name,
			},
			Type: primitive.Properties.Type.Proto(),
		},
	}, nil
}

func (c *Controller) GetPrimitive(ctx context.Context, request *primitiveapi.GetPrimitiveRequest) (*primitiveapi.GetPrimitiveResponse, error) {
	protocol := &v1beta3.Protocol{}
	protocolName := types.NamespacedName{
		Namespace: request.Protocol.Namespace,
		Name:      request.Protocol.Name,
	}
	err := c.client.Get(ctx, protocolName, protocol)
	if err != nil {
		return nil, err
	}

	primitive := &v1beta3.Primitive{}
	primitiveName := getPrimitiveNamespacedName(request.Protocol, request.Primitive)
	err = c.client.Get(ctx, primitiveName, primitive)
	if err != nil {
		return nil, err
	}
	return &primitiveapi.GetPrimitiveResponse{
		Primitive: primitiveapi.PrimitiveMetadata{
			Protocol: protocolapi.ProtocolId{
				Namespace: primitive.Namespace,
				Name:      primitive.Properties.Protocol,
			},
			Primitive: primitiveapi.PrimitiveId{
				Namespace: primitive.Properties.Namespace,
				Name:      primitive.Properties.Name,
			},
			Type: primitive.Properties.Type.Proto(),
		},
	}, nil
}

func (c *Controller) GetPrimitives(ctx context.Context, request *primitiveapi.GetPrimitivesRequest) (*primitiveapi.GetPrimitivesResponse, error) {
	if request.Protocol != nil && request.Type != primitiveapi.PrimitiveType_UNKNOWN && request.Primitive != nil {
		protocol := &v1beta3.Protocol{}
		protocolName := types.NamespacedName{
			Namespace: request.Protocol.Namespace,
			Name:      request.Protocol.Name,
		}
		err := c.client.Get(ctx, protocolName, protocol)
		if err != nil {
			return nil, err
		}

		primitive := &v1beta3.Primitive{}
		primitiveName := getPrimitiveNamespacedName(*request.Protocol, *request.Primitive)
		err = c.client.Get(ctx, primitiveName, primitive)
		if err != nil {
			return nil, err
		}

		if primitive.Properties.Type.Proto() != request.Type {
			return nil, status.Error(codes.NotFound, "primitive not found")
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: []primitiveapi.PrimitiveMetadata{
				{
					Protocol: protocolapi.ProtocolId{
						Namespace: primitive.Namespace,
						Name:      primitive.Properties.Protocol,
					},
					Primitive: primitiveapi.PrimitiveId{
						Namespace: primitive.Properties.Namespace,
						Name:      primitive.Properties.Name,
					},
					Type: primitive.Properties.Type.Proto(),
				},
			},
		}, nil
	} else if request.Protocol != nil && request.Type != primitiveapi.PrimitiveType_UNKNOWN {
		if request.Protocol.Name != "" {
			protocol := &v1beta3.Protocol{}
			protocolName := types.NamespacedName{
				Namespace: request.Protocol.Namespace,
				Name:      request.Protocol.Name,
			}
			err := c.client.Get(ctx, protocolName, protocol)
			if err != nil {
				return nil, err
			}
		}

		primitivesList := &v1beta3.PrimitiveList{}
		primitivesListFields := map[string]string{
			"properties.type": string(v1beta3.GetPrimitiveType(request.Type)),
		}
		if request.Protocol.Name != "" {
			primitivesListFields["properties.protocol"] = request.Protocol.Name
		}
		primitivesListOpts := &client.ListOptions{
			Namespace:     request.Protocol.Namespace,
			FieldSelector: fields.SelectorFromSet(primitivesListFields),
		}
		err := c.client.List(ctx, primitivesList, primitivesListOpts)
		if err != nil {
			return nil, err
		}
		primitives := make([]primitiveapi.PrimitiveMetadata, 0, len(primitivesList.Items))
		for _, primitive := range primitivesList.Items {
			primitives = append(primitives, primitiveapi.PrimitiveMetadata{
				Protocol: protocolapi.ProtocolId{
					Namespace: primitive.Namespace,
					Name:      primitive.Properties.Protocol,
				},
				Primitive: primitiveapi.PrimitiveId{
					Namespace: primitive.Properties.Namespace,
					Name:      primitive.Properties.Name,
				},
				Type: primitive.Properties.Type.Proto(),
			})
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: primitives,
		}, nil
	} else if request.Protocol != nil {
		if request.Protocol.Name != "" {
			protocol := &v1beta3.Protocol{}
			protocolName := types.NamespacedName{
				Namespace: request.Protocol.Namespace,
				Name:      request.Protocol.Name,
			}
			err := c.client.Get(ctx, protocolName, protocol)
			if err != nil {
				return nil, err
			}
		}

		primitivesList := &v1beta3.PrimitiveList{}
		primitivesListFields := map[string]string{}
		if request.Protocol.Name != "" {
			primitivesListFields["properties.protocol"] = request.Protocol.Name
		}
		primitivesListOpts := &client.ListOptions{
			Namespace:     request.Protocol.Namespace,
			FieldSelector: fields.SelectorFromSet(primitivesListFields),
		}
		err := c.client.List(ctx, primitivesList, primitivesListOpts)
		if err != nil {
			return nil, err
		}
		primitives := make([]primitiveapi.PrimitiveMetadata, 0, len(primitivesList.Items))
		for _, primitive := range primitivesList.Items {
			primitives = append(primitives, primitiveapi.PrimitiveMetadata{
				Protocol: protocolapi.ProtocolId{
					Namespace: primitive.Namespace,
					Name:      primitive.Properties.Protocol,
				},
				Primitive: primitiveapi.PrimitiveId{
					Namespace: primitive.Properties.Namespace,
					Name:      primitive.Properties.Name,
				},
				Type: primitive.Properties.Type.Proto(),
			})
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: primitives,
		}, nil
	} else {
		primitivesList := &v1beta3.PrimitiveList{}
		err := c.client.List(ctx, primitivesList)
		if err != nil {
			return nil, err
		}
		primitives := make([]primitiveapi.PrimitiveMetadata, 0, len(primitivesList.Items))
		for _, primitive := range primitivesList.Items {
			primitives = append(primitives, primitiveapi.PrimitiveMetadata{
				Protocol: protocolapi.ProtocolId{
					Namespace: primitive.Namespace,
					Name:      primitive.Properties.Protocol,
				},
				Primitive: primitiveapi.PrimitiveId{
					Namespace: primitive.Properties.Namespace,
					Name:      primitive.Properties.Name,
				},
				Type: primitive.Properties.Type.Proto(),
			})
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: primitives,
		}, nil
	}
}

func (c *Controller) DeletePrimitive(ctx context.Context, request *primitiveapi.DeletePrimitiveRequest) (*primitiveapi.DeletePrimitiveResponse, error) {
	protocol := &v1beta3.Protocol{}
	protocolName := types.NamespacedName{
		Namespace: request.Protocol.Namespace,
		Name:      request.Protocol.Name,
	}
	err := c.client.Get(ctx, protocolName, protocol)
	if err != nil {
		return nil, err
	}

	primitive := &v1beta3.Primitive{}
	primitiveName := getPrimitiveNamespacedName(request.Protocol, request.Primitive)
	err = c.client.Get(ctx, primitiveName, primitive)
	if err != nil {
		return nil, err
	}

	err = c.client.Delete(ctx, primitive)
	if err != nil {
		return nil, err
	}
	return &primitiveapi.DeletePrimitiveResponse{
		Primitive: primitiveapi.PrimitiveMetadata{
			Protocol: protocolapi.ProtocolId{
				Namespace: primitive.Namespace,
				Name:      primitive.Properties.Protocol,
			},
			Primitive: primitiveapi.PrimitiveId{
				Namespace: primitive.Properties.Namespace,
				Name:      primitive.Properties.Name,
			},
			Type: primitive.Properties.Type.Proto(),
		},
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
		databaseapi.RegisterDatabaseServiceServer(s, c)
		cluster.RegisterClusterServiceServer(s, c)
		gossipapi.RegisterGossipServiceServer(s, c)
		pbapi.RegisterReplicaGroupServiceServer(s, c)
		if err := s.Serve(lis); err != nil {
			errs <- err
		}
	}()

	go c.processClusterResponses(stop)
	go c.processGossipGroupResponses(stop)
	go c.processReplicaGroupResponses(stop)

	select {
	case e := <-errs:
		return e
	case <-stop:
		log.Info("Stopping controller server")
		s.Stop()
		return nil
	}
}

func (c *Controller) processClusterResponses(stop <-chan struct{}) {
	go func() {
		<-stop
		close(c.clusterResponseIn)
	}()
	for response := range c.clusterResponseIn {
		c.mu.RLock()
		responseChs, ok := c.clusterResponsesOut[response.ClusterID.String()]
		if ok {
			for _, responseCh := range responseChs {
				responseCh <- response
			}
		}
		c.mu.RUnlock()
	}
}

func (c *Controller) processGossipGroupResponses(stop <-chan struct{}) {
	go func() {
		<-stop
		close(c.clusterResponseIn)
	}()
	for response := range c.gossipGroupResponseIn {
		c.mu.RLock()
		responseChs, ok := c.gossipGroupResponsesOut[response.Group.ID.String()]
		if ok {
			for _, responseCh := range responseChs {
				responseCh <- response
			}
		}
		c.mu.RUnlock()
	}
}

func (c *Controller) processReplicaGroupResponses(stop <-chan struct{}) {
	go func() {
		<-stop
		close(c.clusterResponseIn)
	}()
	for response := range c.replicaGroupResponseIn {
		c.mu.RLock()
		responseChs, ok := c.replicaGroupResponsesOut[response.Group.ID.String()]
		if ok {
			for _, responseCh := range responseChs {
				responseCh <- response
			}
		}
		c.mu.RUnlock()
	}
}

var _ databaseapi.DatabaseServiceServer = &Controller{}
var _ cluster.ClusterServiceServer = &Controller{}
var _ gossipapi.GossipServiceServer = &Controller{}
var _ pbapi.ReplicaGroupServiceServer = &Controller{}
var _ primitiveapi.PrimitiveServiceServer = &Controller{}
