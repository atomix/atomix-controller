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
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type protocolSource struct {
	cache cache.Cache
}

func (p *protocolSource) InjectCache(cache cache.Cache) error {
	p.cache = cache
	return nil
}

func (p *protocolSource) Start(eventHandler handler.EventHandler, limitingInterface workqueue.RateLimitingInterface, predicate ...predicate.Predicate) error {
	i, err := p.cache.GetInformer(&corev2beta1.StoragePlugin{})
	if err != nil {
		log.Error(err)
		return err
	}
	i.AddEventHandler(p.newPluginHandler(eventHandler, limitingInterface, predicate...))
	return nil
}

func (p *protocolSource) newPluginHandler(eventHandler handler.EventHandler, limitingInterface workqueue.RateLimitingInterface, predicate ...predicate.Predicate) clientcache.ResourceEventHandler {
	funcs := clientcache.ResourceEventHandlerFuncs{}
	funcs.AddFunc = func(obj interface{}) {
		if o, ok := obj.(runtime.Object); ok {
			plugin := o.(*corev2beta1.StoragePlugin)
			for _, version := range plugin.Spec.Versions {
				gvc := schema.GroupVersionKind{
					Group:   plugin.Spec.Group,
					Kind:    plugin.Spec.Kind,
					Version: version.Name,
				}
				log.Infof("Starting Source %s", gvc)
				object := &unstructured.Unstructured{}
				object.SetGroupVersionKind(gvc)
				kind := &source.Kind{
					Type: object,
				}
				if err := kind.InjectCache(p.cache); err != nil {
					log.Error(err)
				} else {
					if err := kind.Start(eventHandler, limitingInterface, predicate...); err != nil {
						log.Error(err)
					}
				}
			}
		} else {
			log.Error("Missing runtime.Object", obj)
		}
	}
	return funcs
}
