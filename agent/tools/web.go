package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// --- HTTP Client ---

// HTTPClient wraps http.Client for dependency injection in tests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewHTTPClient returns a new HTTP client for web tools.
func NewHTTPClient() HTTPClient {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

// --- RegisterWebTools ---

// RegisterWebTools adds SearchWeb and FetchURL tool handlers to the registry.
func RegisterWebTools(registry *ToolRegistry, client HTTPClient) {
	if client == nil {
		client = NewHTTPClient()
	}
	registry.Register("SearchWeb", makeSearchWeb(client), SearchWebSchema)
	registry.Register("FetchURL", makeFetchURL(client), FetchURLSchema)
}

// --- SearchWeb ---

// searchBaseURL is the default SearXNG instance. Overridable for testing.
var searchBaseURL = "https://searx.be"

// SearchWebSchema is the JSON schema for the SearchWeb tool.
var SearchWebSchema = map[string]interface{}{
	"name":        "SearchWeb",
	"description": "Search the web for information. Returns a list of results with titles, URLs, and snippets.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (1-50, default 10)",
			},
		},
		"required": []string{"query"},
	},
}

func makeSearchWeb(client HTTPClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		query := strArg(args, "query", "")
		if query == "" {
			return "query is required", true
		}

		// Reject oversized queries
		const maxQueryLen = 4096
		if len(query) > maxQueryLen {
			return fmt.Sprintf("Query too long (%d chars, max %d)", len(query), maxQueryLen), true
		}

		limit := intArg(args, "limit", 10)
		if limit < 1 {
			limit = 1
		}
		if limit > 50 {
			limit = 50
		}

		// Call SearXNG API
		searchURL := fmt.Sprintf("%s/search?q=%s&format=json&limit=%d",
			searchBaseURL, url.QueryEscape(query), limit)

		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			return fmt.Sprintf("SearchWeb error: %v", err), true
		}
		req.Header.Set("User-Agent", "Mnemosyne/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("SearchWeb error: %v", err), true
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB max
		if err != nil {
			return fmt.Sprintf("SearchWeb error: %v", err), true
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("SearchWeb error: HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)])), true
		}

		// Parse SearXNG JSON response
		return parseSearXNGResponse(body, limit)
	}
}

// --- FetchURL ---

// FetchURLSchema is the JSON schema for the FetchURL tool.
var FetchURLSchema = map[string]interface{}{
	"name":        "FetchURL",
	"description": "Fetch a URL and extract its text content. Returns the page title and body text with HTML stripped.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to fetch (https:// or http:// only)",
			},
			"max_content_length": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum content length to download in bytes (default 100000)",
			},
		},
		"required": []string{"url"},
	},
}

func makeFetchURL(client HTTPClient) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		rawURL := strArg(args, "url", "")
		if rawURL == "" {
			return "url is required", true
		}

		// Validate URL scheme
		if !isAllowedURL(rawURL) {
			return fmt.Sprintf("URL blocked: %q is not an allowed URL (only https:// and http:// to public domains are allowed)", rawURL), true
		}

		maxContentLen := intArg(args, "max_content_length", 100000)
		if maxContentLen < 1000 {
			maxContentLen = 1000
		}
		if maxContentLen > 1000000 {
			maxContentLen = 1000000
		}

		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return fmt.Sprintf("FetchURL error: %v", err), true
		}
		req.Header.Set("User-Agent", "Mnemosyne/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("FetchURL error: %v", err), true
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("FetchURL error: HTTP %d", resp.StatusCode), true
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxContentLen)))
		if err != nil {
			return fmt.Sprintf("FetchURL error: %v", err), true
		}

		contentType := resp.Header.Get("Content-Type")

		// Extract text based on content type
		if strings.Contains(contentType, "html") || strings.Contains(contentType, "text/") {
			text := extractTextFromHTML(string(body))
			if len(text) > maxContentLen {
				text = text[:maxContentLen] + "\n\n[Content truncated]"
			}
			return text, false
		}

		// For non-text content, return size info
		return fmt.Sprintf("Binary content: %s (%d bytes)", contentType, len(body)), false
	}
}

// --- URL Safety ---

// isAllowedURL checks if a URL is safe to fetch (no localhost, no private IPs, no SSRF targets).
func isAllowedURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Only allow http and https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	// Block hostname-based private/local addresses
	host := parsed.Hostname()
	if host == "" {
		return false
	}

	// Check for localhost variations
	if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || host == "::1" {
		return false
	}

	// Check for metadata endpoints
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return false
	}

	// Parse IP and check for private ranges
	addr, err := netip.ParseAddr(host)
	if err == nil {
		// It's an IP address — check for private/loopback/link-local
		if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
			return false
		}
	}

	// Hostname that isn't an IP — allowed (will be resolved by DNS)
	return true
}

// --- HTML Text Extraction ---

var (
	scriptRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	multiSp  = regexp.MustCompile(`\s{2,}`)
)

// extractTextFromHTML strips HTML tags and returns readable text.
func extractTextFromHTML(html string) string {
	// Remove script and style blocks (with content)
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")

	// Convert block-level tags to newlines
	blockTags := []string{"p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6",
		"li", "tr", "hr", "section", "article", "header", "footer", "nav"}
	for _, tag := range blockTags {
		re := regexp.MustCompile(`(?i)</?` + tag + `[^>]*>`)
		text = re.ReplaceAllString(text, "\n")
	}

	// Strip all remaining HTML tags
	text = tagRe.ReplaceAllString(text, "")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Normalize whitespace
	text = multiSp.ReplaceAllString(text, " ")

	// Trim lines and remove excessive blank lines
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return strings.Join(result, "\n")
}

// parseSearXNGResponse parses the SearXNG JSON response and formats results.
func parseSearXNGResponse(body []byte, limit int) (string, bool) {
	type SearXResult struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	type SearXResponse struct {
		Results []SearXResult `json:"results"`
	}

	var resp SearXResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Sprintf("SearchWeb error: failed to parse response: %v", err), true
	}

	if len(resp.Results) == 0 {
		return "No results found.", false
	}

	// Format results
	var b strings.Builder
	count := min(len(resp.Results), limit)
	for i := 0; i < count; i++ {
		r := resp.Results[i]
		b.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, r.Title, r.URL))
		if r.Content != "" {
			// Truncate long snippets
			content := r.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			b.WriteString(fmt.Sprintf("   %s\n", content))
		}
	}

	return b.String(), false
}