// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aj-nt/mnemosyne/agent/provider"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// --- Mock Image Analyzer ---

type MockImageAnalyzer struct {
	result string
	err    error
}

func (m *MockImageAnalyzer) AnalyzeImage(ctx context.Context, imagePath, question, detail string) (string, error) {
	return m.result, m.err
}

// --- Mock Image Generator ---

type MockImageGenerator struct {
	paths []string
	err   error
}

func (m *MockImageGenerator) GenerateImage(ctx context.Context, prompt, size string, n int) ([]string, error) {
	return m.paths, m.err
}

// --- Mock Provider for vision tests ---

type MockVisionProvider struct {
	supportsVision bool
	result         string
	err            error
}

func (m *MockVisionProvider) Name() string                                                  { return "mock-vision" }
func (m *MockVisionProvider) EndpointURL() string                                            { return "" }
func (m *MockVisionProvider) Capabilities() agentctx.ProviderCapabilities {
	return agentctx.ProviderCapabilities{
		SupportsVision:        m.supportsVision,
		SupportsStreaming:      true,
		SupportsFunctionCalling: true,
		MaxContextTokens:      128000,
	}
}
func (m *MockVisionProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	return "mock_request", nil
}
func (m *MockVisionProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	if m.err != nil {
		return agentctx.ProviderResult{}, m.err
	}
	return agentctx.ProviderResult{RawResponse: m.result}, nil
}
func (m *MockVisionProvider) Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	text, _ := result.RawResponse.(string)
	return agentctx.ParsedResponse{Text: text}, nil
}
func (m *MockVisionProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	return nil, errors.New("streaming not supported in mock")
}

// --- AnalyzeImage Tool Tests ---

