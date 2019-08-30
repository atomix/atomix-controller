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
	"bytes"
	"errors"
	yaml2 "github.com/ghodss/yaml"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"reflect"
	"strings"
)

var (
	registry = newRegistry()
)

// Register registers a protocol implementation
func Register(name string, image string, obj proto.Message) {
	registry.register(name, image, obj)
}

// newRegistry creates a new protocol registry
func newRegistry() *Registry {
	return &Registry{
		protocolTypes: make(map[string]*Handler),
		protocolNames: make(map[string]*Handler),
	}
}

// Registry is a registry of supported protocol types
type Registry struct {
	protocolTypes map[string]*Handler
	protocolNames map[string]*Handler
}

// register registers a protocol type
func (r *Registry) register(name string, image string, obj proto.Message) {
	typeOf := reflect.ValueOf(obj).Elem().Type()
	handler := &Handler{
		Name:      name,
		Image:     image,
		protoType: typeOf,
	}
	r.protocolTypes[proto.MessageName(obj)] = handler
	r.protocolNames[name] = handler
}

// getByType gets a protocol handler by type
func (r *Registry) getByType(name string) (*Handler, error) {
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		name = parts[len(parts)-1]
	}
	handler, ok := r.protocolTypes[name]
	if !ok {
		return nil, errors.New("unknown protocol type " + name)
	}
	return handler, nil
}

// getByName gets a gets a protocol handler by name
func (r *Registry) getByName(name string) (*Handler, error) {
	handler, ok := r.protocolNames[name]
	if !ok {
		return nil, errors.New("unknown protocol type " + name)
	}
	return handler, nil
}

// NewManager creates a new protocol manager
func NewManager() *Manager {
	return &Manager{
		registry: registry,
	}
}

// Manager provides protocol type information from the registry
type Manager struct {
	registry *Registry
}

// GetProtocolByType looks up a protocol by its type
func (m *Manager) GetProtocolByType(name string) (*Handler, error) {
	return m.registry.getByType(name)
}

// GetProtocolByName looks up a protocol by its name
func (m *Manager) GetProtocolByName(name string) (*Handler, error) {
	return m.registry.getByName(name)
}

// Handler handles encoding and decoding of protocols to/from Protobuf message format and YAML for node configuration
type Handler struct {
	Name      string
	Image     string
	protoType reflect.Type
}

// YAMLToProto transcodes the given YAML protocol configuration to a Protobuf message
func (h *Handler) YAMLToProto(yaml []byte) (proto.Message, error) {
	json, err := yaml2.YAMLToJSON(yaml)
	if err != nil {
		return nil, err
	}

	obj := reflect.New(h.protoType).Interface().(proto.Message)
	err = jsonpb.Unmarshal(bytes.NewReader(json), obj)
	if err != nil {
		return nil, err
	}
	return obj, err
}

// YAMLToJSON transcodes the given YAML protocol configuration to JSON
func (h *Handler) YAMLToJSON(yaml []byte) ([]byte, error) {
	return yaml2.YAMLToJSON(yaml)
}

// ProtoToYAML transcodes the given Protobuf message to a YAML protocol configuration
func (h *Handler) ProtoToYAML(bytes []byte) ([]byte, error) {
	obj := reflect.New(h.protoType).Interface().(proto.Message)
	err := proto.Unmarshal(bytes, obj)
	if err != nil {
		return nil, err
	}
	return yaml2.Marshal(obj)
}
