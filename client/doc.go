// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

// Package client provides a Go gRPC client for the Mnemosyne memory daemon.
//
// Connect to a running daemon and perform memory operations: add, search, replace,
// remove, and retrieve the computed hot block. Also supports session management,
// agent registration, heartbeat, and Telepathy subscriptions.
//
// For resilient connections with automatic reconnection, use ResilientClient.
// For agent lifecycle management, use AgentSession.
//
// Basic usage:
//
//	conn, err := client.Connect("localhost:50051")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
//	c := client.NewClient(conn)
//	hb, err := c.GetHotBlock(context.Background(), "memory", 8000)
package client
