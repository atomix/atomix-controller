// SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	protocolapi "github.com/atomix/atomix-api/go/atomix/protocol"
	"github.com/atomix/atomix-go-framework/pkg/atomix/broker"
	"github.com/atomix/atomix-go-framework/pkg/atomix/cluster"
	"github.com/atomix/atomix-go-framework/pkg/atomix/logging"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logging.SetLevel(logging.InfoLevel)

	cmd := &cobra.Command{
		Use: "atomix-broker",
	}

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Create a new broker node
	broker := broker.NewBroker(cluster.NewCluster(
		cluster.NewNetwork(),
		protocolapi.ProtocolConfig{},
		cluster.WithMemberID("atomix-broker"),
		cluster.WithPort(5678)))

	// Start the node
	if err := broker.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Wait for an interrupt signal
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch

	// Stop the node after an interrupt
	if err := broker.Stop(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
