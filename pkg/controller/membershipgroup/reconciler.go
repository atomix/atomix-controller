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

package membershipgroup

import (
	"context"
	api "github.com/atomix/api/proto/atomix/controller"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"k8s.io/apimachinery/pkg/api/errors"
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

var log = logf.Log.WithName("membership_group_controller")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, eventCh chan<- api.JoinMembershipGroupResponse) error {
	r := &Reconciler{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		config:     mgr.GetConfig(),
		responseCh: eventCh,
	}

	// Create a new controller
	c, err := controller.New("membership-group-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MembershipGroup
	err = c.Watch(&source.Kind{Type: &v1beta3.MembershipGroup{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Membership and requeue the MembershipGroup
	err = c.Watch(&source.Kind{Type: &v1beta3.Membership{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &membershipMapper{
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
	responseCh chan<- api.JoinMembershipGroupResponse
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling MembershipGroup")

	// Fetch the MembershipGroup instance
	membershipGroup := &v1beta3.MembershipGroup{}
	err := r.client.Get(context.TODO(), request.NamespacedName, membershipGroup)
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
		} else {
			logger.Info("Deleting Membership", "Namespace", membership.Namespace, "Name", membership.Name)
		}
	}

	// Assign a group leader from the set of active members if necessary
	leader := membershipGroup.Status.Leader
	if len(members) > 0 && (leader == "" || !members[leader]) {
		for member := range members {
			leader = member
			break
		}
		membershipGroup.Status.Leader = leader
		membershipGroup.Status.Term = membershipGroup.Status.Term + 1
		err = r.client.Status().Update(context.TODO(), membershipGroup)
		return reconcile.Result{}, err
	} else if leader != "" && len(members) == 0 {
		membershipGroup.Status.Leader = ""
		err = r.client.Status().Update(context.TODO(), membershipGroup)
		return reconcile.Result{}, err
	}

	// Construct a response leader/term
	responseTerm := api.TermID(membershipGroup.Status.Term)
	var responseLeader *api.MemberId
	if leader != "" {
		responseLeader = &api.MemberId{
			Namespace: membershipGroup.Namespace,
			Name:      leader,
		}
	}

	// Construct response membership from the set of members that have not been deleted
	responseMembers := make([]api.Member, 0, len(membershipList.Items))
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
			responseMembers = append(responseMembers, api.Member{
				ID: api.MemberId{
					Name:      member.Name,
					Namespace: member.Namespace,
				},
				Host: member.Service,
				Port: member.Port.IntVal,
			})
		}
	}

	// Sort the membership to aid in deduplicating responses
	sort.Slice(responseMembers, func(i, j int) bool {
		return responseMembers[i].ID.Name < responseMembers[j].ID.Name
	})

	// Construct a membership response
	response := api.JoinMembershipGroupResponse{
		Group: api.MembershipGroup{
			ID: api.MembershipGroupId{
				Namespace: membershipGroup.Namespace,
				Name:      membershipGroup.Name,
			},
			Term:    responseTerm,
			Leader:  responseLeader,
			Members: responseMembers,
		},
	}

	// For each membership, if the membership has been deleted, finalize it before producing the response
	for _, membership := range membershipList.Items {
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

type membershipMapper struct {
	client client.Client
}

func (m *membershipMapper) Map(object handler.MapObject) []reconcile.Request {
	membership := object.Object.(*v1beta3.Membership)
	membershipGroup := &v1beta3.MembershipGroup{}
	membershipGroupName := types.NamespacedName{
		Namespace: membership.Namespace,
		Name:      membership.Bind.Group,
	}
	err := m.client.Get(context.TODO(), membershipGroupName, membershipGroup)
	if err != nil {
		return []reconcile.Request{}
	}
	return []reconcile.Request{
		{
			NamespacedName: membershipGroupName,
		},
	}
}
