// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package replication

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PeerConfig holds configuration for connecting to a peer daemon.
type PeerConfig struct {
	ID         string
	Addr       string
	Namespaces []string
}

// Peer represents a connection to a remote daemon.
type Peer struct {
	ID         string
	Addr       string
	Namespaces []string
	conn       *grpc.ClientConn
	client     pb.VassagoClient
}

// NewPeer creates a new peer (does not connect yet).
func NewPeer(cfg PeerConfig) *Peer {
	return &Peer{
		ID:         cfg.ID,
		Addr:       cfg.Addr,
		Namespaces: cfg.Namespaces,
	}
}

// Connect establishes a gRPC connection to the peer.
func (p *Peer) Connect(ctx context.Context) error {
	// nolint:staticcheck // SA1019: DialContext remains supported throughout 1.x
	conn, err := grpc.DialContext(ctx, p.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", p.Addr, err)
	}
	p.conn = conn
	p.client = pb.NewVassagoClient(conn)
	slog.Info("replication: connected to peer", "peer_id", p.ID, "addr", p.Addr)
	return nil
}

// Close disconnects from the peer.
func (p *Peer) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// SyncChanges pulls memory deltas from the peer for a namespace since the given timestamp.
func (p *Peer) SyncChanges(ctx context.Context, namespace string, since int64) (*pb.SyncChangesResponse, error) {
	if p.client == nil {
		return nil, fmt.Errorf("peer %s not connected", p.ID)
	}
	return p.client.SyncChanges(ctx, &pb.SyncChangesRequest{
		PeerId:    p.ID,
		Namespace: namespace,
		Since:     since,
	})
}
