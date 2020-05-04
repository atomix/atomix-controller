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

package partitiongroup

import (
	"context"
	"fmt"
	"github.com/atomix/api/proto/atomix/pb"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sort"
)

var log = logf.Log.WithName("partition_group_controller")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, responseCh chan<- pb.JoinReplicaGroupResponse) error {
	r := &Reconciler{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		config:     mgr.GetConfig(),
		responseCh: responseCh,
	}

	// Create a new controller
	c, err := controller.New("partition-group-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource PartitionGroup
	err = c.Watch(&source.Kind{Type: &v1beta3.PartitionGroup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource PartitionGroupMembership and requeue the PartitionGroup
	err = c.Watch(&source.Kind{Type: &v1beta3.PartitionGroupMembership{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &partitionGroupMembershipMapper{
			client: mgr.GetClient(),
		},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles a PartitionGroup object
type Reconciler struct {
	client     client.Client
	scheme     *runtime.Scheme
	config     *rest.Config
	responseCh chan<- pb.JoinReplicaGroupResponse
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling PartitionGroup")

	// Fetch the PartitionGroup instance
	partitionGroup := &v1beta3.PartitionGroup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, partitionGroup)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if err := r.reconcilePartitions(partitionGroup); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.reconcileMemberships(partitionGroup); err != nil {
		return reconcile.Result{}, err
	}

	partitionGroupMemberships := &v1beta3.PartitionGroupMembershipList{}
	partitionGroupMembershipFields := map[string]string{
		"bind.group": partitionGroup.Name,
	}
	listOpts := &client.ListOptions{Namespace: partitionGroup.Namespace, FieldSelector: fields.SelectorFromSet(partitionGroupMembershipFields)}
	err = r.client.List(context.TODO(), partitionGroupMemberships, listOpts)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Construct a set of partition group members
	partitionGroupMembers := make(map[string]bool)
	for _, partitionGroupMembership := range partitionGroupMemberships.Items {
		if partitionGroupMembership.DeletionTimestamp == nil {
			partitionGroupMembers[partitionGroupMembership.Bind.Member] = true
		} else {
			logger.Info("Deleting PartitionGroupMembership", "Namespace", partitionGroupMembership.Namespace, "Name", partitionGroupMembership.Name)
		}
	}

	numPartitions := int(partitionGroup.Spec.Partitions)
	partitions := make([]pb.Partition, 0)
	for partition := 1; partition <= numPartitions; partition++ {
		membershipGroup := &v1beta3.MembershipGroup{}
		membershipGroupName := types.NamespacedName{
			Namespace: partitionGroup.Namespace,
			Name:      fmt.Sprintf("%s-%d", partitionGroup.Name, partition),
		}
		err = r.client.Get(context.TODO(), membershipGroupName, membershipGroup)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Get the list of members in the group
		membershipList := &v1beta3.MembershipList{}
		membershipListOpts := &client.ListOptions{
			Namespace: membershipGroup.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"bind.group": membershipGroup.Name,
			}),
		}
		err = r.client.List(context.TODO(), membershipList, membershipListOpts)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Create a set of active members
		members := make(map[string]bool)
		for _, membership := range membershipList.Items {
			if membership.DeletionTimestamp == nil {
				members[membership.Bind.Member] = true
			}
		}

		// Assign a group leader from the set of active members if necessary
		leader := membershipGroup.Status.Leader
		if len(members) > 0 && (leader == "" || !members[leader]) {
			membershipGroup.Status.Leader = membershipList.Items[0].Bind.Member
			membershipGroup.Status.Term = membershipGroup.Status.Term + 1
			err = r.client.Status().Update(context.TODO(), membershipGroup)
			return reconcile.Result{}, err
		} else if leader != "" && len(members) == 0 {
			membershipGroup.Status.Leader = ""
			err = r.client.Status().Update(context.TODO(), membershipGroup)
			return reconcile.Result{}, err
		}

		// Construct a response leader/term
		responseTerm := pb.Term(membershipGroup.Status.Term)
		var responseLeader *pb.ReplicaId
		if leader != "" {
			responseLeader = &pb.ReplicaId{
				Namespace: membershipGroup.Namespace,
				Name:      leader,
			}
		}

		// Construct response membership from the set of members that have not been deleted
		responseReplicas := make([]pb.Replica, 0, len(membershipList.Items))
		for _, membership := range membershipList.Items {
			if membership.DeletionTimestamp != nil {
				continue
			}
			member := &v1beta3.Member{}
			memberName := types.NamespacedName{
				Namespace: membership.Namespace,
				Name:      membership.Bind.Member,
			}
			err := r.client.Get(context.TODO(), memberName, member)
			if err != nil {
				return reconcile.Result{}, nil
			}
			if member.DeletionTimestamp == nil {
				// If the member is not in the set of partition group members, skip the response
				if !partitionGroupMembers[membership.Bind.Member] {
					return reconcile.Result{}, nil
				}
				responseReplicas = append(responseReplicas, pb.Replica{
					ID: pb.ReplicaId{
						Name:      member.Name,
						Namespace: member.Namespace,
					},
					Host: member.Service,
					Port: member.Port.IntVal,
				})
			}
		}

		// Sort the membership to aid in deduplicating responses
		sort.Slice(responseReplicas, func(i, j int) bool {
			return responseReplicas[i].ID.Name < responseReplicas[j].ID.Name
		})

		partitions = append(partitions, pb.Partition{
			ID: pb.PartitionId{
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
	response := pb.JoinReplicaGroupResponse{
		Group: pb.ReplicaGroup{
			ID: pb.ReplicaGroupId{
				Namespace: partitionGroup.Namespace,
				Name:      partitionGroup.Name,
			},
			Partitions: partitions,
		},
	}

	// For each membership, if the membership has been deleted, finalize it before producing the response
	for _, membership := range partitionGroupMemberships.Items {
		if membership.DeletionTimestamp != nil {
			finalize := false
			for _, finalizer := range membership.Finalizers {
				if finalizer == "event" {
					finalize = true
					break
				}
			}
			if finalize {
				finalizers := make([]string, 0, len(membership.Finalizers)-1)
				for _, finalizer := range membership.Finalizers {
					if finalizer != "event" {
						finalizers = append(finalizers, finalizer)
					}
				}
				update := &membership
				update.Finalizers = finalizers
				err = r.client.Update(context.TODO(), update)
				if err != nil {
					return reconcile.Result{}, err
				}
			}
		}
	}

	// Produce the response
	r.responseCh <- response
	return reconcile.Result{}, nil
}

func (r *Reconciler) reconcilePartitions(partitionGroup *v1beta3.PartitionGroup) error {
	partitions := partitionGroup.Spec.Partitions
	if partitions == 0 {
		partitions = 1
	}

	for i := 1; i <= int(partitions); i++ {
		if err := r.reconcilePartition(partitionGroup, i); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcilePartition(partitionGroup *v1beta3.PartitionGroup, partitionID int) error {
	membershipGroup := &v1beta3.MembershipGroup{}
	err := r.client.Get(context.TODO(), getMembershipGroupNamespacedName(partitionGroup, partitionID), membershipGroup)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return r.addPartition(partitionGroup, partitionID)
	}
	return nil
}

func (r *Reconciler) addPartition(partitionGroup *v1beta3.PartitionGroup, partitionID int) error {
	log.Info("Creating MembershipGroup", "Name", getMembershipGroupName(partitionGroup, partitionID), "Namespace", partitionGroup.Namespace)
	owner := metav1.OwnerReference{
		APIVersion: partitionGroup.APIVersion,
		Kind:       partitionGroup.Kind,
		Name:       partitionGroup.ObjectMeta.Name,
		UID:        partitionGroup.ObjectMeta.UID,
	}
	membershipGroup := &v1beta3.MembershipGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:            getMembershipGroupName(partitionGroup, partitionID),
			Namespace:       partitionGroup.Namespace,
			Labels:          partitionGroup.Labels,
			Annotations:     partitionGroup.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
	}
	return r.client.Create(context.TODO(), membershipGroup)
}

func (r *Reconciler) reconcileMemberships(partitionGroup *v1beta3.PartitionGroup) error {
	partitionGroupMemberships := &v1beta3.PartitionGroupMembershipList{}
	partitionGroupMembershipFields := map[string]string{
		"bind.group": partitionGroup.Name,
	}
	listOpts := &client.ListOptions{Namespace: partitionGroup.Namespace, FieldSelector: fields.SelectorFromSet(partitionGroupMembershipFields)}
	err := r.client.List(context.TODO(), partitionGroupMemberships, listOpts)
	if err != nil {
		return err
	}

	if len(partitionGroupMemberships.Items) == 0 {
		return nil
	}

	partitions := int(partitionGroup.Spec.Partitions)
	if partitions == 0 {
		partitions = 1
	}
	replicationFactor := int(partitionGroup.Spec.ReplicationFactor)

	i := 0
	for j := 1; j <= partitions; j++ {
		membershipGroup := &v1beta3.MembershipGroup{}
		membershipGroupName := getMembershipGroupNamespacedName(partitionGroup, j)
		err := r.client.Get(context.TODO(), membershipGroupName, membershipGroup)
		if err != nil {
			return err
		}

		membershipList := &v1beta3.MembershipList{}
		membershipFields := map[string]string{
			"bind.group": getMembershipGroupName(partitionGroup, j),
		}
		listOpts := &client.ListOptions{Namespace: partitionGroup.Namespace, FieldSelector: fields.SelectorFromSet(membershipFields)}
		err = r.client.List(context.TODO(), membershipList, listOpts)
		if err != nil {
			return err
		}

		memberships := make(map[string]bool)
		for _, membership := range membershipList.Items {
			memberships[membership.Bind.Member] = true
		}

		if replicationFactor == 0 || len(memberships) < replicationFactor {
			log.Info("Reconciling partition MembershipGroup", "Namespace", partitionGroup.Namespace, "Group", getMembershipGroupName(partitionGroup, j), "Members", len(partitionGroupMemberships.Items))
			for k := 0; k < len(partitionGroupMemberships.Items); k++ {
				partitionGroupMembership := partitionGroupMemberships.Items[(i+k)%len(partitionGroupMemberships.Items)]
				_, exists := memberships[partitionGroupMembership.Bind.Member]
				if !exists {
					member := &v1beta3.Member{}
					memberName := types.NamespacedName{
						Namespace: partitionGroupMembership.Namespace,
						Name:      partitionGroupMembership.Bind.Member,
					}
					err = r.client.Get(context.TODO(), memberName, member)
					if err != nil {
						if errors.IsNotFound(err) {
							continue
						}
						return err
					}

					log.Info("Creating Membership", "Namespace", partitionGroup.Namespace, "Group", getMembershipGroupName(partitionGroup, j), "Member", partitionGroupMembership.Bind.Member)
					owner := metav1.OwnerReference{
						APIVersion: member.APIVersion,
						Kind:       member.Kind,
						Name:       member.ObjectMeta.Name,
						UID:        member.ObjectMeta.UID,
					}

					membership := &v1beta3.Membership{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:       partitionGroup.Namespace,
							Name:            fmt.Sprintf("%s-%s", partitionGroupMembership.Bind.Member, getMembershipGroupName(partitionGroup, j)),
							OwnerReferences: []metav1.OwnerReference{owner},
							Finalizers:      []string{"event"},
						},
						Bind: v1beta3.MembershipBinding{
							Member: partitionGroupMembership.Bind.Member,
							Group:  getMembershipGroupName(partitionGroup, j),
						},
					}
					err = r.client.Create(context.TODO(), membership)
					if err != nil {
						return err
					}
					i++
				} else {
					log.Info("Membership found", "Namespace", partitionGroup.Namespace, "Group", getMembershipGroupName(partitionGroup, j), "Member", partitionGroupMembership.Bind.Member)
				}
			}
		}
	}
	return nil
}

func getMembershipGroupName(partitionGroup *v1beta3.PartitionGroup, partitionID int) string {
	return fmt.Sprintf("%s-%d", partitionGroup.Name, partitionID)
}

func getMembershipGroupNamespacedName(partitionGroup *v1beta3.PartitionGroup, partitionID int) types.NamespacedName {
	return types.NamespacedName{
		Namespace: partitionGroup.Namespace,
		Name:      getMembershipGroupName(partitionGroup, partitionID),
	}
}

type partitionGroupMembershipMapper struct {
	client client.Client
}

func (m *partitionGroupMembershipMapper) Map(object handler.MapObject) []reconcile.Request {
	partitionGroupMembership := object.Object.(*v1beta3.PartitionGroupMembership)
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: partitionGroupMembership.Namespace,
				Name:      partitionGroupMembership.Bind.Group,
			},
		},
	}
}
