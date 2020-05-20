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
	databaseapi "github.com/atomix/api/proto/atomix/database"
	primitiveapi "github.com/atomix/api/proto/atomix/primitive"
	"github.com/atomix/kubernetes-controller/pkg/apis/cloud/v1beta3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func registerPrimitiveServiceServer(s *grpc.Server, srv primitiveapi.PrimitiveServiceServer) {
	primitiveapi.RegisterPrimitiveServiceServer(s, srv)
}

func getPrimitiveNamespacedName(database databaseapi.DatabaseId, primitive primitiveapi.PrimitiveId) types.NamespacedName {
	return types.NamespacedName{
		Namespace: database.Namespace,
		Name:      primitive.Name,
	}
}

func (c *Controller) CreatePrimitive(ctx context.Context, request *primitiveapi.CreatePrimitiveRequest) (*primitiveapi.CreatePrimitiveResponse, error) {
	database := &v1beta3.Database{}
	databaseName := types.NamespacedName{
		Namespace: request.Database.Namespace,
		Name:      request.Database.Name,
	}
	err := c.client.Get(ctx, databaseName, database)
	if err != nil {
		return nil, err
	}

	primitive := &v1beta3.Primitive{}
	primitiveName := getPrimitiveNamespacedName(request.Database, request.Primitive)
	err = c.client.Get(ctx, primitiveName, primitive)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}

		primitiveOwner := metav1.OwnerReference{
			APIVersion: database.APIVersion,
			Kind:       database.Kind,
			Name:       database.ObjectMeta.Name,
			UID:        database.ObjectMeta.UID,
		}

		primitive = &v1beta3.Primitive{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       request.Database.Namespace,
				Name:            request.Primitive.Name,
				OwnerReferences: []metav1.OwnerReference{primitiveOwner},
			},
			Properties: v1beta3.PrimitiveProperties{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: request.Primitive.Namespace,
					Name:      request.Primitive.Name,
				},
				Type:     v1beta3.GetPrimitiveType(request.Type),
				Database: database.Name,
			},
		}
		err = c.client.Create(ctx, primitive)
		if err != nil {
			return nil, err
		}
	}
	return &primitiveapi.CreatePrimitiveResponse{
		Primitive: primitiveapi.PrimitiveMetadata{
			Database: databaseapi.DatabaseId{
				Namespace: primitive.Namespace,
				Name:      primitive.Properties.Database,
			},
			Primitive: primitiveapi.PrimitiveId{
				Namespace: primitive.Properties.Namespace,
				Name:      primitive.Properties.Name,
			},
			Type: primitive.Properties.Type.Proto(),
		},
	}, nil
}

func (c *Controller) GetPrimitive(ctx context.Context, request *primitiveapi.GetPrimitiveRequest) (*primitiveapi.GetPrimitiveResponse, error) {
	database := &v1beta3.Database{}
	databaseName := types.NamespacedName{
		Namespace: request.Database.Namespace,
		Name:      request.Database.Name,
	}
	err := c.client.Get(ctx, databaseName, database)
	if err != nil {
		return nil, err
	}

	primitive := &v1beta3.Primitive{}
	primitiveName := getPrimitiveNamespacedName(request.Database, request.Primitive)
	err = c.client.Get(ctx, primitiveName, primitive)
	if err != nil {
		return nil, err
	}
	return &primitiveapi.GetPrimitiveResponse{
		Primitive: primitiveapi.PrimitiveMetadata{
			Database: databaseapi.DatabaseId{
				Namespace: primitive.Namespace,
				Name:      primitive.Properties.Database,
			},
			Primitive: primitiveapi.PrimitiveId{
				Namespace: primitive.Properties.Namespace,
				Name:      primitive.Properties.Name,
			},
			Type: primitive.Properties.Type.Proto(),
		},
	}, nil
}

