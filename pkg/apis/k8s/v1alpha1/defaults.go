/*
 * Copyright 2019 Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

func SetDefaults_Partition(partition *Partition) {
	if partition.Spec.Version == "" {
		partition.Spec.Version = "latest"
	}
	if partition.Spec.Size == 0 {
		partition.Spec.Size = 1
	}
	if partition.Spec.Raft != nil {
		SetDefaults_RaftProtocol(partition.Spec.Raft)
	} else if partition.Spec.PrimaryBackup != nil {
		SetDefaults_PrimaryBackupProtocol(partition.Spec.PrimaryBackup)
	} else if partition.Spec.Log != nil {
		SetDefaults_DistributedLogProtocol(partition.Spec.Log)
	}
}

func SetDefaults_PartitionGroup(group *PartitionGroup) {
	if group.Spec.Version == "" {
		group.Spec.Version = "3.1.0"
	}
	if group.Spec.Size == 0 {
		group.Spec.Size = 1
	}
	if group.Spec.Partitions == 0 {
		group.Spec.Partitions = int(group.Spec.Size)
	}
	if group.Spec.PartitionSize == 0 {
		group.Spec.PartitionSize = 1
	}
	if group.Spec.MemberGroupStrategy == "" {
		group.Spec.MemberGroupStrategy = NodeAwareMemberGroupStrategy
	}
	if group.Spec.Raft != nil {
		SetDefaults_RaftProtocol(group.Spec.Raft)
	} else if group.Spec.PrimaryBackup != nil {
		SetDefaults_PrimaryBackupProtocol(group.Spec.PrimaryBackup)
	} else if group.Spec.Log != nil {
		SetDefaults_DistributedLogProtocol(group.Spec.Log)
	}
}

// Sets default options on the Raft partition group
func SetDefaults_RaftProtocol(protocol *RaftProtocol) {
	SetDefaults_Storage(&protocol.Storage)
	SetDefaults_Compaction(&protocol.Compaction)
}

// Sets default options on the primary-backup partition group
func SetDefaults_PrimaryBackupProtocol(protocol *PrimaryBackupProtocol) {
}

// Sets default options on the log partition group
func SetDefaults_DistributedLogProtocol(group *DistributedLogProtocol) {
	SetDefaults_Storage(&group.Storage)
	SetDefaults_Compaction(&group.Compaction)
}

func SetDefaults_Storage(storage *Storage) {
	if storage.Size == "" {
		storage.Size = "2G"
	}
	if storage.SegmentSize == "" {
		storage.SegmentSize = "64M"
	}
	if storage.EntrySize == "" {
		storage.EntrySize = "1K"
	}
	if storage.Level == "" {
		storage.Level = MappedStorage
	}
}

func SetDefaults_Compaction(compaction *Compaction) {
	if compaction.FreeDiskBuffer == 0 {
		compaction.FreeDiskBuffer = .2
	}
	if compaction.FreeMemoryBuffer == 0 {
		compaction.FreeMemoryBuffer = .2
	}
}
