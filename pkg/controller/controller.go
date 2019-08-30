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
	"encoding/json"
	"fmt"
	api "github.com/atomix/atomix-api/proto/atomix/controller"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/partition"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/partitionset"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/protocol"
	k8sutil "github.com/atomix/atomix-k8s-controller/pkg/controller/util/k8s"
	"google.golang.org/grpc"
	"io"
	"k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller_atomix")

// AddController adds the Atomix controller to the k8s controller manager
func AddController(mgr manager.Manager) error {
	protocols := protocol.NewManager()

	c := newController(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), protocols)
	err := mgr.Add(c)
	if err != nil {
		return err
	}

	if err = partition.Add(mgr, protocols); err != nil {
		return err
	}
	if err = partitionset.Add(mgr, protocols); err != nil {
		return err
	}
	return nil
}

// newController creates a new controller server
func newController(client client.Client, scheme *runtime.Scheme, config *rest.Config, protocols *protocol.Manager, opts ...grpc.ServerOption) *Controller {
	return &Controller{
		client:    client,
		scheme:    scheme,
		config:    config,
		opts:      opts,
		protocols: protocols,
		elections: make(map[electionID]*election),
	}
}

// Controller an implementation of the Atomix controller API
type Controller struct {
	api.ControllerServiceServer

	client    client.Client
	scheme    *runtime.Scheme
	config    *rest.Config
	opts      []grpc.ServerOption
	protocols *protocol.Manager
	elections map[electionID]*election
}

// CreatePartitionGroup creates a partition group via the k8s API
func (c *Controller) CreatePartitionGroup(ctx context.Context, r *api.CreatePartitionGroupRequest) (*api.CreatePartitionGroupResponse, error) {
	group := &v1alpha1.PartitionSet{}
	name := k8sutil.GetPartitionSetNamespacedName(r.ID)

	err := c.client.Get(ctx, name, group)
	if err != nil && k8serrors.IsNotFound(err) {
		group, err = k8sutil.NewPartitionSetFromProto(r.ID, r.Spec, c.protocols)
		if err != nil {
			return nil, err
		}
		if err = c.client.Create(context.TODO(), group); err != nil {
			return nil, err
		}
	}
	return &api.CreatePartitionGroupResponse{}, nil
}

// DeletePartitionGroup deletes a partition group via the k8s API
func (c *Controller) DeletePartitionGroup(ctx context.Context, r *api.DeletePartitionGroupRequest) (*api.DeletePartitionGroupResponse, error) {
	group := &v1alpha1.PartitionSet{}
	name := k8sutil.GetPartitionSetNamespacedName(r.ID)

	if err := c.client.Get(ctx, name, group); err != nil {
		return nil, err
	}

	if err := c.client.Delete(ctx, group); err != nil {
		return nil, err
	}
	return &api.DeletePartitionGroupResponse{}, nil
}

// GetPartitionGroups returns a list of partition groups read from the k8s API
func (c *Controller) GetPartitionGroups(ctx context.Context, request *api.GetPartitionGroupsRequest) (*api.GetPartitionGroupsResponse, error) {
	if request.ID.Name != "" {
		return c.getPartitionGroup(ctx, request)
	}
	return c.getPartitionGroups(ctx, request)
}

// getPartitionGroups gets all partition groups for the given request
func (c *Controller) getPartitionGroups(ctx context.Context, request *api.GetPartitionGroupsRequest) (*api.GetPartitionGroupsResponse, error) {
	groups := &v1alpha1.PartitionSetList{}

	opts := &client.ListOptions{
		Namespace: k8sutil.GetPartitionSetNamespace(request.ID),
	}

	if err := c.client.List(ctx, opts, groups); err != nil {
		return nil, err
	}

	pbgroups := make([]*api.PartitionGroup, 0, len(groups.Items))
	for _, group := range groups.Items {
		pbgroup, err := k8sutil.NewPartitionGroupProtoFromSet(&group, c.protocols)
		if err != nil {
			return nil, err
		}

		options := &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(k8sutil.GetPartitionLabelsForPartitionSet(&group)),
		}
		partitions := &v1alpha1.PartitionList{}
		err = c.client.List(context.TODO(), options, partitions)
		if err != nil {
			return nil, err
		}

		pbpartitions := make([]*api.Partition, 0, len(partitions.Items))
		for _, partition := range partitions.Items {
			pbpartition, err := k8sutil.NewPartitionProto(&partition)
			if err != nil {
				return nil, err
			}
			pbpartitions = append(pbpartitions, pbpartition)
		}
		pbgroup.Partitions = pbpartitions

		pbgroups = append(pbgroups, pbgroup)
	}

	return &api.GetPartitionGroupsResponse{
		Groups: pbgroups,
	}, nil
}

// getPartitionGroup gets a single partition group for the given request
func (c *Controller) getPartitionGroup(ctx context.Context, request *api.GetPartitionGroupsRequest) (*api.GetPartitionGroupsResponse, error) {
	group := &v1alpha1.PartitionSet{}
	name := k8sutil.GetPartitionSetNamespacedName(request.ID)
	err := c.client.Get(context.TODO(), name, group)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return &api.GetPartitionGroupsResponse{
				Groups: []*api.PartitionGroup{},
			}, nil
		}
		return nil, err
	}

	proto, err := k8sutil.NewPartitionGroupProtoFromSet(group, c.protocols)
	if err != nil {
		return nil, err
	}

	options := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(k8sutil.GetPartitionLabelsForPartitionSet(group)),
	}
	partitions := &v1alpha1.PartitionList{}
	err = c.client.List(context.TODO(), options, partitions)
	if err != nil {
		return nil, err
	}

	partitionProtos := make([]*api.Partition, 0, len(partitions.Items))
	for _, partition := range partitions.Items {
		partitionProto, err := k8sutil.NewPartitionProto(&partition)
		if err != nil {
			return nil, err
		}
		partitionProtos = append(partitionProtos, partitionProto)
	}
	proto.Partitions = partitionProtos

	return &api.GetPartitionGroupsResponse{
		Groups: []*api.PartitionGroup{proto},
	}, nil
}

