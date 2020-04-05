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

package protocol

import (
	"context"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetProtocol gets the protocol matching the given kind
func GetProtocol(c client.Client, kind schema.GroupVersionKind) (*v1beta1.Protocol, error) {
	protocols := &v1beta1.ProtocolList{}
	err := c.List(context.TODO(), &client.ListOptions{}, protocols)
	if err != nil {
		return nil, err
	}
	for _, protocol := range protocols.Items {
		if kind.Group == protocol.Spec.Group && kind.Version == protocol.Spec.Version && kind.Kind == protocol.Spec.Kind {
			return &protocol, nil
		}
	}
	return nil, errors.NewNotFound(schema.GroupResource{
		Group:    kind.Group,
		Resource: kind.Kind,
	}, kind.Kind)
}
