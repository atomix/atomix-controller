package util

import (
	"errors"
	"github.com/atomix/atomix-k8s-controller/pkg/apis/k8s/v1alpha1"
	"github.com/atomix/atomix-k8s-controller/proto/atomix/partition"
	"github.com/golang/protobuf/ptypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strconv"
	"time"
)

func GetPartitionGroupName(id *partition.PartitionGroupId) string {
	return id.Name
}

func GetPartitionGroupNamespace(id *partition.PartitionGroupId) string {
	if id.Namespace != "" {
		return id.Namespace
	} else {
		return DefaultNamespace
	}
}

func GetPartitionGroupNamespacedName(id *partition.PartitionGroupId) types.NamespacedName {
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

func NewPartitionProto(p *v1alpha1.Partition) (*partition.Partition, error) {
	id, err := getPartitionIdFromAnnotation(p)
	if err != nil {
		return nil, err
	}
	return &partition.Partition{
		PartitionId: int32(id),
		Endpoints: []*partition.PartitionEndpoint{
			{
				Host: GetPartitionServiceName(p),
				Port: 5678,
			},
		},
	}, nil
}

func NewPartitionGroupProto(group *v1alpha1.PartitionGroup) (*partition.PartitionGroup, error) {
	spec, err := newPartitionGroupSpecProto(group)
	if err != nil {
		return nil, err
	}
	return &partition.PartitionGroup{
		Id: &partition.PartitionGroupId{
			Name:      group.Name,
			Namespace: group.Namespace,
		},
		Spec: spec,
	}, nil
}

func newPartitionGroupSpecProto(group *v1alpha1.PartitionGroup) (*partition.PartitionGroupSpec, error) {
	if group.Spec.Raft != nil {
		return &partition.PartitionGroupSpec{
			Replicas:      uint32(group.Spec.PartitionSize),
			Partitions:    uint32(group.Spec.Partitions),
			PartitionSize: uint32(group.Spec.PartitionSize),
			Group: &partition.PartitionGroupSpec_Raft{
				Raft: newPartitionGroupRaftProto(group.Spec),
			},
		}, nil
	} else if group.Spec.PrimaryBackup != nil {
		return &partition.PartitionGroupSpec{
			Group: &partition.PartitionGroupSpec_PrimaryBackup{
				PrimaryBackup: newPartitionGroupPrimaryBackupProto(group.Spec),
			},
		}, nil
	} else if group.Spec.Log != nil {
		return &partition.PartitionGroupSpec{
			Group: &partition.PartitionGroupSpec_Log{
				Log: newPartitionGroupLogProto(group.Spec),
			},
		}, nil
	}
	return nil, errors.New("no partition group configuration specified")
}

func newPartitionGroupRaftProto(spec v1alpha1.PartitionGroupSpec) *partition.RaftPartitionGroup {
	group := &partition.RaftPartitionGroup{
		Storage:    newStorageProto(spec.Raft.Storage),
		Compaction: newCompactionProto(spec.Raft.Compaction),
	}
	if spec.Raft.ElectionTimeout != nil {
		group.ElectionTimeout = ptypes.DurationProto(time.Duration(*spec.Raft.ElectionTimeout))
	}
	if spec.Raft.HeartbeatInterval != nil {
		group.HeartbeatInterval = ptypes.DurationProto(time.Duration(*spec.Raft.HeartbeatInterval))
	}
	return group
}

func newPartitionGroupPrimaryBackupProto(spec v1alpha1.PartitionGroupSpec) *partition.PrimaryBackupPartitionGroup {
	return &partition.PrimaryBackupPartitionGroup{
		MemberGroupStrategy: newMemberGroupStrategyProto(spec.MemberGroupStrategy),
	}
}

func newPartitionGroupLogProto(spec v1alpha1.PartitionGroupSpec) *partition.DistributedLogPartitionGroup {
	return &partition.DistributedLogPartitionGroup{
		MemberGroupStrategy: newMemberGroupStrategyProto(spec.MemberGroupStrategy),
		Storage:             newStorageProto(spec.Log.Storage),
		Compaction:          newCompactionProto(spec.Log.Compaction),
	}
}

func newStorageProto(spec v1alpha1.Storage) *partition.StorageSpec {
	storage := &partition.StorageSpec{
		Level:         newStorageLevelProto(spec.Level),
		FlushOnCommit: spec.FlushOnCommit,
	}

	entrySize, err := strconv.ParseInt(spec.EntrySize, 0, 32)
	if err == nil {
		storage.MaxEntrySize = uint32(entrySize)
	}

	segmentSize, err := strconv.ParseInt(spec.SegmentSize, 0, 32)
	if err == nil {
		storage.SegmentSize = uint32(segmentSize)
	}
	return storage
}

func newStorageLevelProto(level v1alpha1.StorageLevel) partition.StorageLevel {
	if level == v1alpha1.DiskStorage {
		return partition.StorageLevel_DISK
	} else if level == v1alpha1.MappedStorage {
		return partition.StorageLevel_MAPPED
	}
	return partition.StorageLevel_DISK
}

func newMemberGroupStrategyProto(strategy v1alpha1.MemberGroupStrategy) partition.MemberGroupStrategy {
	if strategy == v1alpha1.NodeAwareMemberGroupStrategy {
		return partition.MemberGroupStrategy_HOST_AWARE
	} else if strategy == v1alpha1.RackAwareMemberGroupStrategy {
		return partition.MemberGroupStrategy_RACK_AWARE
	} else if strategy == v1alpha1.ZoneAwareMemberGroupStrategy {
		return partition.MemberGroupStrategy_ZONE_AWARE
	}
	return partition.MemberGroupStrategy_HOST_AWARE
}

func newCompactionProto(spec v1alpha1.Compaction) *partition.CompactionSpec {
	return &partition.CompactionSpec{
		Dynamic:        spec.Dynamic,
		FreeDiskBuffer: float64(spec.FreeDiskBuffer),
	}
}

func NewPartitionGroup(id *partition.PartitionGroupId, pbspec *partition.PartitionGroupSpec) *v1alpha1.PartitionGroup {
	spec := v1alpha1.PartitionGroupSpec{
		Version:       "latest",
		Partitions:    int(pbspec.Partitions),
		PartitionSize: int(pbspec.PartitionSize),
	}

	switch g := pbspec.Group.(type) {
	case *partition.PartitionGroupSpec_Raft:
		spec.Raft = newRaftPartitionGroup(g.Raft)
	case *partition.PartitionGroupSpec_PrimaryBackup:
		spec.PrimaryBackup = newPrimaryBackupPartitionGroup(g.PrimaryBackup)
		spec.MemberGroupStrategy = newMemberGroupStrategy(g.PrimaryBackup.MemberGroupStrategy)
	case *partition.PartitionGroupSpec_Log:
		spec.Log = newLogPartitionGroup(g.Log)
		spec.MemberGroupStrategy = newMemberGroupStrategy(g.Log.MemberGroupStrategy)
	}

	ns := id.Namespace
	if ns == "" {
		ns = DefaultNamespace
	}

	return &v1alpha1.PartitionGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id.Name,
			Namespace: id.Namespace,
			Labels:    newPartitionGroupLabels(id.Name),
		},
		Spec: spec,
	}
}

