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
	Registry = newRegistry()
)

func newRegistry() *ProtocolRegistry {
	return &ProtocolRegistry{
		protocolTypes: make(map[string]*ProtocolHandler),
		protocolNames: make(map[string]*ProtocolHandler),
	}
}

type ProtocolRegistry struct {
	protocolTypes map[string]*ProtocolHandler
	protocolNames map[string]*ProtocolHandler
}

func (r *ProtocolRegistry) Register(name string, image string, obj proto.Message) {
	typeOf := reflect.ValueOf(obj).Elem().Type()
	handler := &ProtocolHandler{
		Name:      name,
		Image:     image,
		protoType: typeOf,
	}
	r.protocolTypes[proto.MessageName(obj)] = handler
	r.protocolNames[name] = handler
}

func (r *ProtocolRegistry) getByType(name string) (*ProtocolHandler, error) {
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

func (r *ProtocolRegistry) getByName(name string) (*ProtocolHandler, error) {
	handler, ok := r.protocolNames[name]
	if !ok {
		return nil, errors.New("unknown protocol type " + name)
	}
	return handler, nil
}

func NewManager() *ProtocolManager {
	return &ProtocolManager{
		registry: Registry,
	}
}

type ProtocolManager struct {
	registry *ProtocolRegistry
}

func (m *ProtocolManager) GetProtocolByType(name string) (*ProtocolHandler, error) {
	return m.registry.getByType(name)
}

func (m *ProtocolManager) GetProtocolByName(name string) (*ProtocolHandler, error) {
	return m.registry.getByName(name)
}

type ProtocolHandler struct {
	Name      string
	Image     string
	protoType reflect.Type
}

func (h *ProtocolHandler) YamlToMessage(yaml []byte) (proto.Message, error) {
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

func (h *ProtocolHandler) YamlToJson(yaml []byte) ([]byte, error) {
	return yaml2.YAMLToJSON(yaml)
}

func (h *ProtocolHandler) ProtoToYaml(bytes []byte) ([]byte, error) {
	obj := reflect.New(h.protoType).Interface().(proto.Message)
	err := proto.Unmarshal(bytes, obj)
	if err != nil {
		return nil, err
	}
	return yaml2.Marshal(obj)
}
