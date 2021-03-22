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

package v2beta1

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
)

func newPodMapper(mgr manager.Manager) handler.Mapper {
	return &podMapper{
		client: mgr.GetClient(),
	}
}

type podMapper struct {
	client client.Client
}

func (m *podMapper) Map(object handler.MapObject) []reconcile.Request {
	pods := &corev1.PodList{}
	if err := m.client.List(context.TODO(), pods); err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(pods.Items))
	for _, pod := range pods.Items {
		if injectPrimitives, err := strconv.ParseBool(pod.Annotations[injectAnnotation]); err == nil && injectPrimitives {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: pod.Namespace,
					Name:      pod.Name,
				},
			})
		}
	}
	return requests
}

var _ handler.Mapper = &podMapper{}
