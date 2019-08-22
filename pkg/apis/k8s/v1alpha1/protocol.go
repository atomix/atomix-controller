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

import (
	"github.com/atomix/atomix-api/proto/atomix/protocols/log"
	"github.com/atomix/atomix-api/proto/atomix/protocols/raft"
	"github.com/atomix/atomix-k8s-controller/pkg/controller/protocol"
)

func init() {
	protocol.Registry.Register("raft", "atomix/atomix-raft-protocol:latest", &raft.RaftProtocol{})
	protocol.Registry.Register("log", "atomix/atomix-raft-protocol:latest", &log.LogProtocol{})
}