func (c *Controller) GetPrimitives(ctx context.Context, request *primitiveapi.GetPrimitivesRequest) (*primitiveapi.GetPrimitivesResponse, error) {
	if request.Database != nil && request.Type != primitiveapi.PrimitiveType_UNKNOWN && request.Primitive != nil {
		database := &v1beta3.Database{}
		databaseName := types.NamespacedName{
			Namespace: request.Database.Namespace,
			Name:      request.Database.Name,
		}
		err := c.client.Get(ctx, databaseName, database)
		if err != nil {
			return nil, err
		}

		primitive := &v1beta3.Primitive{}
		primitiveName := getPrimitiveNamespacedName(*request.Database, *request.Primitive)
		err = c.client.Get(ctx, primitiveName, primitive)
		if err != nil {
			return nil, err
		}

		if primitive.Properties.Type.Proto() != request.Type {
			return nil, status.Error(codes.NotFound, "primitive not found")
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: []primitiveapi.PrimitiveMetadata{
				{
					Database: databaseapi.DatabaseId{
						Namespace: primitive.Namespace,
						Name:      primitive.Properties.Database,
					},
					Primitive: primitiveapi.PrimitiveId{
						Namespace: primitive.Properties.Namespace,
						Name:      primitive.Properties.Name,
					},
					Type: primitive.Properties.Type.Proto(),
				},
			},
		}, nil
	} else if request.Database != nil && request.Type != primitiveapi.PrimitiveType_UNKNOWN {
		if request.Database.Name != "" {
			database := &v1beta3.Database{}
			databaseName := types.NamespacedName{
				Namespace: request.Database.Namespace,
				Name:      request.Database.Name,
			}
			err := c.client.Get(ctx, databaseName, database)
			if err != nil {
				return nil, err
			}
		}

		primitivesList := &v1beta3.PrimitiveList{}
		primitivesListFields := map[string]string{
			"properties.type": string(v1beta3.GetPrimitiveType(request.Type)),
		}
		if request.Database.Name != "" {
			primitivesListFields["properties.protocol"] = request.Database.Name
		}
		primitivesListOpts := &client.ListOptions{
			Namespace:     request.Database.Namespace,
			FieldSelector: fields.SelectorFromSet(primitivesListFields),
		}
		err := c.client.List(ctx, primitivesList, primitivesListOpts)
		if err != nil {
			return nil, err
		}
		primitives := make([]primitiveapi.PrimitiveMetadata, 0, len(primitivesList.Items))
		for _, primitive := range primitivesList.Items {
			primitives = append(primitives, primitiveapi.PrimitiveMetadata{
				Database: databaseapi.DatabaseId{
					Namespace: primitive.Namespace,
					Name:      primitive.Properties.Database,
				},
				Primitive: primitiveapi.PrimitiveId{
					Namespace: primitive.Properties.Namespace,
					Name:      primitive.Properties.Name,
				},
				Type: primitive.Properties.Type.Proto(),
			})
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: primitives,
		}, nil
	} else if request.Database != nil {
		if request.Database.Name != "" {
			database := &v1beta3.Database{}
			databaseName := types.NamespacedName{
				Namespace: request.Database.Namespace,
				Name:      request.Database.Name,
			}
			err := c.client.Get(ctx, databaseName, database)
			if err != nil {
				return nil, err
			}
		}

		primitivesList := &v1beta3.PrimitiveList{}
		primitivesListFields := map[string]string{}
		if request.Database.Name != "" {
			primitivesListFields["properties.protocol"] = request.Database.Name
		}
		primitivesListOpts := &client.ListOptions{
			Namespace:     request.Database.Namespace,
			FieldSelector: fields.SelectorFromSet(primitivesListFields),
		}
		err := c.client.List(ctx, primitivesList, primitivesListOpts)
		if err != nil {
			return nil, err
		}
		primitives := make([]primitiveapi.PrimitiveMetadata, 0, len(primitivesList.Items))
		for _, primitive := range primitivesList.Items {
			primitives = append(primitives, primitiveapi.PrimitiveMetadata{
				Database: databaseapi.DatabaseId{
					Namespace: primitive.Namespace,
					Name:      primitive.Properties.Database,
				},
				Primitive: primitiveapi.PrimitiveId{
					Namespace: primitive.Properties.Namespace,
					Name:      primitive.Properties.Name,
				},
				Type: primitive.Properties.Type.Proto(),
			})
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: primitives,
		}, nil
	} else {
		primitivesList := &v1beta3.PrimitiveList{}
		err := c.client.List(ctx, primitivesList)
		if err != nil {
			return nil, err
		}
		primitives := make([]primitiveapi.PrimitiveMetadata, 0, len(primitivesList.Items))
		for _, primitive := range primitivesList.Items {
			primitives = append(primitives, primitiveapi.PrimitiveMetadata{
				Database: databaseapi.DatabaseId{
					Namespace: primitive.Namespace,
					Name:      primitive.Properties.Database,
				},
				Primitive: primitiveapi.PrimitiveId{
					Namespace: primitive.Properties.Namespace,
					Name:      primitive.Properties.Name,
				},
				Type: primitive.Properties.Type.Proto(),
			})
		}
		return &primitiveapi.GetPrimitivesResponse{
			Primitives: primitives,
		}, nil
	}
}

func (c *Controller) DeletePrimitive(ctx context.Context, request *primitiveapi.DeletePrimitiveRequest) (*primitiveapi.DeletePrimitiveResponse, error) {
	database := &v1beta3.Database{}
	databaseName := types.NamespacedName{
		Namespace: request.Database.Namespace,
		Name:      request.Database.Name,
	}
	err := c.client.Get(ctx, databaseName, database)
	if err != nil {
		return nil, err
	}

	primitive := &v1beta3.Primitive{}
	primitiveName := getPrimitiveNamespacedName(request.Database, request.Primitive)
	err = c.client.Get(ctx, primitiveName, primitive)
	if err != nil {
		return nil, err
	}

	err = c.client.Delete(ctx, primitive)
	if err != nil {
		return nil, err
	}
	return &primitiveapi.DeletePrimitiveResponse{
		Primitive: primitiveapi.PrimitiveMetadata{
			Database: databaseapi.DatabaseId{
				Namespace: primitive.Namespace,
				Name:      primitive.Properties.Database,
			},
			Primitive: primitiveapi.PrimitiveId{
				Namespace: primitive.Properties.Namespace,
				Name:      primitive.Properties.Name,
			},
			Type: primitive.Properties.Type.Proto(),
		},
	}, nil
}
