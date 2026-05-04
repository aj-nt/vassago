// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Mock Browser Driver ---

// MockBrowserDriver implements BrowserDriver for testing.
type MockBrowserDriver struct {
	navigateCalled bool
	extractCalled  bool
	clickSelector  string
	scrollCalled   bool
	scrollDirection string
	scrollAmount   int
	typeCalled     bool
	typeSelector   string
	typeText       string
	typeSubmit     bool
	waitSelector   string
	snapshotCalled bool
	history        []string
	goBackCalled   bool
	navigateErr    error
	extractErr     error
	screenshotErr  error
	clickErr       error
	scrollErr      error
	typeErr        error
	waitErr        error
	snapshotErr    error
	goBackErr      error
	extractResult  string // custom extract text (defaults to "Concurrent content")
	snapshotResult string // custom snapshot result
	mu             sync.Mutex
}

func (m *MockBrowserDriver) Navigate(ctx context.Context, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.navigateCalled = true
	return m.navigateErr
}

func (m *MockBrowserDriver) ExtractText(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extractCalled = true
	text := m.extractResult
	if text == "" {
		text = "Concurrent content"
	}
	return text, m.extractErr
}

func (m *MockBrowserDriver) Screenshot(ctx context.Context, path string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.screenshotErr != nil {
		return "", m.screenshotErr
	}
	// Create a minimal file for testing
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("fake-png-data"), 0644)
	return path, nil
}

func (m *MockBrowserDriver) Click(ctx context.Context, selector string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clickSelector = selector
	return m.clickErr
}

func (m *MockBrowserDriver) Scroll(ctx context.Context, direction string, amount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scrollCalled = true
	m.scrollDirection = direction
	m.scrollAmount = amount
	return m.scrollErr
}

func (m *MockBrowserDriver) Type(ctx context.Context, selector, text string, submit bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typeCalled = true
	m.typeSelector = selector
	m.typeText = text
	m.typeSubmit = submit
	return m.typeErr
}

func (m *MockBrowserDriver) WaitForElement(ctx context.Context, selector string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waitSelector = selector
	return m.waitErr
}

func (m *MockBrowserDriver) Snapshot(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshotCalled = true
	if m.snapshotResult != "" {
		return m.snapshotResult, m.snapshotErr
	}
	return `{"title":"Test Page","url":"https://example.com","headings":[],"links":[],"forms":[],"scrollY":0}`, m.snapshotErr
}

func (m *MockBrowserDriver) GetHistory() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.history
}

func (m *MockBrowserDriver) GoBack(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.goBackCalled = true
	if len(m.history) > 0 {
		m.history = m.history[:len(m.history)-1]
	}
	return m.goBackErr
}

func (m *MockBrowserDriver) Close() error {
	return nil
}

// --- Helper to set up mock driver ---

func setupMockBrowser() *MockBrowserDriver {
	mock := &MockBrowserDriver{}
	browserDriver = mock
	browserReady = true
	browserConfig = BrowserConfig{
		Enabled:  true,
		Headless: true,
		CacheDir: filepath.Join(os.TempDir(), "mnemosyne-test-screenshots"),
	}
	return mock
}

func teardownMockBrowser() {
	browserDriver = nil
	browserReady = false
}

// --- QA Tests ---

func TestBrowse_Navigate(t *testing.T) {
	mock := setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "navigate",
		"url":    "https://example.com",
	})

	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	if !mock.navigateCalled {
		t.Error("Navigate was not called")
	}
	if !strings.Contains(result, "example.com") {
		t.Errorf("result should contain URL, got: %s", result)
	}
}

func TestBrowse_ExtractText(t *testing.T) {
	mock := setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "extract",
	})

	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	if !mock.extractCalled {
		t.Error("ExtractText was not called")
	}
	if !strings.Contains(result, "Concurrent content") {
		t.Errorf("result should contain page text, got: %s", result)
	}
}

func TestBrowse_Screenshot(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	tmpDir := t.TempDir()
	screenshotPath := filepath.Join(tmpDir, "test.png")

	result, isError := Browse(map[string]interface{}{
		"action":          "screenshot",
		"screenshot_path": screenshotPath,
	})

	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	if !strings.Contains(result, screenshotPath) {
		t.Errorf("result should contain path, got: %s", result)
	}

	// Verify file was created
	if _, err := os.Stat(screenshotPath); os.IsNotExist(err) {
		t.Error("screenshot file was not created")
	}
}

func TestBrowse_Click(t *testing.T) {
	mock := setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action":   "click",
		"selector": "#submit-button",
	})

	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	if mock.clickSelector != "#submit-button" {
		t.Errorf("wrong selector clicked: got %s, want #submit-button", mock.clickSelector)
	}
	if !strings.Contains(result, "#submit-button") {
		t.Errorf("result should mention selector, got: %s", result)
	}
}

