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
	"github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newPodLogger(pod corev1.Pod) logging.Logger {
	fields := []logging.Field{
		logging.String("pod", types.NamespacedName{pod.Namespace, pod.Name}.String()),
	}
	return log.WithFields(fields...)
}

func newPrimitiveLogger(pod corev1.Pod, primitive v2beta1.Primitive) logging.Logger {
	fields := []logging.Field{
		logging.String("pod", types.NamespacedName{pod.Namespace, pod.Name}.String()),
		logging.String("primitive", types.NamespacedName{primitive.Namespace, primitive.Name}.String()),
		logging.String("type", primitive.OwnerReferences[0].Kind),
		logging.String("protocol", types.NamespacedName{primitive.Spec.Store.Namespace, primitive.Spec.Store.Name}.String()),
	}
	return log.WithFields(fields...)
}

func newStoreLogger(pod corev1.Pod, store v2beta1.Store) logging.Logger {
	fields := []logging.Field{
		logging.String("pod", types.NamespacedName{pod.Namespace, pod.Name}.String()),
		logging.String("store", types.NamespacedName{store.Namespace, store.Name}.String()),
	}
	return log.WithFields(fields...)
}