func TestAnalyzeImage_MockAnalyzer(t *testing.T) {
	mockAnalyzer := &MockImageAnalyzer{
		result: "This image shows a sunset over the ocean",
		err:    nil,
	}

	originalAnalyzer := imageAnalyzer
	imageAnalyzer = mockAnalyzer
	defer func() { imageAnalyzer = originalAnalyzer }()

	t.Run("successful analysis", func(t *testing.T) {
		result, isError := AnalyzeImage(map[string]interface{}{
			"image_path": "/tmp/test.jpg",
			"question":   "What is in this image?",
			"detail":     "low",
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "sunset") {
			t.Errorf("expected sunset in result, got: %s", result)
		}
	})

	t.Run("missing image_path", func(t *testing.T) {
		result, isError := AnalyzeImage(map[string]interface{}{
			"question": "What is in this image?",
		})
		if !isError {
			t.Fatal("expected error for missing image_path")
		}
		if !strings.Contains(result, "image_path is required") {
			t.Errorf("expected 'image_path is required', got: %s", result)
		}
	})

	t.Run("missing question", func(t *testing.T) {
		result, isError := AnalyzeImage(map[string]interface{}{
			"image_path": "/tmp/test.jpg",
		})
		if !isError {
			t.Fatal("expected error for missing question")
		}
		if !strings.Contains(result, "question is required") {
			t.Errorf("expected 'question is required', got: %s", result)
		}
	})

	t.Run("invalid detail level", func(t *testing.T) {
		result, isError := AnalyzeImage(map[string]interface{}{
			"image_path": "/tmp/test.jpg",
			"question":   "What is this?",
			"detail":     "medium",
		})
		if !isError {
			t.Fatal("expected error for invalid detail")
		}
		if !strings.Contains(result, "detail must be either 'low' or 'high'") {
			t.Errorf("expected 'detail must be' error, got: %s", result)
		}
	})

	t.Run("analyzer error", func(t *testing.T) {
		errAnalyzer := &MockImageAnalyzer{
			err: errors.New("vision not available"),
		}
		orig := imageAnalyzer
		imageAnalyzer = errAnalyzer
		defer func() { imageAnalyzer = orig }()

		result, isError := AnalyzeImage(map[string]interface{}{
			"image_path": "/tmp/test.jpg",
			"question":   "What is this?",
		})
		if !isError {
			t.Fatal("expected error from analyzer")
		}
		if !strings.Contains(result, "Image analysis failed") {
			t.Errorf("expected 'Image analysis failed', got: %s", result)
		}
	})

	t.Run("vision not enabled", func(t *testing.T) {
		orig := imageAnalyzer
		imageAnalyzer = nil
		defer func() { imageAnalyzer = orig }()

		result, isError := AnalyzeImage(map[string]interface{}{
			"image_path": "/tmp/test.jpg",
			"question":   "What is this?",
		})
		if !isError {
			t.Fatal("expected error for nil analyzer")
		}
		if !strings.Contains(result, "not enabled") {
			t.Errorf("expected 'not enabled', got: %s", result)
		}
	})
}

// --- GenerateImage Tool Tests ---

func TestGenerateImage_MockGenerator(t *testing.T) {
	mockGenerator := &MockImageGenerator{
		paths: []string{"/tmp/generated_123.png"},
		err:   nil,
	}

	originalGenerator := imageGenerator
	imageGenerator = mockGenerator
	defer func() { imageGenerator = originalGenerator }()

	t.Run("successful generation", func(t *testing.T) {
		result, isError := GenerateImage(map[string]interface{}{
			"prompt": "A beautiful landscape",
			"size":   "1024x1024",
			"n":      1,
		})
		if isError {
			t.Fatalf("unexpected error: %s", result)
		}
		if !strings.Contains(result, "/tmp/generated_123.png") {
			t.Errorf("expected path in result, got: %s", result)
		}
	})

	t.Run("missing prompt", func(t *testing.T) {
		result, isError := GenerateImage(map[string]interface{}{
			"size": "1024x1024",
		})
		if !isError {
			t.Fatal("expected error for missing prompt")
		}
		if !strings.Contains(result, "prompt is required") {
			t.Errorf("expected 'prompt is required', got: %s", result)
		}
	})

	t.Run("generator not enabled", func(t *testing.T) {
		orig := imageGenerator
		imageGenerator = nil
		defer func() { imageGenerator = orig }()

		result, isError := GenerateImage(map[string]interface{}{
			"prompt": "A landscape",
		})
		if !isError {
			t.Fatal("expected error for nil generator")
		}
		if !strings.Contains(result, "not enabled") {
			t.Errorf("expected 'not enabled', got: %s", result)
		}
	})

	t.Run("generation error", func(t *testing.T) {
		errGenerator := &MockImageGenerator{
			err: errors.New("API rate limit exceeded"),
		}
		orig := imageGenerator
		imageGenerator = errGenerator
		defer func() { imageGenerator = orig }()

		result, isError := GenerateImage(map[string]interface{}{
			"prompt": "A landscape",
		})
		if !isError {
			t.Fatal("expected error from generator")
		}
		if !strings.Contains(result, "Image generation failed") {
			t.Errorf("expected 'Image generation failed', got: %s", result)
		}
	})
}

// --- Red Team Tests ---

func TestAnalyzeImage_RedTeam(t *testing.T) {
	mockAnalyzer := &MockImageAnalyzer{
		result: "Analysis result",
		err:    nil,
	}
	originalAnalyzer := imageAnalyzer
	imageAnalyzer = mockAnalyzer
	defer func() { imageAnalyzer = originalAnalyzer }()

	t.Run("path traversal in image_path", func(t *testing.T) {
		// The analyzer itself doesn't check paths (that's done by ProviderImageAnalyzer)
		// But the tool handler accepts any path since the analyzer handles validation
		// This test verifies the analyzer interface works
		result, _ := AnalyzeImage(map[string]interface{}{
			"image_path": "/etc/passwd",
			"question":   "What is this?",
		})
		_ = result
	})

	t.Run("empty arguments", func(t *testing.T) {
		result, isError := AnalyzeImage(map[string]interface{}{})
		if !isError {
			t.Fatal("expected error for empty args")
		}
		if !strings.Contains(result, "image_path is required") {
			t.Errorf("expected image_path required, got: %s", result)
		}
	})

	t.Run("nil arguments", func(t *testing.T) {
		result, isError := AnalyzeImage(nil)
		if !isError {
			t.Fatal("expected error for nil args")
		}
		if !strings.Contains(result, "image_path is required") {
			t.Errorf("expected image_path required, got: %s", result)
		}
	})

	t.Run("very long question", func(t *testing.T) {
		longQuestion := strings.Repeat("a", 50000)
		result, isError := AnalyzeImage(map[string]interface{}{
			"image_path": "/tmp/test.jpg",
			"question":   longQuestion,
		})
		// Should succeed (mock doesn't limit question length)
		// but result should be truncated
		if isError {
			t.Errorf("unexpected error: %s", result)
		}
	})
}

func TestGenerateImage_RedTeam(t *testing.T) {
	mockGenerator := &MockImageGenerator{
		paths: []string{"/tmp/gen.png"},
		err:   nil,
	}
	originalGenerator := imageGenerator
	imageGenerator = mockGenerator
	defer func() { imageGenerator = originalGenerator }()

	t.Run("empty arguments", func(t *testing.T) {
		result, isError := GenerateImage(map[string]interface{}{})
		if !isError {
			t.Fatal("expected error for empty args")
		}
		if !strings.Contains(result, "prompt is required") {
			t.Errorf("expected prompt required, got: %s", result)
		}
	})

	t.Run("nil arguments", func(t *testing.T) {
		result, isError := GenerateImage(nil)
		if !isError {
			t.Fatal("expected error for nil args")
		}
		if !strings.Contains(result, "prompt is required") {
			t.Errorf("expected prompt required, got: %s", result)
		}
	})
}

// --- ProviderImageAnalyzer Tests ---

func TestProviderImageAnalyzer(t *testing.T) {
	// Create a temporary test image file
	tempFile, err := os.CreateTemp("", "test*.jpg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Write([]byte("dummy image data"))
	tempFile.Close()

	t.Run("provider without vision support", func(t *testing.T) {
		mockProv := &MockVisionProvider{supportsVision: false}
		registry := provider.NewProviderRegistry()
		registry.Register("mock", mockProv)
		registry.SetPrimary("mock")

		config := VisionConfig{
			Enabled:      true,
			MaxImageSize: 20 * 1024 * 1024,
		}
		analyzer := NewProviderImageAnalyzer(registry, config)

		_, err := analyzer.AnalyzeImage(context.Background(), tempFile.Name(), "What is this?", "low")
		if err == nil {
			t.Fatal("expected error for provider without vision support")
		}
		if !strings.Contains(err.Error(), "does not support vision") {
			t.Errorf("expected 'does not support vision' error, got: %v", err)
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		mockProv := &MockVisionProvider{supportsVision: true}
		registry := provider.NewProviderRegistry()
		registry.Register("mock", mockProv)
		registry.SetPrimary("mock")

		// Set allowed dirs to enforce path safety
		originalAllowed := allowedDirs
		allowedDirs = []string{"/tmp"}
		defer func() { allowedDirs = originalAllowed }()

		config := VisionConfig{
			Enabled:      true,
			MaxImageSize: 20 * 1024 * 1024,
		}
		analyzer := NewProviderImageAnalyzer(registry, config)

		_, err := analyzer.AnalyzeImage(context.Background(), "/etc/shadow", "What is this?", "low")
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
		if !strings.Contains(err.Error(), "access denied") {
			t.Errorf("expected 'access denied' error, got: %v", err)
		}
	})

	t.Run("invalid image type", func(t *testing.T) {
		// Create a temp file with wrong extension
		badFile, err := os.CreateTemp("", "test*.exe")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(badFile.Name())
		badFile.Write([]byte("not an image"))
		badFile.Close()

		mockProv := &MockVisionProvider{supportsVision: true}
		registry := provider.NewProviderRegistry()
		registry.Register("mock", mockProv)
		registry.SetPrimary("mock")

		config := VisionConfig{
			Enabled:      true,
			MaxImageSize: 20 * 1024 * 1024,
		}
		analyzer := NewProviderImageAnalyzer(registry, config)

		_, err = analyzer.AnalyzeImage(context.Background(), badFile.Name(), "What is this?", "low")
		if err == nil {
			t.Fatal("expected error for invalid image type")
		}
		if !strings.Contains(err.Error(), "invalid image type") {
			t.Errorf("expected 'invalid image type' error, got: %v", err)
		}
	})
}

// --- isValidImageType Tests ---

func TestIsValidImageType(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"photo.png", true},
		{"photo.gif", true},
		{"photo.webp", true},
		{"photo.JPG", true},
		{"photo.PNG", true},
		{"document.pdf", false},
		{"script.exe", false},
		{"data.csv", false},
		{"archive.zip", false},
		{"noext", false},
	}

	for _, tt := range tests {
		got := isValidImageType(tt.path)
		if got != tt.want {
			t.Errorf("isValidImageType(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// --- detectMimeType Tests ---

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"photo.png", "image/png"},
		{"photo.gif", "image/gif"},
		{"photo.webp", "image/webp"},
		{"photo.unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		got := detectMimeType(tt.path)
		if got != tt.want {
			t.Errorf("detectMimeType(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- Config Default Tests ---

func TestDefaultVisionConfig(t *testing.T) {
	cfg := DefaultVisionConfig()
	if cfg.Enabled {
		t.Error("default vision should be disabled")
	}
	if cfg.MaxImageSize != 20*1024*1024 {
		t.Errorf("expected max image size 20MB, got %d", cfg.MaxImageSize)
	}
}

func TestDefaultImageGenConfig(t *testing.T) {
	cfg := DefaultImageGenConfig()
	if cfg.Enabled {
		t.Error("default image_gen should be disabled")
	}
	if cfg.Provider != "openai" {
		t.Errorf("expected provider openai, got %s", cfg.Provider)
	}
	if cfg.Model != "dall-e-3" {
		t.Errorf("expected model dall-e-3, got %s", cfg.Model)
	}
	if cfg.MaxPromptLength != 1000 {
		t.Errorf("expected max_prompt_length 1000, got %d", cfg.MaxPromptLength)
	}
}

// --- RegisterVisionTools / RegisterImageGenTools ---

func TestRegisterVisionTools_Disabled(t *testing.T) {
	registry := NewToolRegistry()
	config := VisionConfig{Enabled: false}
	RegisterVisionTools(registry, config, nil)

	_, err := registry.Get("AnalyzeImage")
	if err == nil {
		t.Error("AnalyzeImage should not be registered when disabled")
	}
}

func TestRegisterImageGenTools_Disabled(t *testing.T) {
	registry := NewToolRegistry()
	config := ImageGenConfig{Enabled: false}
	RegisterImageGenTools(registry, config)

	_, err := registry.Get("GenerateImage")
	if err == nil {
		t.Error("GenerateImage should not be registered when disabled")
	}
}