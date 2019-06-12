package k8s

import (
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/protocol"
	"github.com/atomix/atomix-k8s-controller/proto/atomix/controller"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func GetPartitionGroupName(id *controller.PartitionGroupId) string {
	return id.Name
}

func GetPartitionGroupNamespace(id *controller.PartitionGroupId) string {
	if id.Namespace != "" {
		return id.Namespace
	} else {
		return DefaultNamespace
	}
}

func GetPartitionGroupNamespacedName(id *controller.PartitionGroupId) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionGroupName(id),
		Namespace: GetPartitionGroupNamespace(id),
	}
}

// GetPartitionGroupServiceName returns the service name for a partition group
func GetPartitionGroupServiceName(group *v1alpha1.PartitionGroup) string {
	return group.Name
}

// GetPartitionGroupServiceNamespacedName returns the namespaced service name for a partition group
func GetPartitionGroupServiceNamespacedName(group *v1alpha1.PartitionGroup) types.NamespacedName {
	return types.NamespacedName{
		Name:      GetPartitionGroupServiceName(group),
		Namespace: group.Namespace,
	}
}

// NewPartitionGroupService returns a new service for a partition group
func NewPartitionGroupService(group *v1alpha1.PartitionGroup) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetPartitionGroupServiceName(group),
			Namespace: group.Namespace,
			Labels:    group.Labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "api",
					Port: 5678,
				},
			},
		},
	}
}

// NewPartitionGroupEndpoints returns an Endpoints object for the given partition group
func NewPartitionGroupEndpoints(group *v1alpha1.PartitionGroup) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group.Name,
			Namespace: group.Namespace,
		},
		Subsets: []corev1.EndpointSubset{},
	}
}

// NewPartitionGroupEndpointPorts returns an EndpointPort for a partition group
func NewPartitionGroupEndpointPorts() []corev1.EndpointPort {
	return []corev1.EndpointPort{
		{
			Name: "api",
			Port: 5678,
		},
	}
}

func NewPartitionProto(p *v1alpha1.Partition) (*controller.Partition, error) {
	id, err := getPartitionIdFromAnnotation(p)
	if err != nil {
		return nil, err
	}
	return &controller.Partition{
		PartitionId: int32(id),
		Endpoints: []*controller.PartitionEndpoint{
			{
				Host: GetPartitionServiceName(p),
				Port: 5678,
			},
		},
	}, nil
}

func NewPartitionGroupProto(group *v1alpha1.PartitionGroup, protocols *protocol.ProtocolManager) (*controller.PartitionGroup, error) {
	spec, err := newPartitionGroupSpecProto(group, protocols)
	if err != nil {
		return nil, err
	}
	return &controller.PartitionGroup{
		Id: &controller.PartitionGroupId{
			Name:      group.Name,
			Namespace: group.Namespace,
		},
		Spec: spec,
	}, nil
}

func newPartitionGroupSpecProto(group *v1alpha1.PartitionGroup, protocols *protocol.ProtocolManager) (*controller.PartitionGroupSpec, error) {
	protocol, err := protocols.GetProtocolByName(group.Spec.Protocol)
	if err != nil {
		return nil, err
	}

	message, err := protocol.YamlToMessage([]byte(group.Spec.Config))
	if err != nil {
		return nil, err
	}
	bytes, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}

	return &controller.PartitionGroupSpec{
		Partitions:    uint32(group.Spec.Partitions),
		PartitionSize: uint32(group.Spec.PartitionSize),
		Protocol: &any.Any{
			TypeUrl: "type.googleapis.com/" + proto.MessageName(message),
			Value:   bytes,
		},
	}, nil
}

func NewPartitionGroup(id *controller.PartitionGroupId, pbspec *controller.PartitionGroupSpec, protocols *protocol.ProtocolManager) (*v1alpha1.PartitionGroup, error) {
	protocol, err := protocols.GetProtocolByType(pbspec.Protocol.TypeUrl)
	if err != nil {
		return nil, err
	}

	ns := id.Namespace
	if ns == "" {
		ns = DefaultNamespace
	}

	yaml, err := protocol.ProtoToYaml(pbspec.Protocol.Value)
	if err != nil {
		return nil, err
	}

	return &v1alpha1.PartitionGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id.Name,
			Namespace: id.Namespace,
			Labels:    newPartitionGroupLabels(id.Name),
		},
		Spec: v1alpha1.PartitionGroupSpec{
			Partitions:    int(pbspec.Partitions),
			PartitionSize: int(pbspec.PartitionSize),
			Protocol:      protocol.Name,
			Image:         protocol.Image,
			Config:        string(yaml),
		},
	}, nil
}

// newPartitionLabels returns a new labels map containing the partition group info
func newPartitionGroupLabels(group string) map[string]string {
	return map[string]string{
		AppKey:   AtomixApp,
		TypeKey:  GroupType,
		GroupKey: group,
	}
}
