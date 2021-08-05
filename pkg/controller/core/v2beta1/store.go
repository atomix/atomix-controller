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
	corev2beta1 "github.com/atomix/atomix-controller/pkg/apis/core/v2beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func addStoreController(mgr manager.Manager) error {
	r := &StoreReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("store-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Stores
	err = c.Watch(&source.Kind{Type: &corev2beta1.Store{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to protocols
	err = c.Watch(&protocolSource{}, &handler.EnqueueRequestForOwner{
		OwnerType:    &corev2beta1.Store{},
		IsController: true,
	})
	if err != nil {
		return err
	}
	return nil
}

// StoreReconciler is a Reconciler for Stores
type StoreReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reconciles Store resources
func (r *StoreReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Infof("Reconciling Store '%s'", request.NamespacedName)
	store := &corev2beta1.Store{}
	err := r.client.Get(context.TODO(), request.NamespacedName, store)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err)
		return reconcile.Result{}, err
	}
	return r.reconcileStore(store)
}

func (r *StoreReconciler) reconcileStore(store *corev2beta1.Store) (reconcile.Result, error) {
	if ok, err := r.reconcileProtocol(store); err != nil {
		return reconcile.Result{}, err
	} else if ok {
		return reconcile.Result{Requeue: true}, nil
	}
	return reconcile.Result{}, nil
}

func (r *StoreReconciler) reconcileProtocol(store *corev2beta1.Store) (bool, error) {
	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, store.Spec.Protocol.Raw)
	if err != nil {
		log.Error(err)
		return false, err
	}

	protocol := object.(*unstructured.Unstructured)
	stored := &unstructured.Unstructured{}
	stored.SetGroupVersionKind(protocol.GroupVersionKind())
	name := types.NamespacedName{
		Namespace: store.Namespace,
		Name:      store.Name,
	}
	err = r.client.Get(context.TODO(), name, stored)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			log.Error(err)
			return false, err
		}

		log.Infof("Creating protocol for Store %s", types.NamespacedName{Namespace: store.Namespace, Name: store.Name})
		protocol.SetNamespace(store.Namespace)
		protocol.SetName(store.Name)

		if err := controllerutil.SetControllerReference(store, protocol, r.scheme); err != nil {
			log.Error(err)
			return false, err
		}

		err = r.client.Create(context.TODO(), protocol)
		if err == nil {
			return true, nil
		} else if !k8serrors.IsAlreadyExists(err) {
			log.Error(err)
			return false, err
		}
	}
	return r.reconcileProtocolStatus(store, stored)
}

// nolint:gocyclo
func (r *StoreReconciler) reconcileProtocolStatus(store *corev2beta1.Store, protocol *unstructured.Unstructured) (bool, error) {
	revision, ok, err := unstructured.NestedInt64(protocol.UnstructuredContent(), "status", "revision")
	if err != nil {
		log.Error(err)
		return false, err
	} else if !ok {
		return false, nil
	}

	if revision > 0 && store.Status.Protocol.Revision <= revision {
		protocolStatus := corev2beta1.ProtocolStatus{
			Revision: revision,
		}

		var readyReplicas int32
		replicas, ok, err := unstructured.NestedSlice(protocol.UnstructuredContent(), "status", "replicas")
		if err != nil {
			return false, err
		} else if ok {
			for _, r := range replicas {
				replicaObj := r.(map[string]interface{})
				replica := corev2beta1.ReplicaStatus{}
				id, ok, err := unstructured.NestedString(replicaObj, "id")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					replica.ID = id
				}

				nodeID, ok, err := unstructured.NestedString(replicaObj, "nodeId")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					replica.NodeID = nodeID
				}

				host, ok, err := unstructured.NestedString(replicaObj, "host")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					replica.Host = pointer.StringPtr(host)
				}

				port, ok, err := unstructured.NestedInt64(replicaObj, "port")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					replica.Port = pointer.Int32Ptr(int32(port))
				}

				extraPorts, ok, err := unstructured.NestedMap(replicaObj, "extraPorts")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					replica.ExtraPorts = make(map[string]int32)
					for name := range extraPorts {
						extraPort, ok, err := unstructured.NestedInt64(extraPorts, name)
						if err != nil {
							log.Error(err)
							return false, err
						} else if ok {
							replica.ExtraPorts[name] = int32(extraPort)
						}
					}
				}

				ready, _, err := unstructured.NestedBool(replicaObj, "ready")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ready {
					replica.Ready = ready
					readyReplicas++
				}

				protocolStatus.Replicas = append(protocolStatus.Replicas, replica)
			}
		}

		var readyPartitions int32
		partitions, ok, err := unstructured.NestedSlice(protocol.UnstructuredContent(), "status", "partitions")
		if err != nil {
			return false, err
		} else if ok {
			for _, p := range partitions {
				partitionObj := p.(map[string]interface{})
				partition := corev2beta1.PartitionStatus{}
				id, ok, err := unstructured.NestedInt64(partitionObj, "id")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					partition.ID = uint32(id)
				}

				host, ok, err := unstructured.NestedString(partitionObj, "host")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					partition.Host = pointer.StringPtr(host)
				}

				port, ok, err := unstructured.NestedInt64(partitionObj, "port")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					partition.Port = pointer.Int32Ptr(int32(port))
				}

				replicas, ok, err := unstructured.NestedSlice(partitionObj, "replicas")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					for _, replica := range replicas {
						partition.Replicas = append(partition.Replicas, replica.(string))
					}
				}

				readReplicas, ok, err := unstructured.NestedSlice(partitionObj, "readReplicas")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ok {
					for _, readReplica := range readReplicas {
						partition.ReadReplicas = append(partition.ReadReplicas, readReplica.(string))
					}
				}

				ready, _, err := unstructured.NestedBool(partitionObj, "ready")
				if err != nil {
					log.Error(err)
					return false, err
				} else if ready {
					partition.Ready = ready
					readyPartitions++
				}
				protocolStatus.Partitions = append(protocolStatus.Partitions, partition)
			}
		}

		if revision == store.Status.Protocol.Revision && readyReplicas == store.Status.ReadyReplicas && readyPartitions == store.Status.ReadyPartitions {
			return false, nil
		}

		log.Infof("Protocol status changed for Store %s", types.NamespacedName{Namespace: store.Namespace, Name: store.Name})
		store.Status.Protocol = protocolStatus
		store.Status.Replicas = int32(len(store.Status.Protocol.Replicas))
		store.Status.ReadyReplicas = readyReplicas
		store.Status.Partitions = int32(len(store.Status.Protocol.Partitions))
		store.Status.ReadyPartitions = readyPartitions
		store.Status.Ready = store.Status.ReadyReplicas > 0 && store.Status.ReadyReplicas == store.Status.Replicas &&
			store.Status.ReadyPartitions > 0 && store.Status.ReadyPartitions == store.Status.Partitions

		err = r.client.Status().Update(context.TODO(), store)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				log.Error(err)
				return false, err
			}
			return false, nil
		}
		return true, nil
	}
	return false, nil
}
