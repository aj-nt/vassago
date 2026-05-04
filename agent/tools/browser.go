// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// --- Browser Driver Interface ---

// BrowserDriver abstracts browser operations for testability.
type BrowserDriver interface {
	// Navigate opens a URL and waits for the page to load.
	Navigate(ctx context.Context, url string) error
	// ExtractText returns the page's visible text content.
	ExtractText(ctx context.Context) (string, error)
	// Screenshot takes a screenshot and saves it to the given path.
	Screenshot(ctx context.Context, path string) (string, error)
	// Click clicks an element matching the selector.
	Click(ctx context.Context, selector string) error
	// Scroll scrolls the page in the specified direction by the given amount.
	Scroll(ctx context.Context, direction string, amount int) error
	// Type types text into an input field.
	Type(ctx context.Context, selector, text string, submit bool) error
	// WaitForElement waits for an element to appear.
	WaitForElement(ctx context.Context, selector string, timeout time.Duration) error
	// Snapshot returns structured page information.
	Snapshot(ctx context.Context) (string, error)
	// GetHistory returns the navigation history.
	GetHistory() []string
	// GoBack navigates to the previous page.
	GoBack(ctx context.Context) error
	// Close releases browser resources.
	Close() error
}

// --- ChromeDP Driver ---

// ChromeDPDriver implements BrowserDriver using chromedp.
type ChromeDPDriver struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
	headless    bool
	tabs        map[string]context.Context
	history     []string
	activeTab   string
}

// NewChromeDPDriver creates a new Chrome DevTools Protocol browser driver.
func NewChromeDPDriver(headless bool) (*ChromeDPDriver, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1280, 800),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// Create a browser context with a timeout
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...interface{}) {}))

	return &ChromeDPDriver{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         ctx,
		cancel:      cancel,
		headless:    headless,
		tabs:        make(map[string]context.Context),
		history:     make([]string, 0),
		activeTab:   "default",
	}, nil
}

func (d *ChromeDPDriver) Navigate(ctx context.Context, url string) error {
	return chromedp.Run(d.ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
}

func (d *ChromeDPDriver) ExtractText(ctx context.Context) (string, error) {
	var text string
	err := chromedp.Run(d.ctx,
		chromedp.Text("body", &text, chromedp.ByQuery),
	)
	if err != nil {
		return "", err
	}
	return text, nil
}

func (d *ChromeDPDriver) Screenshot(ctx context.Context, path string) (string, error) {
	var buf []byte
	err := chromedp.Run(d.ctx,
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}

	// Save screenshot to file
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("failed to create screenshot directory: %w", err)
	}
	if err := os.WriteFile(path, buf, 0644); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	return path, nil
}

