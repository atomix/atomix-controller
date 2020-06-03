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
	membershipapi "github.com/atomix/api/proto/atomix/membership"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
)

func registerMembershipServiceServer(s *grpc.Server, srv membershipapi.MembershipServiceServer) {
	membershipapi.RegisterMembershipServiceServer(s, srv)
}

// JoinGroup handles a group join request
func (c *Controller) JoinGroup(request *membershipapi.JoinGroupRequest, stream membershipapi.MembershipService_JoinGroupServer) error {
	log.Info("Received JoinGroupRequest", "Request", request)

	ch := make(chan membershipapi.JoinGroupResponse)
	key := uuid.New().String()
	c.mu.Lock()
	membersOut, ok := c.membershipResponsesOut[request.GroupID.String()]
	if !ok {
		membersOut = make(map[string]chan<- membershipapi.JoinGroupResponse)
		c.membershipResponsesOut[request.GroupID.String()] = membersOut
	}
	membersOut[key] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		membersOut, ok := c.membershipResponsesOut[request.GroupID.String()]
		if ok {
			delete(membersOut, key)
			if len(membersOut) == 0 {
				delete(c.membershipResponsesOut, request.GroupID.String())
			}
		}
		c.mu.Unlock()
	}()

	// If no member was added, send an initial response to acknowledge the stream
	var initialResponse *membershipapi.JoinGroupResponse
	if request.Member == nil {
		// Get the set of members in the member's scope
		memberList := &v1beta3.MemberList{}
		memberListFields := map[string]string{
			"properties.namespace": request.GroupID.Name,
		}
		memberListOpts := &client.ListOptions{
			Namespace:     request.GroupID.Namespace,
			FieldSelector: fields.SelectorFromSet(memberListFields),
		}
		err := c.client.List(stream.Context(), memberList, memberListOpts)
		if err != nil {
			return err
		}

		// Create a list of members that have not been deleted
		members := make([]membershipapi.Member, 0, len(memberList.Items))
		for _, member := range memberList.Items {
			if member.DeletionTimestamp == nil {
				members = append(members, membershipapi.Member{
					ID: membershipapi.MemberId{
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
		initialResponse = &membershipapi.JoinGroupResponse{
			GroupID: membershipapi.GroupId{
				Namespace: request.GroupID.Namespace,
				Name:      request.GroupID.Name,
			},
			Members: members,
		}

		// Send the initial response
		log.Info("Sending JoinGroupResponse", "Response", initialResponse)
		err = stream.Send(initialResponse)
		if err != nil {
			return err
		}
	} else {
		log.Info("Joining Member to group", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
		// Get the pod joining the group
		pod := &corev1.Pod{}
		name := types.NamespacedName{
			Namespace: request.Member.ID.Namespace,
			Name:      request.Member.ID.Name,
		}
		err := c.client.Get(stream.Context(), name, pod)
		if err != nil {
			log.Error(err, "Failed to join Member to group", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
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
					Namespace: request.GroupID.Name,
					Name:      request.Member.ID.Name,
				},
				Service: request.Member.Host,
				Port:    intstr.FromInt(int(request.Member.Port)),
			},
		}

		// Create the member
		err = c.client.Create(stream.Context(), member)
		if err != nil && !errors.IsAlreadyExists(err) {
			log.Error(err, "Failed to join Member to group", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			return err
		}
	}

	go func() {
		<-stream.Context().Done()
		if request.Member != nil {
			log.Info("Leaving Member from group", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			member := &v1beta3.Member{}
			name := types.NamespacedName{
				Namespace: request.Member.ID.Namespace,
				Name:      request.Member.ID.Name,
			}
			err := c.client.Get(context.TODO(), name, member)
			if err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to leave Member from group", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
			} else if err == nil {
				err = c.client.Delete(context.TODO(), member)
				if err != nil && !errors.IsNotFound(err) {
					log.Error(err, "Failed to leave Member from group", "Namespace", request.Member.ID.Namespace, "Name", request.Member.ID.Name)
				}
			}
		}
		close(ch)
	}()

	// Process response changes
	var lastResponse membershipapi.JoinGroupResponse
	if initialResponse != nil {
		lastResponse = *initialResponse
	}
	for response := range ch {
		if response.String() != lastResponse.String() {
			log.Info("Sending JoinGroupResponse", "Response", response)
			err := stream.Send(&response)
			if err != nil {
				log.Error(err, "An error occurred in the membership response stream")
			}
			lastResponse = response
		}
	}
	return nil
}
