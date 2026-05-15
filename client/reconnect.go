// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	pb "github.com/aj-nt/vassago/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// ReconnectPolicy controls how the client reconnects after a disconnect.
type ReconnectPolicy struct {
	// MaxRetries is the maximum number of reconnection attempts (0 = unlimited).
	MaxRetries int
	// InitialBackoff is the first backoff duration.
	InitialBackoff time.Duration
	// MaxBackoff caps the exponential backoff.
	MaxBackoff time.Duration
	// Multiplier controls backoff growth (typically 2.0).
	Multiplier float64
}

// DefaultReconnectPolicy returns a sensible default policy.
func DefaultReconnectPolicy() ReconnectPolicy {
	return ReconnectPolicy{
		MaxRetries:     10,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
	}
}

// ResilientClient wraps Client with automatic reconnection on disconnect.
type ResilientClient struct {
	cfg    Config
	policy ReconnectPolicy
	mu     sync.Mutex
	client *Client
	closed bool
}

// NewResilientClient creates a client that automatically reconnects on failure.
func NewResilientClient(ctx context.Context, cfg Config, policy ReconnectPolicy) (*ResilientClient, error) {
	rc := &ResilientClient{cfg: cfg, policy: policy}
	if err := rc.connect(ctx); err != nil {
		return nil, err
	}
	return rc, nil
}

// connect attempts to establish a connection with exponential backoff.
func (rc *ResilientClient) connect(ctx context.Context) error {
	var lastErr error
	backoff := rc.policy.InitialBackoff

	for attempt := 0; attempt <= rc.policy.MaxRetries || rc.policy.MaxRetries == 0; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			// Exponential backoff
			backoff = time.Duration(math.Min(
				float64(backoff)*rc.policy.Multiplier,
				float64(rc.policy.MaxBackoff),
			))
		}

		c, err := Connect(ctx, rc.cfg)
		if err == nil {
			rc.mu.Lock()
			rc.client = c
			rc.mu.Unlock()
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("failed to connect after %d attempts: %w", rc.policy.MaxRetries, lastErr)
}

// isConnectionError returns true if the error indicates a connection problem
// that might be resolved by reconnecting.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	return s.Code() == codes.Unavailable || s.Code() == codes.Canceled
}

// getClient returns the current client, reconnecting if necessary.
func (rc *ResilientClient) getClient() (*Client, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.closed {
		return nil, fmt.Errorf("client is closed")
	}
	if rc.client == nil {
		return nil, fmt.Errorf("client not connected")
	}
	return rc.client, nil
}

// reconnect forces a reconnection to the daemon.
func (rc *ResilientClient) reconnect(ctx context.Context) error {
	rc.mu.Lock()
	if rc.closed {
		rc.mu.Unlock()
		return fmt.Errorf("client is closed")
	}
	if rc.client != nil {
		rc.client.Close()
		rc.client = nil
	}
	rc.mu.Unlock()
	return rc.connect(ctx)
}

// AddMemory adds or updates a memory entry, with automatic reconnection on connection failure.
func (rc *ResilientClient) AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error) {
	c, err := rc.getClient()
	if err != nil {
		return nil, err
	}
	result, err := c.AddMemory(ctx, target, category, key, content, priority, sourceAgent)
	if err != nil {
		// Try reconnecting once on connection error
		if isConnectionError(err) {
			if reconnectErr := rc.reconnect(ctx); reconnectErr != nil {
				return nil, reconnectErr
			}
			c, err = rc.getClient()
			if err != nil {
				return nil, err
			}
			return c.AddMemory(ctx, target, category, key, content, priority, sourceAgent)
		}
	}
	return result, err
}

// GetMemory retrieves a memory by ID.
func (rc *ResilientClient) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	c, err := rc.getClient()
	if err != nil {
		return nil, err
	}
	result, err := c.GetMemory(ctx, id)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return nil, rerr
		}
		if c, err = rc.getClient(); err != nil {
			return nil, err
		}
		return c.GetMemory(ctx, id)
	}
	return result, err
}

// RemoveMemory deletes a memory by ID.
func (rc *ResilientClient) RemoveMemory(ctx context.Context, id string) error {
	c, err := rc.getClient()
	if err != nil {
		return err
	}
	err = c.RemoveMemory(ctx, id)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return rerr
		}
		if c, err = rc.getClient(); err != nil {
			return err
		}
		return c.RemoveMemory(ctx, id)
	}
	return err
}

// SearchMemories searches memories using FTS5.
func (rc *ResilientClient) SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error) {
	c, err := rc.getClient()
	if err != nil {
		return nil, err
	}
	result, err := c.SearchMemories(ctx, query, target, limit)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return nil, rerr
		}
		if c, err = rc.getClient(); err != nil {
			return nil, err
		}
		return c.SearchMemories(ctx, query, target, limit)
	}
	return result, err
}

// ListMemories lists memories with optional filtering.
func (rc *ResilientClient) ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	c, err := rc.getClient()
	if err != nil {
		return nil, err
	}
	result, err := c.ListMemories(ctx, target, category, minPriority)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return nil, rerr
		}
		if c, err = rc.getClient(); err != nil {
			return nil, err
		}
		return c.ListMemories(ctx, target, category, minPriority)
	}
	return result, err
}

// GetHotBlock retrieves the computed hot block for an agent.
func (rc *ResilientClient) GetHotBlock(ctx context.Context, target, agentID string, charLimit int32, boostCategories, suppressCategories []string, boostPriority int32) (string, error) {
	c, err := rc.getClient()
	if err != nil {
		return "", err
	}
	result, err := c.GetHotBlock(ctx, target, agentID, charLimit, boostCategories, suppressCategories, boostPriority)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return "", rerr
		}
		if c, err = rc.getClient(); err != nil {
			return "", err
		}
		return c.GetHotBlock(ctx, target, agentID, charLimit, boostCategories, suppressCategories, boostPriority)
	}
	return result, err
}

// CreateSession creates a new session.
func (rc *ResilientClient) CreateSession(ctx context.Context, agentID, source, title string) (string, error) {
	c, err := rc.getClient()
	if err != nil {
		return "", err
	}
	result, err := c.CreateSession(ctx, agentID, source, title)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return "", rerr
		}
		if c, err = rc.getClient(); err != nil {
			return "", err
		}
		return c.CreateSession(ctx, agentID, source, title)
	}
	return result, err
}

// HealthCheck verifies the daemon is reachable and responding.
func (rc *ResilientClient) HealthCheck(ctx context.Context) error {
	c, err := rc.getClient()
	if err != nil {
		return err
	}
	err = c.HealthCheck(ctx)
	if err != nil && isConnectionError(err) {
		if rerr := rc.reconnect(ctx); rerr != nil {
			return rerr
		}
		if c, err = rc.getClient(); err != nil {
			return err
		}
		return c.HealthCheck(ctx)
	}
	return err
}

// Close closes the connection. No reconnection will be attempted.
func (rc *ResilientClient) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.closed = true
	if rc.client != nil {
		return rc.client.Close()
	}
	return nil
}

// RawConnection returns a raw gRPC connection for RPCs not in the client wrapper.
// This is useful for one-off RPCs like Consolidate that aren't in the client package.
func RawConnection(addr string) (*grpc.ClientConn, pb.VassagoClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}
	return conn, pb.NewVassagoClient(conn), nil
}