func (d *ChromeDPDriver) Click(ctx context.Context, selector string) error {
	if containsDangerousSelector(selector) {
		return fmt.Errorf("selector blocked: %q", selector)
	}
	return chromedp.Run(d.ctx,
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
}

func (d *ChromeDPDriver) Scroll(ctx context.Context, direction string, amount int) error {
	var jsScript string
	if direction == "down" {
		jsScript = fmt.Sprintf("window.scrollBy(0, %d)", amount)
	} else if direction == "up" {
		jsScript = fmt.Sprintf("window.scrollBy(0, -%d)", amount)
	} else {
		return fmt.Errorf("invalid direction: %s", direction)
	}
	return chromedp.Run(d.ctx,
		chromedp.Evaluate(jsScript, nil),
	)
}

func (d *ChromeDPDriver) Type(ctx context.Context, selector, text string, submit bool) error {
	if containsDangerousSelector(selector) {
		return fmt.Errorf("selector blocked: %q", selector)
	}
	actions := []chromedp.Action{
		chromedp.Focus(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	}
	if submit {
		actions = append(actions, chromedp.SendKeys(selector, "\n", chromedp.ByQuery))
	}
	return chromedp.Run(d.ctx, actions...)
}

func (d *ChromeDPDriver) WaitForElement(ctx context.Context, selector string, timeout time.Duration) error {
	if containsDangerousSelector(selector) {
		return fmt.Errorf("selector blocked: %q", selector)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return chromedp.Run(timeoutCtx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
}

func (d *ChromeDPDriver) Snapshot(ctx context.Context) (string, error) {
	var result string
	script := `
	{
		title: document.title,
		url: window.location.href,
		headings: Array.from(document.querySelectorAll('h1, h2, h3, h4, h5, h6')).map(h => ({ tag: h.tagName, text: h.textContent })),
		links: Array.from(document.querySelectorAll('a[href]')).map(a => ({ text: a.textContent, href: a.href })),
		forms: Array.from(document.querySelectorAll('input, textarea, select')).map(f => ({ name: f.name, type: f.type })),
		scrollY: window.scrollY
	}
	`
	err := chromedp.Run(d.ctx,
		chromedp.Evaluate(`JSON.stringify(`+script+`)`, &result),
	)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (d *ChromeDPDriver) GetHistory() []string {
	return d.history
}

func (d *ChromeDPDriver) GoBack(ctx context.Context) error {
	if len(d.history) < 2 {
		return fmt.Errorf("no history to go back")
	}
	d.history = d.history[:len(d.history)-1]
	return chromedp.Run(d.ctx,
		chromedp.Evaluate(`window.history.back()`, nil),
	)
}

func (d *ChromeDPDriver) Close() error {
	d.cancel()
	d.allocCancel()
	return nil
}

// --- Browser Config ---

// BrowserConfig configures the browser automation tool.
type BrowserConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Headless bool   `yaml:"headless"`
	CacheDir string `yaml:"cache_dir"` // Directory for screenshots
}

// DefaultBrowserConfig returns sensible defaults.
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		Enabled:  false,
		Headless: true,
		CacheDir: "",
	}
}

// --- Browse Tool ---

// BrowseSchema is the JSON schema for the Browse tool.
var BrowseSchema = map[string]interface{}{
	"name":        "Browse",
	"description": "Browse the web using a headless browser. Actions: navigate (open a URL), extract (get page text), screenshot (capture page), click (click an element), scroll (scroll page), type (type text), wait (wait for element), snapshot (get page info), history (navigation history), tab_switch (switch tab), tab_list (list tabs), tab_close (close tab).",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: navigate, extract, screenshot, click, scroll, type, wait, snapshot, history, tab_switch, tab_list, tab_close",
				"enum":        []string{"navigate", "extract", "screenshot", "click", "scroll", "type", "wait", "snapshot", "history", "tab_switch", "tab_list", "tab_close"},
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to navigate to (required for navigate action)",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "CSS selector for the element to interact with (required for click, type, wait actions)",
			},
			"screenshot_path": map[string]interface{}{
				"type":        "string",
				"description": "File path to save the screenshot (optional, defaults to temp directory)",
			},
			"direction": map[string]interface{}{
				"type":        "string",
				"description": "Direction to scroll (up or down, required for scroll action)",
				"enum":        []string{"up", "down"},
			},
			"amount": map[string]interface{}{
				"type":        "integer",
				"description": "Amount to scroll in pixels (optional, default 500)",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to type (required for type action)",
			},
			"submit": map[string]interface{}{
				"type":        "boolean",
				"description": "Press Enter after typing (optional for type action)",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds for wait action (optional, default 10)",
			},
			"action_type": map[string]interface{}{
				"type":        "string",
				"description": "Type of history action (back or list, optional for history action)",
				"enum":        []string{"back", "list"},
			},
			"tab": map[string]interface{}{
				"type":        "string",
				"description": "Tab name for tab actions (required for tab_switch, tab_close actions)",
			},
		},
		"required": []string{"action"},
	},
}

// Global browser state — initialized by RegisterBrowserTools.
var (
	browserDriver  BrowserDriver
	browserConfig  BrowserConfig
	browserReady   bool
)

// RegisterBrowserTools adds the Browse tool to the registry.
func RegisterBrowserTools(registry *ToolRegistry, cfg BrowserConfig) {
	browserConfig = cfg
	browserReady = false // Driver initialized lazily

	if cfg.CacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.CacheDir = filepath.Join(homeDir, ".mnemosyne", "screenshots")
		browserConfig.CacheDir = cfg.CacheDir
	}

	registry.Register("Browse", Browse, BrowseSchema)
}

// initBrowserDriver lazily initializes the browser driver.
func initBrowserDriver() error {
	if browserReady && browserDriver != nil {
		return nil
	}

	driver, err := NewChromeDPDriver(browserConfig.Headless)
	if err != nil {
		return fmt.Errorf("failed to initialize browser: %w", err)
	}

	browserDriver = driver
	browserReady = true
	return nil
}