// EnterElection enters a partition node into a leader election
func (c *Controller) EnterElection(r *api.PartitionElectionRequest, s api.ControllerService_EnterElectionServer) error {
	id := electionID{
		namespace: r.PartitionID.Group.Namespace,
		name:      r.PartitionID.Group.Name,
		partition: int(r.PartitionID.Partition),
	}

	election, ok := c.elections[id]
	if !ok {
		election = newElection(id, c)
		c.elections[id] = election
	}

	ch := make(chan term)
	err := election.enter(r.Member, ch)
	if err != nil {
		return err
	}

	for {
		term := <-ch
		response := &api.PartitionElectionResponse{
			Term: &api.PrimaryTerm{
				Term:       term.term,
				Primary:    term.primary,
				Candidates: term.candidates,
			},
		}
		if err := s.Send(response); err != nil {
			if err == io.EOF {
				return election.leave(r.Member)
			}
			return err
		}
	}
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

// electionID is an identifier for the election for a single partition
type electionID struct {
	namespace string
	name      string
	partition int
}

func (e electionID) String() string {
	return fmt.Sprintf("%s-%s-%d", e.namespace, e.name, e.partition)
}

// term provides primary and term information for a partition primary election
type term struct {
	primary    string
	term       int64
	candidates []string
}

// newElection returns a new primary election controller for a single partition
func newElection(id electionID, controller *Controller) *election {
	return &election{
		id:         id,
		controller: controller,
	}
}

// election manages the primary election for a single partition
type election struct {
	id         electionID
	controller *Controller
	candidates map[string]chan term
}

// electionState stores the state of a single primary election
type electionState struct {
	term       int64
	candidates []string
}

// enter adds a candidate to the election and if necessary updates the term
func (e *election) enter(candidate string, ch chan term) error {
	e.candidates[candidate] = ch

	// Initialize the ConfigMap and create a namespaced name
	cm := &v1.ConfigMap{}
	name := types.NamespacedName{
		Namespace: k8sutil.GetControllerNamespace(),
		Name:      k8sutil.GetControllerName() + "-elections",
	}

	// Ensure the elections ConfigMap has been created in k8s
	err := e.controller.client.Get(context.TODO(), name, cm)
	if err != nil && k8serrors.IsNotFound(err) {
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: name.Namespace,
				Name:      name.Name,
			},
			BinaryData: make(map[string][]byte),
		}
		if err = e.controller.client.Create(context.TODO(), cm); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Ensure the elections ConfigMap has been initialized with this election
	bytes, ok := cm.BinaryData[e.id.String()]
	if !ok {
		bytes, err = json.Marshal(electionState{
			term:       0,
			candidates: []string{},
		})
		if err != nil {
			return err
		}
		cm.BinaryData[e.id.String()] = bytes
	}

	// Parse the existing state of this election from the ConfigMap
	election := &electionState{}
	if err = json.Unmarshal(bytes, election); err != nil {
		return err
	}

	// Append the candidate to the candidates list and produce a term change.
	// If the candidate is the first to be added, increment the term and
	// produce an event with the candidate as the primary. Otherwise,
	// simply enter the candidate to the list and update the ConfigMap.
	size := len(election.candidates)
	election.candidates = append(election.candidates)
	if size == 0 {
		election.term = election.term + 1
	}

	// Update the ConfigMap to store the election results
	if err = e.controller.client.Update(context.TODO(), cm); err != nil {
		return err
	}

	// Produce the term change event
	e.changeTerm(term{
		term:       election.term,
		primary:    election.candidates[0],
		candidates: election.candidates,
	})
	return nil
}

// leave removes a candidate from the election and if necessary updates the term
func (e *election) leave(candidate string) error {
	delete(e.candidates, candidate)

	// Initialize the ConfigMap and create a namespaced name
	cm := &v1.ConfigMap{}
	name := types.NamespacedName{
		Namespace: k8sutil.GetControllerNamespace(),
		Name:      k8sutil.GetControllerName() + "-elections",
	}

	// Read the elections ConfigMap and return if it does not exist
	err := e.controller.client.Get(context.TODO(), name, cm)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	// Get the election state from the elections ConfigMap and return if it doesn't exist
	bytes, ok := cm.BinaryData[e.id.String()]
	if !ok {
		return nil
	}

	// Parse the existing state of this election from the ConfigMap
	election := &electionState{}
	if err = json.Unmarshal(bytes, election); err != nil {
		return err
	}

	// Create a slice of candidates with the candidate removed
	candidates := []string{}
	for _, c := range election.candidates {
		if c != candidate {
			candidates = append(candidates, c)
		}
	}

	// If the list of candidates has not changed, return
	if len(candidates) == len(election.candidates) {
		return nil
	}

	// If the first element in the candidates list changed, bump the term
	if len(candidates) > 0 && candidates[0] != election.candidates[0] {
		election.term = election.term + 1
	}
	election.candidates = candidates

	// Update the ConfigMap to store the election results
	if err = e.controller.client.Update(context.TODO(), cm); err != nil {
		return err
	}

	// Produce the term change event
	e.changeTerm(term{
		term:       election.term,
		primary:    election.candidates[0],
		candidates: election.candidates,
	})
	return nil
}

func (e *election) changeTerm(t term) {
	for _, candidate := range e.candidates {
		candidate <- t
	}
}