func TestBrowse_MissingAction(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{})
	if !isError {
		t.Error("expected error for missing action")
	}
	if !strings.Contains(result, "action") {
		t.Errorf("error should mention 'action', got: %s", result)
	}
}

func TestBrowse_InvalidAction(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "delete",
	})
	if !isError {
		t.Error("expected error for invalid action")
	}
	if !strings.Contains(result, "Invalid action") {
		t.Errorf("error should mention Invalid action, got: %s", result)
	}
}

func TestBrowse_NavigateMissingURL(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "navigate",
	})
	if !isError {
		t.Error("expected error for missing URL")
	}
	if !strings.Contains(result, "url") {
		t.Errorf("error should mention 'url', got: %s", result)
	}
}

func TestBrowse_ClickMissingSelector(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "click",
	})
	if !isError {
		t.Error("expected error for missing selector")
	}
	if !strings.Contains(result, "selector") {
		t.Errorf("error should mention 'selector', got: %s", result)
	}
}

func TestBrowse_NavigateBlocksSSRF(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	ssrfURLs := []string{
		"http://localhost:8080/admin",
		"http://127.0.0.1/secrets",
		"http://169.254.169.254/metadata",
		"ftp://example.com",
		"file:///etc/passwd",
	}

	for _, rawURL := range ssrfURLs {
		result, isError := Browse(map[string]interface{}{
			"action": "navigate",
			"url":    rawURL,
		})
		if !isError {
			t.Errorf("expected SSRF block for URL %q, got: %s", rawURL, result)
		}
		if !strings.Contains(result, "blocked") && !strings.Contains(result, "not an allowed URL") {
			t.Errorf("error should mention URL block for %q, got: %s", rawURL, result)
		}
	}
}

func TestBrowse_NavigateDriverError(t *testing.T) {
	mock := setupMockBrowser()
	mock.navigateErr = errors.New("connection refused")
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "navigate",
		"url":    "https://example.com",
	})
	if !isError {
		t.Error("expected error on driver failure")
	}
	if !strings.Contains(result, "Navigation failed") {
		t.Errorf("error should mention Navigation failed, got: %s", result)
	}
}

func TestBrowse_ExtractDriverError(t *testing.T) {
	mock := setupMockBrowser()
	mock.extractErr = errors.New("DOM not ready")
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action": "extract",
	})
	if !isError {
		t.Error("expected error on driver failure")
	}
	if !strings.Contains(result, "Text extraction failed") {
		t.Errorf("error should mention extraction failure, got: %s", result)
	}
}

func TestBrowse_ClickDriverError(t *testing.T) {
	mock := setupMockBrowser()
	mock.clickErr = errors.New("element not found")
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action":   "click",
		"selector": "#nonexistent",
	})
	if !isError {
		t.Error("expected error on driver failure")
	}
	if !strings.Contains(result, "Click failed") {
		t.Errorf("error should mention Click failed, got: %s", result)
	}
}

func TestBrowse_ScreenshotDriverError(t *testing.T) {
	mock := setupMockBrowser()
	mock.screenshotErr = errors.New("rendering failed")
	defer teardownMockBrowser()

	result, isError := Browse(map[string]interface{}{
		"action":          "screenshot",
		"screenshot_path": filepath.Join(t.TempDir(), "out.png"),
	})
	if !isError {
		t.Error("expected error on driver failure")
	}
	if !strings.Contains(result, "Screenshot failed") {
		t.Errorf("error should mention Screenshot failed, got: %s", result)
	}
}

// --- Red Team Tests ---

func TestBrowse_RedTeam_DangerousSelector(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	dangerousSelectors := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"<script>alert(1)</script>",
		"img onerror=alert(1)",
		"img onload=alert(1)",
		"img onclick=alert(1)",
	}

	for _, selector := range dangerousSelectors {
		result, isError := Browse(map[string]interface{}{
			"action":   "click",
			"selector": selector,
		})
		if !isError {
			t.Errorf("expected block for dangerous selector %q, got: %s", selector, result)
		}
		if !strings.Contains(result, "blocked") && !strings.Contains(result, "dangerous") {
			t.Errorf("error should mention blocked/dangerous for %q, got: %s", selector, result)
		}
	}
}

func TestBrowse_RedTeam_ScreenshotPathTraversal(t *testing.T) {
	// Set up allowed dirs so path traversal is actually blocked
	originalAllowed := allowedDirs
	allowedDirs = []string{"/tmp"}
	defer func() { allowedDirs = originalAllowed }()

	setupMockBrowser()
	defer teardownMockBrowser()

	traversalPaths := []string{
		"../../../etc/passwd",
		"/etc/shadow",
	}

	for _, path := range traversalPaths {
		result, isError := Browse(map[string]interface{}{
			"action":          "screenshot",
			"screenshot_path": path,
		})
		if !isError {
			t.Errorf("expected error for path traversal %q, got: %s", path, result)
		}
	}
}

