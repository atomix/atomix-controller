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

package main

import (
	"bytes"
	"fmt"
	"github.com/atomix/api/go/atomix/protocol"
	"github.com/atomix/go-framework/pkg/atomix/cluster"
	"github.com/atomix/go-framework/pkg/atomix/coordinator"
	"github.com/golang/protobuf/jsonpb"
	"io/ioutil"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
)

func main() {
	configFile := os.Args[1]
	configBytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	config := protocol.ProtocolConfig{}
	if err := jsonpb.Unmarshal(bytes.NewReader(configBytes), &config); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	node := coordinator.NewNode(cluster.NewCluster(config))
	if err := node.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
