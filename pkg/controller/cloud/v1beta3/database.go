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

package v1beta3

import (
	"context"
	"errors"
	"fmt"
	databaseapi "github.com/atomix/api/proto/atomix/database"
	"github.com/atomix/atomix-controller/pkg/apis/cloud/v1beta3"
	"google.golang.org/grpc"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const clusterDomainEnv = "CLUSTER_DOMAIN"

func registerDatabaseServiceServer(s *grpc.Server, srv databaseapi.DatabaseServiceServer) {
	databaseapi.RegisterDatabaseServiceServer(s, srv)
}

// GetDatabase get a database managed by the controller
func (c *Controller) GetDatabase(ctx context.Context, request *databaseapi.GetDatabaseRequest) (*databaseapi.GetDatabaseResponse, error) {
	database := &v1beta3.Database{}
	name := types.NamespacedName{
		Namespace: GetDatabaseNamespace(request.ID),
		Name:      request.ID.Name,
	}
	err := c.client.Get(ctx, name, database)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.New("database not found")
		}
		return nil, err
	}

	pbdatabase := NewDatabaseProto(database)
	options := &client.ListOptions{
		Namespace:     GetDatabaseNamespace(request.ID),
		LabelSelector: labels.SelectorFromSet(GetPartitionLabelsForDatabase(database)),
	}
	partitions := &v1beta3.PartitionList{}
	err = c.client.List(context.TODO(), partitions, options)
	if err != nil {
		return nil, err
	}

	if len(partitions.Items) != int(database.Spec.Partitions) {
		return nil, errors.New("database not found")
	}

	pbpartitions := make([]databaseapi.Partition, 0, len(partitions.Items))
	for _, partition := range partitions.Items {
		pbpartitions = append(pbpartitions, *NewPartitionProto(&partition))
	}
	pbdatabase.Partitions = pbpartitions
	return &databaseapi.GetDatabaseResponse{
		Database: pbdatabase,
	}, nil
}

// GetDatabases get a list of databases managed by the controller
func (c *Controller) GetDatabases(ctx context.Context, request *databaseapi.GetDatabasesRequest) (*databaseapi.GetDatabasesResponse, error) {
	databases := &v1beta3.DatabaseList{}

	opts := &client.ListOptions{
		Namespace: GetDatabaseNamespace(databaseapi.DatabaseId{Namespace: request.Namespace}),
	}

	if err := c.client.List(ctx, databases, opts); err != nil {
		return nil, err
	}

	pbdatabases := make([]databaseapi.Database, 0, len(databases.Items))
	for _, database := range databases.Items {
		pbdatabase := NewDatabaseProto(&database)

		options := &client.ListOptions{
			Namespace:     GetDatabaseNamespace(databaseapi.DatabaseId{Namespace: request.Namespace}),
			LabelSelector: labels.SelectorFromSet(GetPartitionLabelsForDatabase(&database)),
		}
		partitions := &v1beta3.PartitionList{}
		err := c.client.List(context.TODO(), partitions, options)
		if err != nil {
			return nil, err
		}

		if len(partitions.Items) != int(database.Spec.Partitions) {
			continue
		}

		pbpartitions := make([]databaseapi.Partition, 0, len(partitions.Items))
		for _, partition := range partitions.Items {
			pbpartitions = append(pbpartitions, *NewPartitionProto(&partition))
		}
		pbdatabase.Partitions = pbpartitions
		pbdatabases = append(pbdatabases, *pbdatabase)
	}

	return &databaseapi.GetDatabasesResponse{
		Databases: pbdatabases,
	}, nil
}

// NewDatabaseController creates a new Database controller and adds it to the Manager. The Manager will set fields on the
// controller and Start it when the Manager is Started.
func NewDatabaseController(mgr manager.Manager) (controller.Controller, error) {
	r := &DatabaseReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	// Create a new controller
	c, err := controller.New("database-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	// Watch for changes to primary resource Database
	err = c.Watch(&source.Kind{Type: &v1beta3.Database{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return nil, err
	}

	// Watch for changes to secondary resource Partition and requeue the owner Database
	err = c.Watch(&source.Kind{Type: &v1beta3.Partition{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &v1beta3.Database{},
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

var _ reconcile.Reconciler = &DatabaseReconciler{}

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
}

// Reconcile reads that state of the cluster for a Database object and makes changes based on the state read
// and what is in the Database.Spec
func (r *DatabaseReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Database")

	// Fetch the Database instance
	database := &v1beta3.Database{}
	err := r.client.Get(context.TODO(), request.NamespacedName, database)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	v1beta3.SetDatabaseDefaults(database)

	for i := 1; i <= int(database.Spec.Partitions); i++ {
		if err = r.reconcilePartition(database, i); err != nil {
			return reconcile.Result{}, err
		}
	}

	if err = r.reconcileStatus(database); err != nil {
		if k8serrors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *DatabaseReconciler) reconcilePartition(database *v1beta3.Database, partitionID int) error {
	partition := &v1beta3.Partition{}
	err := r.client.Get(context.TODO(), GetPartitionNamespacedName(database, partitionID), partition)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return r.addPartition(database, partitionID)
	}
	return nil
}

func (r *DatabaseReconciler) addPartition(database *v1beta3.Database, partitionID int) error {
	log.Info("Creating Partition", "Name", GetPartitionName(database, partitionID), "Namespace", database.Namespace)
	partition := NewPartition(database, partitionID)
	if err := controllerutil.SetControllerReference(database, partition, r.scheme); err != nil {
		return err
	}
	return r.client.Create(context.TODO(), partition)
}

func (r *DatabaseReconciler) reconcileStatus(database *v1beta3.Database) error {
	readyPartitions := 0
	for i := 1; i <= int(database.Spec.Partitions); i++ {
		partition := &v1beta3.Partition{}
		err := r.client.Get(context.TODO(), GetPartitionNamespacedName(database, i), partition)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if partition.Status.Ready {
			readyPartitions++
		}
	}

	if int(database.Status.ReadyPartitions) != readyPartitions {
		log.Info("Updating Database status", "Name", database.Name, "Namespace", database.Namespace, "ReadyPartitions", readyPartitions)
		database.Status.ReadyPartitions = int32(readyPartitions)
		return r.client.Status().Update(context.TODO(), database)
	}
	return nil
}

// GetDatabaseNamespace returns the Database namespace for the given database ID
func GetDatabaseNamespace(id databaseapi.DatabaseId) string {
	if id.Namespace != "" {
		return id.Namespace
	}
	return defaultNamespace
}

// NewPartitionProto returns the partition proto message for the given Partition
func NewPartitionProto(p *v1beta3.Partition) *databaseapi.Partition {
	domain := os.Getenv(clusterDomainEnv)
	if domain == "" {
		domain = "cluster.local"
	}
	host := fmt.Sprintf("%s.%s.svc.%s", p.Spec.ServiceName, p.Namespace, domain)
	return &databaseapi.Partition{
		PartitionID: databaseapi.PartitionId{
			Partition: p.Spec.PartitionID,
		},
		Endpoints: []databaseapi.PartitionEndpoint{
			{
				Host: host,
				Port: partitionPort,
			},
		},
	}
}

// NewDatabaseProto returns a Database proto message for the given Database
func NewDatabaseProto(database *v1beta3.Database) *databaseapi.Database {
	return &databaseapi.Database{
		ID: databaseapi.DatabaseId{
			Name:      database.Name,
			Namespace: database.Namespace,
		},
	}
}
