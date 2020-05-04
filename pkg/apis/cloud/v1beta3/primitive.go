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
	"github.com/atomix/api/proto/atomix/primitive"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrimitiveType is the type of a primitive
type PrimitiveType string

const (
	PrimitiveTypeUnknown     PrimitiveType = "Unknown"
	PrimitiveTypeCounter     PrimitiveType = "Counter"
	PrimitiveTypeElection    PrimitiveType = "Election"
	PrimitiveTypeIndexedMap  PrimitiveType = "IndexedMap"
	PrimitiveTypeLeaderLatch PrimitiveType = "LeaderLatch"
	PrimitiveTypeList        PrimitiveType = "List"
	PrimitiveTypeLock        PrimitiveType = "Lock"
	PrimitiveTypeLog         PrimitiveType = "Log"
	PrimitiveTypeMap         PrimitiveType = "Map"
	PrimitiveTypeSet         PrimitiveType = "Set"
	PrimitiveTypeValue       PrimitiveType = "Value"
)

// GetProto returns the primitive type proto
func (t PrimitiveType) Proto() primitive.PrimitiveType {
	switch t {
	case PrimitiveTypeCounter:
		return primitive.PrimitiveType_COUNTER
	case PrimitiveTypeElection:
		return primitive.PrimitiveType_ELECTION
	case PrimitiveTypeIndexedMap:
		return primitive.PrimitiveType_INDEXED_MAP
	case PrimitiveTypeLeaderLatch:
		return primitive.PrimitiveType_LEADER_LATCH
	case PrimitiveTypeList:
		return primitive.PrimitiveType_LIST
	case PrimitiveTypeLock:
		return primitive.PrimitiveType_LOCK
	case PrimitiveTypeLog:
		return primitive.PrimitiveType_LOG
	case PrimitiveTypeMap:
		return primitive.PrimitiveType_MAP
	case PrimitiveTypeSet:
		return primitive.PrimitiveType_SET
	case PrimitiveTypeValue:
		return primitive.PrimitiveType_VALUE
	}
	return primitive.PrimitiveType_UNKNOWN
}

// GetPrimitiveType returns the primitive type for the given Protobuf type
func GetPrimitiveType(t primitive.PrimitiveType) PrimitiveType {
	switch t {
	case primitive.PrimitiveType_COUNTER:
		return PrimitiveTypeCounter
	case primitive.PrimitiveType_ELECTION:
		return PrimitiveTypeElection
	case primitive.PrimitiveType_INDEXED_MAP:
		return PrimitiveTypeIndexedMap
	case primitive.PrimitiveType_LEADER_LATCH:
		return PrimitiveTypeLeaderLatch
	case primitive.PrimitiveType_LIST:
		return PrimitiveTypeList
	case primitive.PrimitiveType_LOCK:
		return PrimitiveTypeLock
	case primitive.PrimitiveType_LOG:
		return PrimitiveTypeLog
	case primitive.PrimitiveType_MAP:
		return PrimitiveTypeMap
	case primitive.PrimitiveType_SET:
		return PrimitiveTypeSet
	case primitive.PrimitiveType_VALUE:
		return PrimitiveTypeValue
	}
	return PrimitiveTypeUnknown
}

// PrimitiveProperties contains the properties for a Primitive resource
type PrimitiveProperties struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Protocol          string        `json:"protocol,omitempty"`
	Type              PrimitiveType `json:"type,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Primitive is the Schema for the primitive API
// +k8s:openapi-gen=true
type Primitive struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Properties        PrimitiveProperties `json:"properties,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PrimitiveList contains a list of Primitive
type PrimitiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the set of items in the list
	Items []Primitive `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Primitive{}, &PrimitiveList{})
}