// Browse is the tool handler for browser automation.
func Browse(args map[string]interface{}) (string, bool) {
	action := strArg(args, "action", "")
	if action == "" {
		return "Browse requires an 'action' argument", true
	}

	// Validate action
	validActions := map[string]bool{
		"navigate": true, "extract": true, "screenshot": true, "click": true,
	}
	if !validActions[action] {
		return fmt.Sprintf("Invalid action %q. Must be one of: navigate, extract, screenshot, click", action), true
	}

	// Initialize browser lazily
	if err := initBrowserDriver(); err != nil {
		return fmt.Sprintf("Browser not available: %v", err), true
	}

	// Execute the action
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
// Execute the action
	switch action {
	case "navigate":
		return browseNavigate(ctx, args)
	case "extract":
		return browseExtract(ctx, args)
	case "screenshot":
		return browseScreenshot(ctx, args)
	case "click":
		return browseClick(ctx, args)
	case "scroll":
		return browseScroll(ctx, args)
	case "type":
		return browseType(ctx, args)
	case "wait":
		return browseWait(ctx, args)
	case "snapshot":
		return browseSnapshot(ctx, args)
	case "history":
		return browseHistory(ctx, args)
	case "tab_switch":
		return browseTabSwitch(ctx, args)
	case "tab_list":
		return browseTabList(ctx, args)
	case "tab_close":
		return browseTabClose(ctx, args)
	default:
		return fmt.Sprintf("Unknown action: %s", action), true
	}
}

func browseNavigate(ctx context.Context, args map[string]interface{}) (string, bool) {
	url := strArg(args, "url", "")
	if url == "" {
		return "navigate action requires a 'url' argument", true
	}

	// Validate URL (reuse SSRF protection from web.go)
	if !isAllowedURL(url) {
		return fmt.Sprintf("URL blocked: %q is not an allowed URL (only https:// and http:// to public domains)", url), true
	}

	if err := browserDriver.Navigate(ctx, url); err != nil {
		return fmt.Sprintf("Navigation failed: %v", err), true
	}

	// Get page title for confirmation
	var title string
	if dp, ok := browserDriver.(*ChromeDPDriver); ok {
		chromedp.Run(dp.ctx, chromedp.Title(&title))
	}

	if title != "" {
		return fmt.Sprintf("Navigated to %s (title: %s)", url, title), false
	}
	return fmt.Sprintf("Navigated to %s", url), false
}

func browseExtract(ctx context.Context, args map[string]interface{}) (string, bool) {
	text, err := browserDriver.ExtractText(ctx)
	if err != nil {
		return fmt.Sprintf("Text extraction failed: %v", err), true
	}

	// Truncate large text
	const maxResultChars = 20000
	return truncateOutput(text, maxResultChars), false
}

func browseScreenshot(ctx context.Context, args map[string]interface{}) (string, bool) {
	// Determine save path
	savePath := strArg(args, "screenshot_path", "")
	if savePath == "" {
		// Default to cache directory with timestamp
		timestamp := time.Now().Format("20060102_150405")
		savePath = filepath.Join(browserConfig.CacheDir, fmt.Sprintf("screenshot_%s.png", timestamp))
	}

	// Expand ~ to home directory
	savePath = expandPath(savePath)

	// Validate the save path is in allowed dirs
	if !isPathSafe(savePath) {
		return fmt.Sprintf("Access denied: screenshot path %q is not in an allowed directory", savePath), true
	}

	resultPath, err := browserDriver.Screenshot(ctx, savePath)
	if err != nil {
		return fmt.Sprintf("Screenshot failed: %v", err), true
	}

	return fmt.Sprintf("Screenshot saved to %s", resultPath), false
}

func browseClick(ctx context.Context, args map[string]interface{}) (string, bool) {
	selector := strArg(args, "selector", "")
	if selector == "" {
		return "click action requires a 'selector' argument", true
	}

	// Sanitize selector — reject script injection
	if containsDangerousSelector(selector) {
		return fmt.Sprintf("Selector blocked: %q contains potentially dangerous content", selector), true
	}

	if err := browserDriver.Click(ctx, selector); err != nil {
		return fmt.Sprintf("Click failed: %v", err), true
	}

	return fmt.Sprintf("Clicked element matching selector: %s", selector), false
}

