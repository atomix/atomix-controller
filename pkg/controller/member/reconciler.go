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

package member

import (
	"context"
	"github.com/atomix/api/proto/atomix/cluster"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
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

var log = logf.Log.WithName("member_controller")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, responseCh chan<- cluster.JoinClusterResponse) error {
	r := &Reconciler{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		config:     mgr.GetConfig(),
		responseCh: responseCh,
	}

	// Create a new controller
	c, err := controller.New("member-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Member
	err = c.Watch(&source.Kind{Type: &v1beta3.Member{}}, &handler.EnqueueRequestForObject{})
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
	responseCh chan<- cluster.JoinClusterResponse
}

func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Member")

	// Fetch the Member instance
	member := &v1beta3.Member{}
	err := r.client.Get(context.TODO(), request.NamespacedName, member)
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

	if member.DeletionTimestamp != nil {
		logger.Info("Deleting Member", "Namespace", member.Namespace, "Name", member.Name)
	}

	// Get the set of members in the member's scope
	memberList := &v1beta3.MemberList{}
	memberListFields := map[string]string{
		"properties.namespace": member.Properties.Namespace,
	}
	memberListOpts := &client.ListOptions{
		Namespace:     member.Namespace,
		FieldSelector: fields.SelectorFromSet(memberListFields),
	}
	err = r.client.List(context.TODO(), memberList, memberListOpts)
	if err != nil {
		return reconcile.Result{}, err
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
	response := cluster.JoinClusterResponse{
		ClusterID: cluster.ClusterId{
			Namespace: member.Namespace,
			Name:      member.Properties.Namespace,
		},
		Members: members,
	}

	// If the updated member has been deleted, finalize it before producing the response
	if member.DeletionTimestamp != nil {
		finalize := false
		for _, finalizer := range member.Finalizers {
			if finalizer == "event" {
				finalize = true
				break
			}
		}
		if finalize {
			finalizers := make([]string, 0, len(member.Finalizers)-1)
			for _, finalizer := range member.Finalizers {
				if finalizer != "event" {
					finalizers = append(finalizers, finalizer)
				}
			}
			member.Finalizers = finalizers
			err = r.client.Update(context.TODO(), member)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// Produce the response
	r.responseCh <- response
	return reconcile.Result{}, nil
}
