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
)

var log = logf.Log.WithName("membership_group_controller")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, eventCh chan<- types.NamespacedName) error {
	r := &Reconciler{
		client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		config:  mgr.GetConfig(),
		eventCh: eventCh,
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

	// Watch for changes to secondary resource Leadership and requeue the MembershipGroup
	err = c.Watch(&source.Kind{Type: &v1beta3.Leadership{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: &leadershipMapper{
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
	client  client.Client
	scheme  *runtime.Scheme
	config  *rest.Config
	eventCh chan<- types.NamespacedName
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

	defer func() {
		go func() {
			r.eventCh <- types.NamespacedName{
				Namespace: membershipGroup.Namespace,
				Name:      membershipGroup.Name,
			}
		}()
	}()

	leadership := &v1beta3.Leadership{}
	err = r.client.Get(context.TODO(), request.NamespacedName, leadership)
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	} else if err != nil || leadership.DeletionTimestamp != nil {
		membershipList := &v1beta3.MembershipList{}
		membershipFields := map[string]string{
			"bind.group": membershipGroup.Name,
		}
		membershipListOpts := &client.ListOptions{Namespace: membershipGroup.Namespace, FieldSelector: fields.SelectorFromSet(membershipFields)}
		err = r.client.List(context.TODO(), membershipList, membershipListOpts)
		if err != nil {
			return reconcile.Result{}, err
		}

		if len(membershipList.Items) > 0 {
			term := uint64(1)
			if leadership.DeletionTimestamp != nil {
				term = leadership.Term + 1
			}
			membership := membershipList.Items[0]
			owner := metav1.OwnerReference{
				APIVersion: membership.APIVersion,
				Kind:       membership.Kind,
				Name:       membership.ObjectMeta.Name,
				UID:        membership.ObjectMeta.UID,
			}
			leadership := &v1beta3.Leadership{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       request.Namespace,
					Name:            request.Name,
					OwnerReferences: []metav1.OwnerReference{owner},
					Finalizers:      []string{request.Name},
				},
				Group:  membership.Bind.Group,
				Term:   term,
				Leader: membership.Bind.Member,
			}
			err = r.client.Create(context.TODO(), leadership)
			if err != nil {
				return reconcile.Result{}, err
			}
		} else if leadership.DeletionTimestamp != nil {
			leadership.Finalizers = []string{}
			err = r.client.Update(context.TODO(), leadership)
			return reconcile.Result{}, err
		}
	}
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

type leadershipMapper struct {
	client client.Client
}

func (m *leadershipMapper) Map(object handler.MapObject) []reconcile.Request {
	leadership := object.Object.(*v1beta3.Leadership)
	membershipGroup := &v1beta3.MembershipGroup{}
	membershipGroupName := types.NamespacedName{
		Namespace: leadership.Namespace,
		Name:      leadership.Group,
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
