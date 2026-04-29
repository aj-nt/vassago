package provider

import (
	"fmt"
	"io"
	"net/http"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// classifyHTTPResponse reads the response body and classifies it into a ProviderResult.
// Handles 429 (rate limit), 5xx (server error), and non-200 (client error) cases.
func classifyHTTPResponse(resp *http.Response) agentctx.ProviderResult {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return agentctx.ProviderResult{Error: fmt.Errorf("read response body: %w", err)}
	}

	switch {
	case resp.StatusCode == 429:
		return agentctx.ProviderResult{
			ShouldRetry:    true,
			ShouldFallback: true,
			Error:          fmt.Errorf("rate limited: %s", string(body)),
		}
	case resp.StatusCode >= 500:
		return agentctx.ProviderResult{
			ShouldRetry:    resp.StatusCode == 503,
			ShouldFallback: resp.StatusCode == 429,
			Error:          fmt.Errorf("server error %d: %s", resp.StatusCode, string(body)),
		}
	case resp.StatusCode != 200:
		return agentctx.ProviderResult{
			ShouldRetry: false,
			Error:       fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)),
		}
	default:
		return agentctx.ProviderResult{RawResponse: body}
	}
}