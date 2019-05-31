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

type PersistentProtocol struct {
	Storage    Storage    `json:"storage,omitempty"`
	Compaction Compaction `json:"compaction,omitempty"`
}

type RaftProtocol struct {
	PersistentProtocol `json:",inline"`
	ElectionTimeout    *int64 `json:"electionTimeout,omitempty"`
	HeartbeatInterval  *int64 `json:"heartbeatInterval,omitempty"`
}

type PrimaryBackupProtocol struct {
}

type DistributedLogProtocol struct {
	PersistentProtocol `json:",inline"`
}

// StorageLevel describes the storage level of commit logs.
type StorageLevel string

const (
	DiskStorage   StorageLevel = "disk"
	MappedStorage StorageLevel = "mapped"
)

type Storage struct {
	Size          string       `json:"size,omitempty"`
	ClassName     *string      `json:"className,omitempty"`
	SegmentSize   string       `json:"segmentSize,omitempty"`
	EntrySize     string       `json:"entrySize,omitempty"`
	Level         StorageLevel `json:"level,omitempty"`
	FlushOnCommit bool         `json:"flushOnCommit,omitempty"`
}

type Compaction struct {
	Dynamic          bool    `json:"dynamic,omitempty"`
	FreeDiskBuffer   float32 `json:"freeDiskBuffer,omitempty"`
	FreeMemoryBuffer float32 `json:"freeMemoryBuffer,omitempty"`
}