func TestBrowse_RedTeam_EmptyAndWhitespaceAction(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	// Empty action
	_, isError := Browse(map[string]interface{}{
		"action": "",
	})
	if !isError {
		t.Error("expected error for empty action")
	}

	// Whitespace-only action (strArg trims, so it becomes empty)
	_, isError = Browse(map[string]interface{}{
		"action": "   ",
	})
	if !isError {
		t.Error("expected error for whitespace action")
	}
}

func TestBrowse_RedTeam_ConcurrentBrowse(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func() {
			result, isError := Browse(map[string]interface{}{
				"action": "extract",
			})
			if isError {
				t.Errorf("concurrent extract failed: %s", result)
			}
			done <- true
		}()
	}

	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestBrowse_RedTeam_NilArgs(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	result, isError := Browse(nil)
	if !isError {
		t.Error("expected error for nil args")
	}
	if !strings.Contains(result, "action") {
		t.Errorf("error should mention 'action', got: %s", result)
	}
}

func TestBrowse_RedTeam_ScreenshotDefaultPath(t *testing.T) {
	setupMockBrowser()
	defer teardownMockBrowser()

	// Without specifying screenshot_path, should use cache dir
	result, isError := Browse(map[string]interface{}{
		"action": "screenshot",
	})
	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	if !strings.Contains(result, browserConfig.CacheDir) {
		t.Errorf("screenshot should be in cache dir, got: %s", result)
	}
}

func TestBrowse_RedTeam_ExtractTruncation(t *testing.T) {
	mock := setupMockBrowser()
	defer teardownMockBrowser()

	// Generate very long text
	longText := strings.Repeat("A", 50000)
	mock.extractResult = longText

	result, isError := Browse(map[string]interface{}{
		"action": "extract",
	})
	if isError {
		t.Fatalf("unexpected error: %s", result)
	}
	// Result should be truncated
	if len(result) > 20050 { // maxResultChars + some overhead
		t.Errorf("extract result should be truncated, got %d chars", len(result))
	}
}

// --- BrowserConfig Tests ---

func TestDefaultBrowserConfig(t *testing.T) {
	cfg := DefaultBrowserConfig()
	if cfg.Enabled {
		t.Error("default browser should be disabled")
	}
	if !cfg.Headless {
		t.Error("default browser should be headless")
	}
}

func TestRegisterBrowserTools(t *testing.T) {
	registry := NewToolRegistry()

	cfg := BrowserConfig{
		Enabled:  true,
		Headless: true,
		CacheDir: t.TempDir(),
	}
	RegisterBrowserTools(registry, cfg)

	// Verify Browse tool is registered
	_, err := registry.Get("Browse")
	if err != nil {
		t.Fatalf("Browse tool not registered: %v", err)
	}

	// Verify schema is registered
	schemas := registry.Schemas()
	if _, ok := schemas["Browse"]; !ok {
		t.Error("Browse schema not registered")
	}
}

// --- containsDangerousSelector Tests ---

func TestContainsDangerousSelector(t *testing.T) {
	tests := []struct {
		selector string
		want     bool
	}{
		{"#submit", false},
		{".button", false},
		{"a[href='/about']", false},
		{"javascript:alert(1)", true},
		{"data:text/html,<script>", true},
		{"<script>alert(1)</script>", true},
		{"img onerror=alert(1)", true},
		{"img onload=alert(1)", true},
		{"img onclick=alert(1)", true},
		{"#JAVASCRIPT:foo", true}, // case-insensitive
	}

	for _, tt := range tests {
		got := containsDangerousSelector(tt.selector)
		if got != tt.want {
			t.Errorf("containsDangerousSelector(%q) = %v, want %v", tt.selector, got, tt.want)
		}
	}
}

// --- CleanupBrowser Tests ---

func TestCleanupBrowser(t *testing.T) {
	setupMockBrowser()
	CleanupBrowser()
	if browserDriver != nil {
		t.Error("browser driver should be nil after cleanup")
	}
	if browserReady {
		t.Error("browser should not be ready after cleanup")
	}
}

// --- ScreenshotToBase64 Tests ---

func TestScreenshotToBase64(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.png")
	content := "fake-png-data"
	os.WriteFile(tmpFile, []byte(content), 0644)

	encoded, err := ScreenshotToBase64(tmpFile)
	if err != nil {
		t.Fatalf("ScreenshotToBase64 failed: %v", err)
	}
	if len(encoded) == 0 {
		t.Error("base64 encoding should not be empty")
	}
}

func TestScreenshotToBase64_NonexistentFile(t *testing.T) {
	_, err := ScreenshotToBase64("/nonexistent/path.png")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}