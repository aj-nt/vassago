// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestRetryLogic(t *testing.T) {
	t.Run("Retry on transient errors", func(t *testing.T) {
		attempts := 0
		config := RetryConfig{
			MaxRetries: 3,
			BaseDelay:  10 * time.Millisecond,
			MaxDelay:   50 * time.Millisecond,
			ShouldRetry: func(err error) bool {
				return errors.Is(err, context.DeadlineExceeded)
			},
		}

		_, err := WithRetry(context.Background(), config, func() (agentctx.ProviderResult, error) {
			attempts++
			if attempts < 3 {
				return agentctx.ProviderResult{}, context.DeadlineExceeded
			}
			return agentctx.ProviderResult{RawResponse: []byte("success")}, nil
		})

		if err != nil {
			t.Errorf("Expected success after retries, got error: %v", err)
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("No retry on permanent errors", func(t *testing.T) {
		attempts := 0
		config := RetryConfig{
			MaxRetries: 3,
			BaseDelay:  10 * time.Millisecond,
			ShouldRetry: func(err error) bool {
				return false // Never retry
			},
		}

		_, err := WithRetry(context.Background(), config, func() (agentctx.ProviderResult, error) {
			attempts++
			return agentctx.ProviderResult{}, errors.New("permanent error")
		})

		if err == nil {
			t.Error("Expected error, got success")
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("Retry on HTTP 503 errors", func(t *testing.T) {
		attempts := 0
		config := DefaultRetryConfig()

		_, err := WithRetry(context.Background(), config, func() (agentctx.ProviderResult, error) {
			attempts++
			if attempts < 2 {
				resp := &http.Response{
					StatusCode: 503,
					Body:       http.NoBody,
				}
				return classifyHTTPResponse(resp), nil
			}
			return agentctx.ProviderResult{RawResponse: []byte("success")}, nil
		})

		if err != nil {
			t.Errorf("Expected success after retries, got error: %v", err)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})
}

func TestHTTPClientTimeout(t *testing.T) {
	client := NewHTTPClient(1 * time.Millisecond)
	if client.Timeout != 1*time.Millisecond {
		t.Errorf("Expected timeout of 1ms, got %v", client.Timeout)
	}

	// Test that the client has reasonable defaults for connection pooling
	if client.Transport.(*http.Transport).MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns of 100, got %d", client.Transport.(*http.Transport).MaxIdleConns)
	}
}
