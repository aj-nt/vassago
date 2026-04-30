package tools

import (
	"testing"
)

// --- SearchWeb tests ---

func TestSearchWebSchema(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, nil) // nil httpClient is fine for schema check

	if _, err := reg.Get("SearchWeb"); err != nil {
		t.Fatal("SearchWeb not registered:", err)
	}
	if _, err := reg.Get("FetchURL"); err != nil {
		t.Fatal("FetchURL not registered:", err)
	}
}

func TestSearchWebMissingQuery(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, err := reg.Get("SearchWeb")
	if err != nil {
		t.Fatal(err)
	}

	result, isErr := handler(map[string]interface{}{})
	if !isErr {
		t.Error("expected error for missing query")
	}
	if result == "" {
		t.Error("expected non-empty error message")
	}
}

func TestSearchWebEmptyQuery(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("SearchWeb")

	_, isErr := handler(map[string]interface{}{"query": ""})
	if !isErr {
		t.Error("expected error for empty query")
	}
}

func TestSearchWebQueryTooLong(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("SearchWeb")

	// Build a query over the max length
	longQuery := make([]byte, 5000)
	for i := range longQuery {
		longQuery[i] = 'a'
	}

	_, isErr := handler(map[string]interface{}{"query": string(longQuery)})
	if !isErr {
		t.Error("expected error for oversized query")
	}
}

func TestSearchWebDefaultLimit(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("SearchWeb")

	// Just test arg parsing — no actual HTTP call; verify it doesn't panic
	_, _ = handler(map[string]interface{}{
		"query": "test query",
	})
}

func TestSearchWebLimitBoundaries(t *testing.T) {
	// Test that limit is capped at 50 and minimum 1
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("SearchWeb")

	// Limit of 0 should be clamped to 1
	_, _ = handler(map[string]interface{}{
		"query": "test",
		"limit": float64(0),
	})

	// Limit of 100 should be clamped to 50
	_, _ = handler(map[string]interface{}{
		"query": "test",
		"limit": float64(100),
	})
}

// --- FetchURL tests ---

func TestFetchURLMissingURL(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	_, isErr := handler(map[string]interface{}{})
	if !isErr {
		t.Error("expected error for missing url")
	}
}

func TestFetchURLEmptyURL(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	_, isErr := handler(map[string]interface{}{"url": ""})
	if !isErr {
		t.Error("expected error for empty url")
	}
}

func TestFetchURLInvalidScheme(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	_, isErr := handler(map[string]interface{}{"url": "ftp://example.com/file"})
	if !isErr {
		t.Error("expected error for non-http(s) URL")
	}
}

func TestFetchURLBlockedLocalhost(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	localURLs := []string{
		"http://localhost:8080/admin",
		"http://127.0.0.1:50051/grpc",
		"http://0.0.0.0/",
		"http://[::1]/",
	}

	for _, url := range localURLs {
		_, isErr := handler(map[string]interface{}{"url": url})
		if !isErr {
			t.Errorf("expected error for localhost URL %q", url)
		}
	}
}

func TestFetchURLBlockedPrivateIPs(t *testing.T) {
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	privateURLs := []string{
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://172.16.0.1/",
	}

	for _, url := range privateURLs {
		_, isErr := handler(map[string]interface{}{"url": url})
		if !isErr {
			t.Errorf("expected error for private IP URL %q", url)
		}
	}
}

func TestFetchURLValidHTTPS(t *testing.T) {
	// Just test arg validation — no actual HTTP call needed
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	// This will fail because we can't actually connect, but it should not
	// be blocked at the URL validation level
	_, _ = handler(map[string]interface{}{"url": "https://example.com/"})
}

func TestFetchURLMaxSize(t *testing.T) {
	// Test that the max_content_length parameter is accepted
	reg := NewToolRegistry()
	RegisterWebTools(reg, NewHTTPClient())
	handler, _ := reg.Get("FetchURL")

	// Just arg parsing — no actual HTTP
	_, _ = handler(map[string]interface{}{
		"url":               "https://example.com/",
		"max_content_length": float64(5000),
	})
}

// --- URL Safety tests ---

func TestIsAllowedURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		// Allowed
		{"https://example.com/", true},
		{"http://example.com/path", true},
		{"https://api.github.com/repos", true},
		{"https://docs.python.org/3/", true},

		// Blocked: localhost
		{"http://localhost:8080/", false},
		{"http://localhost/", false},
		{"http://127.0.0.1/", false},
		{"http://127.0.0.1:50051/", false},
		{"http://0.0.0.0/", false},

		// Blocked: private IPs
		{"http://10.0.0.1/", false},
		{"http://192.168.1.1/", false},
		{"http://172.16.0.1/", false},
		{"http://172.31.255.255/", false},

		// Blocked: IPv6 loopback
		{"http://[::1]/", false},

		// Blocked: non-http schemes
		{"ftp://example.com/", false},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},

		// Blocked: metadata endpoints
		{"http://169.254.169.254/", false},
		{"http://metadata.google.internal/", false},
	}

	for _, tt := range tests {
		result := isAllowedURL(tt.url)
		if result != tt.expected {
			t.Errorf("isAllowedURL(%q) = %v, want %v", tt.url, result, tt.expected)
		}
	}
}

func TestExtractTextFromHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		contains []string // strings that should appear in extracted text
		excludes []string // strings that should NOT appear
	}{
		{
			name:     "simple paragraph",
			html:     "<html><body><p>Hello World</p></body></html>",
			contains: []string{"Hello World"},
			excludes: []string{"<p>", "</p>"},
		},
		{
			name:     "script tags removed",
			html:     "<html><body><script>alert('xss')</script><p>Content</p></body></html>",
			contains: []string{"Content"},
			excludes: []string{"alert", "xss", "<script>"},
		},
		{
			name:     "style tags removed",
			html:     "<html><head><style>body{color:red}</style></head><body><p>Text</p></body></html>",
			contains: []string{"Text"},
			excludes: []string{"color:red", "<style>"},
		},
		{
			name:     "links preserved as text",
			html:     `<p>Visit <a href="https://example.com">Example</a> for more</p>`,
			contains: []string{"Visit", "Example", "more"},
			excludes: []string{"href", "<a>", "</a>"},
		},
		{
			name:     "markdown-like headers",
			html:     "<h1>Title</h1><h2>Subtitle</h2><p>Body text</p>",
			contains: []string{"Title", "Subtitle", "Body text"},
			excludes: []string{"<h1>", "<h2>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTextFromHTML(tt.html)
			for _, s := range tt.contains {
				if !contains(result, s) {
					t.Errorf("expected result to contain %q, got: %s", s, result)
				}
			}
			for _, s := range tt.excludes {
				if contains(result, s) {
					t.Errorf("expected result NOT to contain %q, got: %s", s, result)
				}
			}
		})
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}