// newPartitionLabels returns a new labels map containing the partition group info
func newPartitionGroupLabels(group string) map[string]string {
	return map[string]string{
		AppKey:   AtomixApp,
		TypeKey:  GroupType,
		GroupKey: group,
	}
}

func newRaftPartitionGroup(pbspec *partition.RaftPartitionGroup) *v1alpha1.RaftProtocol {
	raft := &v1alpha1.RaftProtocol{
		PersistentProtocol: v1alpha1.PersistentProtocol{
			Storage:    newStorage(pbspec.Storage),
			Compaction: newCompaction(pbspec.Compaction),
		},
	}
	if pbspec.ElectionTimeout != nil {
		if d, err := ptypes.Duration(pbspec.ElectionTimeout); err == nil {
			t := int64(d * time.Millisecond)
			raft.ElectionTimeout = &t
		}
	}
	if pbspec.HeartbeatInterval != nil {
		if d, err := ptypes.Duration(pbspec.HeartbeatInterval); err == nil {
			t := int64(d * time.Millisecond)
			raft.HeartbeatInterval = &t
		}
	}
	return raft
}

func newPrimaryBackupPartitionGroup(pbspec *partition.PrimaryBackupPartitionGroup) *v1alpha1.PrimaryBackupProtocol {
	return &v1alpha1.PrimaryBackupProtocol{}
}

func newLogPartitionGroup(pbspec *partition.DistributedLogPartitionGroup) *v1alpha1.DistributedLogProtocol {
	return &v1alpha1.DistributedLogProtocol{
		PersistentProtocol: v1alpha1.PersistentProtocol{
			Storage:    newStorage(pbspec.Storage),
			Compaction: newCompaction(pbspec.Compaction),
		},
	}
}

func newStorage(pbspec *partition.StorageSpec) v1alpha1.Storage {
	storage := v1alpha1.Storage{
		Level:         newStorageLevel(pbspec.Level),
		FlushOnCommit: pbspec.FlushOnCommit,
	}
	if pbspec.SegmentSize > 0 {
		storage.SegmentSize = string(pbspec.SegmentSize)
	}
	if pbspec.MaxEntrySize > 0 {
		storage.EntrySize = string(pbspec.MaxEntrySize)
	}
	return storage
}

func newStorageLevel(level partition.StorageLevel) v1alpha1.StorageLevel {
	if level == partition.StorageLevel_DISK {
		return v1alpha1.DiskStorage
	} else if level == partition.StorageLevel_MAPPED {
		return v1alpha1.MappedStorage
	}
	return v1alpha1.DiskStorage
}

func newCompaction(pbspec *partition.CompactionSpec) v1alpha1.Compaction {
	return v1alpha1.Compaction{
		Dynamic:        pbspec.Dynamic,
		FreeDiskBuffer: float32(pbspec.FreeDiskBuffer),
	}
}

func newMemberGroupStrategy(strategy partition.MemberGroupStrategy) v1alpha1.MemberGroupStrategy {
	if strategy == partition.MemberGroupStrategy_HOST_AWARE {
		return v1alpha1.NodeAwareMemberGroupStrategy
	} else if strategy == partition.MemberGroupStrategy_RACK_AWARE {
		return v1alpha1.RackAwareMemberGroupStrategy
	} else if strategy == partition.MemberGroupStrategy_ZONE_AWARE {
		return v1alpha1.ZoneAwareMemberGroupStrategy
	}
	return v1alpha1.NodeAwareMemberGroupStrategy
}
