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
	driverapi "github.com/atomix/api/go/atomix/driver"
	protocolapi "github.com/atomix/api/go/atomix/protocol"
	proxyapi "github.com/atomix/api/go/atomix/proxy"
	"github.com/atomix/go-framework/pkg/atomix/cluster"
	"github.com/atomix/go-framework/pkg/atomix/coordinator"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
)

func main() {
	cmd := &cobra.Command{
		Use: "atomix-coordinator",
	}
	cmd.Flags().IntP("port", "p", 5678, "the port to which to bind the coordinator")
	cmd.Flags().StringToIntP("drivers", "d", map[string]int{}, "a mapping of drivers to sidecar ports")

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	drivers, err := cmd.Flags().GetStringToInt("drivers")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	node := coordinator.NewNode(cluster.NewCluster(
		protocolapi.ProtocolConfig{},
		cluster.WithMemberID("coordinator"),
		cluster.WithPort(port)))

	for driver, port := range drivers {
		node.RegisterDriver(driverapi.DriverMeta{
			Name: driver,
			Proxy: proxyapi.ProxyMeta{
				Port: int32(port),
			},
		})
	}

	if err := node.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
