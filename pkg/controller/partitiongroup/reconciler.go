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
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("partition_group_controller")

// Add creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &Reconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
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

	// Watch for changes to secondary resource PartitionGroupMembership and requeue the owner PartitionGroup
	err = c.Watch(&source.Kind{Type: &v1beta3.PartitionGroupMembership{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1beta3.PartitionGroup{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciles a PartitionGroup object
type Reconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
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

	if err := r.reconcileStatus(partitionGroup); err != nil {
		if errors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

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
	membershipGroup := &v1beta3.MembershipGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getMembershipGroupName(partitionGroup, partitionID),
			Namespace:   partitionGroup.Namespace,
			Labels:      partitionGroup.Labels,
			Annotations: partitionGroup.Annotations,
		},
	}
	if err := controllerutil.SetControllerReference(partitionGroup, membershipGroup, r.scheme); err != nil {
		return err
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
			for k := 0; k < len(partitionGroupMemberships.Items); k++ {
				partitionGroupMembership := partitionGroupMemberships.Items[(i+k)%len(partitionGroupMemberships.Items)]
				_, exists := memberships[partitionGroupMembership.Bind.Member]
				if !exists {
					member := &v1beta3.Member{}
					memberName := types.NamespacedName{
						Namespace: partitionGroupMembership.Namespace,
						Name:      partitionGroupMembership.Bind.Member,
					}
					err := r.client.Get(context.TODO(), memberName, member)
					if err != nil {
						return err
					}

					membership := &v1beta3.Membership{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: partitionGroup.Namespace,
							Name:      fmt.Sprintf("%s-%s", partitionGroupMembership.Bind.Member, getMembershipGroupName(partitionGroup, j)),
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
				}
			}
		}
	}
	return nil
}

func (r *Reconciler) reconcileStatus(partitionGroup *v1beta3.PartitionGroup) error {
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
