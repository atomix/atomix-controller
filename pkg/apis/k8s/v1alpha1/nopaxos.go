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

package v1alpha1

import corev1 "k8s.io/api/core/v1"

// NOPaxosProtocol is a configuration for the NOPaxos consensus protocol
type NOPaxosProtocol struct {
	// Sequencer is the NOPaxos sequencer spec
	Sequencer NOPaxosSequencerSpec

	// Protocol is the NOPaxos protocol spec
	Protocol NOPaxosProtocolSpec
}

// NOPaxosSequencerSpec is the spec for the NOPaxos sequencer
type NOPaxosSequencerSpec struct {
	// Image is the image to run in the partition. If the image is unspecified, the default image for the protocol
	// will be used.
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the pull policy to apply to partition images
	ImagePullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

// NOPaxosProtocolSpec is the spec for the NOPaxos protocol
type NOPaxosProtocolSpec struct {
	// Image is the image to run in the partition. If the image is unspecified, the default image for the protocol
	// will be used.
	Image string `json:"image,omitempty"`

	// ImagePullPolicy is the pull policy to apply to partition images
	ImagePullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// LeaderTimeoutMillis is the milliseconds to wait before attempting to elect a new leader
	LeaderTimeoutMillis int32 `json:"leaderTimeoutMillis,omitempty"`

	// PingPeriodMillis is the milliseconds at which to send pings between replicas
	PingPeriodMillis int32 `json:"pingPeriodMillis,omitempty"`

	// CheckpointPeriodSeconds is the seconds at which to checkpoint the state of replicas
	CheckpointPeriodSeconds int32 `json:"checkpointPeriodSeconds,omitempty"`

	// SyncPeriodSeconds is the seconds at which to synchronize the state of replicas
	SyncPeriodSeconds int32 `json:"syncPeriodSeconds,omitempty"`
}
