package provider

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries    int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	ShouldRetry   func(error) bool
	ShouldRetryHTTP func(*http.Response) bool
}

// DefaultRetryConfig provides sensible defaults for retry logic
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		ShouldRetry: func(err error) bool {
			return true // Retry on all errors by default
		},
		ShouldRetryHTTP: func(resp *http.Response) bool {
			// Retry on server errors and rate limits
			return resp.StatusCode >= 500 || resp.StatusCode == 429
		},
	}
}

// WithRetry executes a function with retry logic and exponential backoff
func WithRetry(ctx context.Context, config RetryConfig, fn func() (agentctx.ProviderResult, error)) (agentctx.ProviderResult, error) {
	var lastResult agentctx.ProviderResult
	var lastErr error

	for i := 0; i <= config.MaxRetries; i++ {
		// Execute the function
		lastResult, lastErr = fn()

		// If no error and no retry needed, return immediately
		if lastErr == nil && !lastResult.ShouldRetry {
			return lastResult, nil
		}

	// Check if we should retry based on error
	if lastErr != nil {
		if !config.ShouldRetry(lastErr) {
			break
		}
	} else {
		// No error, check if we should retry based on the result
		if !lastResult.ShouldRetry {
			break
		}

		// For HTTP responses, check if we should retry based on status code
		if lastResult.Error != nil {
			if httpResp, ok := lastResult.Error.(interface{ HTTPResponse() *http.Response }); ok {
				if !config.ShouldRetryHTTP(httpResp.HTTPResponse()) {
					break
				}
			}
		}
	}

		// If this was the last attempt, break
		if i == config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff and jitter
		delay := config.BaseDelay * time.Duration(1<<i)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
		// Add jitter to avoid thundering herd
		delay = delay + time.Duration(rand.Int63n(int64(delay/2))) //nolint:gosec // jitter doesn't need crypto rand

		// Wait for the delay or until context is cancelled
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return agentctx.ProviderResult{}, ctx.Err()
		}
	}

	return lastResult, lastErr
}

// NewHTTPClient creates an HTTP client with timeouts
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// Disable HTTP/2 to avoid potential issues with some APIs
			// ForceAttemptHTTP2: false,
			// Configure connection pool
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// HTTPError wraps an error with an HTTP response
type HTTPError struct {
	Response *http.Response
	Err      error
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %v", e.Response.StatusCode, e.Err)
}

func (e *HTTPError) HTTPResponse() *http.Response {
	return e.Response
}