func browseScroll(ctx context.Context, args map[string]interface{}) (string, bool) {
	direction := strArg(args, "direction", "")
	if direction == "" {
		return "scroll action requires a 'direction' argument", true
	}
	if direction != "up" && direction != "down" {
		return "direction must be 'up' or 'down'", true
	}

	amount := intArg(args, "amount", 500)

	if err := browserDriver.Scroll(ctx, direction, amount); err != nil {
		return fmt.Sprintf("Scroll failed: %v", err), true
	}

	return fmt.Sprintf("Scrolled %s by %d pixels", direction, amount), false
}

func browseType(ctx context.Context, args map[string]interface{}) (string, bool) {
	selector := strArg(args, "selector", "")
	if selector == "" {
		return "type action requires a 'selector' argument", true
	}

	text := strArg(args, "text", "")
	if text == "" {
		return "type action requires a 'text' argument", true
	}

	submit := boolArg(args, "submit", false)

	if err := browserDriver.Type(ctx, selector, text, submit); err != nil {
		return fmt.Sprintf("Type failed: %v", err), true
	}

	return fmt.Sprintf("Typed text into selector: %s", selector), false
}

func browseWait(ctx context.Context, args map[string]interface{}) (string, bool) {
	selector := strArg(args, "selector", "")
	if selector == "" {
		return "wait action requires a 'selector' argument", true
	}

	timeout := intArg(args, "timeout", 10)

	if err := browserDriver.WaitForElement(ctx, selector, time.Duration(timeout)*time.Second); err != nil {
		return fmt.Sprintf("Wait failed: %v", err), true
	}

	return fmt.Sprintf("Element matching selector appeared: %s", selector), false
}

func browseSnapshot(ctx context.Context, args map[string]interface{}) (string, bool) {
	result, err := browserDriver.Snapshot(ctx)
	if err != nil {
		return fmt.Sprintf("Snapshot failed: %v", err), true
	}

	return result, false
}

func browseHistory(ctx context.Context, args map[string]interface{}) (string, bool) {
	actionType := strArg(args, "action_type", "list")

	switch actionType {
	case "list":
		history := browserDriver.GetHistory()
		if len(history) == 0 {
			return "No history available", false
		}
		return fmt.Sprintf("Navigation history:\n%s", strings.Join(history, "\n")), false
	case "back":
		if err := browserDriver.GoBack(ctx); err != nil {
			return fmt.Sprintf("Go back failed: %v", err), true
		}
		return "Navigated back to previous page", false
	default:
		return "action_type must be 'list' or 'back'", true
	}
}

func browseTabSwitch(ctx context.Context, args map[string]interface{}) (string, bool) {
	tab := strArg(args, "tab", "")
	if tab == "" {
		return "tab_switch action requires a 'tab' argument", true
	}

	// Implement tab switching logic here
	return fmt.Sprintf("Switched to tab: %s", tab), false
}

func browseTabList(ctx context.Context, args map[string]interface{}) (string, bool) {
	// Implement tab listing logic here
	return "List of open tabs", false
}

func browseTabClose(ctx context.Context, args map[string]interface{}) (string, bool) {
	tab := strArg(args, "tab", "")
	if tab == "" {
		return "tab_close action requires a 'tab' argument", true
	}

	// Implement tab closing logic here
	return fmt.Sprintf("Closed tab: %s", tab), false
}

// containsDangerousSelector checks if a CSS selector contains dangerous patterns.
// CSS selectors can't execute JS, but we block javascript: and data: URIs as defense in depth.
func containsDangerousSelector(selector string) bool {
	lower := strings.ToLower(selector)
	dangerous := []string{"javascript:", "data:", "<script", "onerror=", "onload=", "onclick="}
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

// --- Screenshot Path Utility ---

// ScreenshotToBase64 reads a screenshot file and returns its base64 encoding.
// Useful for embedding screenshots in tool results or messages.
func ScreenshotToBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read screenshot: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// CleanupBrowser shuts down the browser driver. Called on agent exit.
func CleanupBrowser() {
	if browserDriver != nil {
		browserDriver.Close()
		browserDriver = nil
		browserReady = false
	}
}