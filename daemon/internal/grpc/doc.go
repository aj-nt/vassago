// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// Package grpc implements the Vassago gRPC service for the daemon.
//
// It serves all RPCs defined in the Vassago protocol: memory CRUD, hot block,
// session management, agent registration, heartbeat, Telepathy subscriptions,
// consolidation, cron jobs, messaging, and todo management.
//
// The service layer sits between the gRPC transport and the store. It validates
// requests, calls the appropriate store methods, and returns protocol-buffer
// responses. Interceptors handle logging, panic recovery, and API key auth.
package grpc
