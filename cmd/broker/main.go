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
	"fmt"
	protocolapi "github.com/atomix/api/go/atomix/protocol"
	"github.com/atomix/go-framework/pkg/atomix/broker"
	"github.com/atomix/go-framework/pkg/atomix/cluster"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
)

func main() {
	cmd := &cobra.Command{
		Use: "atomix-broker",
	}
	cmd.Flags().IntP("port", "p", 5678, "the port to which to bind the broker")

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	node := broker.NewNode(cluster.NewCluster(
		protocolapi.ProtocolConfig{},
		cluster.WithMemberID("broker"),
		cluster.WithPort(port)))

	if err := node.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
