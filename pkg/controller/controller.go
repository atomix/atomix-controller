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
	membershipapi "github.com/atomix/api/proto/atomix/membership"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"github.com/atomix/kubernetes-controller/pkg/controller/database"
	"github.com/atomix/kubernetes-controller/pkg/controller/member"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
)

var log = logf.Log.WithName("controller_atomix")

// AddController adds the Atomix controller to the k8s controller manager
func AddController(mgr manager.Manager) error {
	clusterCh := make(chan membershipapi.JoinClusterResponse)

	c := newController(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), clusterCh)
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

	if err := mgr.GetFieldIndexer().IndexField(&v1beta3.Primitive{}, "properties.type", func(rawObj runtime.Object) []string {
		primitive := rawObj.(*v1beta3.Primitive)
		return []string{string(primitive.Properties.Type)}
	}); err != nil {
		return err
	}

	if err = database.Add(mgr); err != nil {
		return err
	}
	if err = member.Add(mgr, clusterCh); err != nil {
		return err
	}
	return nil
}

// newController creates a new controller server
func newController(client client.Client, scheme *runtime.Scheme, config *rest.Config, clusterCh chan membershipapi.JoinClusterResponse, opts ...grpc.ServerOption) *Controller {
	return &Controller{
		client:              client,
		scheme:              scheme,
		config:              config,
		opts:                opts,
		clusterResponseIn:   clusterCh,
		clusterResponsesOut: make(map[string]map[string]chan<- membershipapi.JoinClusterResponse),
	}
}

// Controller an implementation of the Atomix controller API
type Controller struct {
	client              client.Client
	scheme              *runtime.Scheme
	config              *rest.Config
	opts                []grpc.ServerOption
	clusterResponseIn   chan membershipapi.JoinClusterResponse
	clusterResponsesOut map[string]map[string]chan<- membershipapi.JoinClusterResponse
	mu                  sync.RWMutex
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
		registerMembershipServiceServer(s, c)
		registerPrimitiveServiceServer(s, c)
		registerDatabaseServiceServer(s, c)
		if err := s.Serve(lis); err != nil {
			errs <- err
		}
	}()

	go c.processMembershipResponses(stop)

	select {
	case e := <-errs:
		return e
	case <-stop:
		log.Info("Stopping controller server")
		s.Stop()
		return nil
	}
}

func (c *Controller) processMembershipResponses(stop <-chan struct{}) {
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
