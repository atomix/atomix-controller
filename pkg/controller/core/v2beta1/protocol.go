// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

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
			for _, driver := range plugin.Spec.Drivers {
				gvc := schema.GroupVersionKind{
					Group:   plugin.Spec.Protocol.Group,
					Kind:    plugin.Spec.Protocol.Kind,
					Version: driver.Version,
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
			for _, version := range plugin.Spec.DeprecatedVersions {
				gvc := schema.GroupVersionKind{
					Group:   plugin.Spec.DeprecatedGroup,
					Kind:    plugin.Spec.DeprecatedKind,
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
