package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aj-nt/mnemosyne/agent/provider"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// --- Image Analyzer Interface ---

// ImageAnalyzer abstracts image analysis for testability
type ImageAnalyzer interface {
	// AnalyzeImage sends an image to the LLM provider for analysis
	AnalyzeImage(ctx context.Context, imagePath, question, detail string) (string, error)
}

// --- Provider Image Analyzer ---

// ProviderImageAnalyzer implements ImageAnalyzer using the provider registry
type ProviderImageAnalyzer struct {
	registry *provider.ProviderRegistry
	config   VisionConfig
}

func NewProviderImageAnalyzer(registry *provider.ProviderRegistry, config VisionConfig) *ProviderImageAnalyzer {
	return &ProviderImageAnalyzer{
		registry: registry,
		config:   config,
	}
}

func (a *ProviderImageAnalyzer) AnalyzeImage(ctx context.Context, imagePath, question, detail string) (string, error) {
	// Validate image path
	if !isPathSafe(imagePath) {
		return "", fmt.Errorf("access denied: image path %q is not in an allowed directory", imagePath)
	}

	// Check file size
	fileInfo, err := os.Stat(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat image file: %w", err)
	}
	if fileInfo.Size() > int64(a.config.MaxImageSize) {
		return "", fmt.Errorf("image too large: %d bytes exceeds maximum of %d bytes", fileInfo.Size(), a.config.MaxImageSize)
	}

	// Validate image type
	if !isValidImageType(imagePath) {
		return "", fmt.Errorf("invalid image type: only JPEG, PNG, GIF, and WEBP are supported")
	}

	// Read image data
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image file: %w", err)
	}

	// Encode image as base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)
	mimeType := detectMimeType(imagePath)

	// Get primary provider
	prov, err := a.registry.GetPrimary()
	if err != nil {
		return "", fmt.Errorf("failed to get primary provider: %w", err)
	}

	// Check if provider supports vision
	caps := prov.Capabilities()
	if !caps.SupportsVision {
		return "", fmt.Errorf("primary provider does not support vision capabilities")
	}

	// Build the conversation context with image content
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{
			{
				Role:    "user",
				Content: question,
				// Add image content - this would be handled by provider-specific preparation
			},
		},
	}

	// Prepare the request with image content
	preparedReq := agentctx.PreparedRequest{
		Messages: convCtx.Messages,
		// Image content would be added here in provider-specific way
		ImageContent: &agentctx.ImageContent{
			Data:   imageBase64,
			MimeType: mimeType,
			Detail: detail,
		},
	}

	// Execute the request
	req, err := prov.Prepare(ctx, convCtx, preparedReq)
	if err != nil {
		return "", fmt.Errorf("failed to prepare request: %w", err)
	}

	result, err := prov.Execute(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}

	// Parse the response
	parsed, err := prov.Parse(ctx, result)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return parsed.Text, nil
}

// --- Image Generator Interface ---

// ImageGenerator abstracts image generation for testability
type ImageGenerator interface {
	// GenerateImage creates an image from a text prompt
	GenerateImage(ctx context.Context, prompt, size string, n int) ([]string, error)
}

// --- DALL-E Image Generator ---

// DalleImageGenerator implements ImageGenerator using OpenAI DALL-E API
type DalleImageGenerator struct {
	apiKey    string
	model     string
	cacheDir  string
	config    ImageGenConfig
}

func NewDalleImageGenerator(apiKey, model, cacheDir string, config ImageGenConfig) *DalleImageGenerator {
	return &DalleImageGenerator{
		apiKey:   apiKey,
		model:    model,
		cacheDir: cacheDir,
		config:   config,
	}
}

func (g *DalleImageGenerator) GenerateImage(ctx context.Context, prompt, size string, n int) ([]string, error) {
	// Validate prompt length
	if len(prompt) > g.config.MaxPromptLength {
		return nil, fmt.Errorf("prompt too long: %d characters exceeds maximum of %d", len(prompt), g.config.MaxPromptLength)
	}

	// Validate size
	validSizes := map[string]bool{
		"256x256":     true,
		"512x512":     true,
		"1024x1024":   true,
	}
	if size != "" && !validSizes[size] {
		return nil, fmt.Errorf("invalid size: must be one of 256x256, 512x512, 1024x1024")
	}

	// Validate n
	if n < 1 || n > 10 {
		return nil, fmt.Errorf("invalid n: must be between 1 and 10")
	}

	// Call DALL-E API
	url := "https://api.openai.com/v1/images/generations"
	
	// Create request body
	requestBody := map[string]interface{}{
		"model":  g.model,
		"prompt": prompt,
		"n":      n,
	}
	if size != "" {
		requestBody["size"] = size
	}

	// Make HTTP request
	reqJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Download and save images
	var imagePaths []string
	for i, data := range response.Data {
		imageData, err := downloadImage(ctx, data.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to download image %d: %w", i, err)
		}

		// Save image
		imagePath := filepath.Join(g.cacheDir, fmt.Sprintf("generated_%d.png", time.Now().UnixNano()))
		if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
			return nil, fmt.Errorf("failed to save image %d: %w", i, err)
		}

		imagePaths = append(imagePaths, imagePath)
	}

	return imagePaths, nil
}

func downloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// --- Vision Config ---

// VisionConfig configures the vision tool
type VisionConfig struct {
	Enabled       bool   `yaml:"enabled"`
	MaxImageSize  int    `yaml:"max_image_size"` // in bytes, default 20MB
}

// DefaultVisionConfig returns sensible defaults
func DefaultVisionConfig() VisionConfig {
	return VisionConfig{
		Enabled:      false,
		MaxImageSize: 20 * 1024 * 1024, // 20MB
	}
}

// --- Image Generation Config ---

// ImageGenConfig configures the image generation tool
type ImageGenConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Provider       string `yaml:"provider"` // openai or stability
	Model          string `yaml:"model"`
	APIKeyEnv      string `yaml:"api_key_env"`
	MaxPromptLength int    `yaml:"max_prompt_length"` // default 1000
	CacheDir       string `yaml:"cache_dir"`
}

// DefaultImageGenConfig returns sensible defaults
func DefaultImageGenConfig() ImageGenConfig {
	return ImageGenConfig{
		Enabled:        false,
		Provider:       "openai",
		Model:          "dall-e-3",
		APIKeyEnv:      "OPENAI_API_KEY",
		MaxPromptLength: 1000,
		CacheDir:       "",
	}
}

// --- AnalyzeImage Tool ---

// AnalyzeImageSchema is the JSON schema for the AnalyzeImage tool
var AnalyzeImageSchema = map[string]interface{}{
	"name":        "AnalyzeImage",
	"description": "Analyze images using vision-capable LLM providers. Ask questions about image content.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"image_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the image file to analyze",
			},
			"question": map[string]interface{}{
				"type":        "string",
				"description": "Question to ask about the image content",
			},
			"detail": map[string]interface{}{
				"type":        "string",
				"description": "Detail level: low or high (optional, default: low)",
				"enum":        []string{"low", "high"},
			},
		},
		"required": []string{"image_path", "question"},
	},
}

// Global vision state
var (
	imageAnalyzer ImageAnalyzer
	visionConfig  VisionConfig
)

// RegisterVisionTools adds the AnalyzeImage tool to the registry
func RegisterVisionTools(registry *ToolRegistry, config VisionConfig, providerRegistry *provider.ProviderRegistry) {
	visionConfig = config
	
	if config.Enabled {
		imageAnalyzer = NewProviderImageAnalyzer(providerRegistry, config)
		registry.Register("AnalyzeImage", AnalyzeImage, AnalyzeImageSchema)
	}
}

// AnalyzeImage is the tool handler for image analysis
func AnalyzeImage(args map[string]interface{}) (string, bool) {
	if imageAnalyzer == nil {
		return "Vision tools are not enabled in configuration", true
	}

	imagePath := strArg(args, "image_path", "")
	question := strArg(args, "question", "")
	detail := strArg(args, "detail", "low")

	if imagePath == "" {
		return "image_path is required", true
	}
	if question == "" {
		return "question is required", true
	}

	// Validate detail
	if detail != "low" && detail != "high" {
		return "detail must be either 'low' or 'high'", true
	}

	ctx := context.Background()
	result, err := imageAnalyzer.AnalyzeImage(ctx, imagePath, question, detail)
	if err != nil {
		return fmt.Sprintf("Image analysis failed: %v", err), true
	}

	// Truncate large results
	const maxResultChars = 20000
	return truncateOutput(result, maxResultChars), false
}

// --- GenerateImage Tool ---

// GenerateImageSchema is the JSON schema for the GenerateImage tool
var GenerateImageSchema = map[string]interface{}{
	"name":        "GenerateImage",
	"description": "Generate images using DALL-E or Stability API from text prompts.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Text prompt describing the image to generate",
			},
			"size": map[string]interface{}{
				"type":        "string",
				"description": "Image size: 256x256, 512x512, or 1024x1024 (optional, default: 1024x1024)",
				"enum":        []string{"256x256", "512x512", "1024x1024"},
			},
			"n": map[string]interface{}{
				"type":        "integer",
				"description": "Number of images to generate (optional, default: 1, max: 10)",
				"minimum":     1,
				"maximum":     10,
			},
		},
		"required": []string{"prompt"},
	},
}

// Global image generation state
var (
	imageGenerator ImageGenerator
	imageGenConfig  ImageGenConfig
)

// RegisterImageGenTools adds the GenerateImage tool to the registry
func RegisterImageGenTools(registry *ToolRegistry, config ImageGenConfig) {
	imageGenConfig = config
	
	if config.Enabled {
		apiKey := os.Getenv(config.APIKeyEnv)
		if apiKey == "" {
			log.Printf("Warning: Image generation enabled but no API key found in %s", config.APIKeyEnv)
			return
		}

		if config.CacheDir == "" {
			homeDir, _ := os.UserHomeDir()
			config.CacheDir = filepath.Join(homeDir, ".mnemosyne", "generated_images")
			imageGenConfig.CacheDir = config.CacheDir
		}

		// Create cache directory
		if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
			log.Printf("Warning: Failed to create image cache directory: %v", err)
			return
		}

		imageGenerator = NewDalleImageGenerator(apiKey, config.Model, config.CacheDir, config)
		registry.Register("GenerateImage", GenerateImage, GenerateImageSchema)
	}
}

// GenerateImage is the tool handler for image generation
func GenerateImage(args map[string]interface{}) (string, bool) {
	if imageGenerator == nil {
		return "Image generation tools are not enabled or configured in configuration", true
	}

	prompt := strArg(args, "prompt", "")
	size := strArg(args, "size", "1024x1024")
	n := intArg(args, "n", 1)

	if prompt == "" {
		return "prompt is required", true
	}

	ctx := context.Background()
	imagePaths, err := imageGenerator.GenerateImage(ctx, prompt, size, n)
	if err != nil {
		return fmt.Sprintf("Image generation failed: %v", err), true
	}

	if len(imagePaths) == 1 {
		return fmt.Sprintf("Image generated and saved to %s", imagePaths[0]), false
	}

	// Multiple images
	pathsList := strings.Join(imagePaths, ", ")
	return fmt.Sprintf("%d images generated and saved to: %s", len(imagePaths), pathsList), false
}

// --- Utility Functions ---

// isValidImageType checks if a file has a valid image extension
func isValidImageType(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	validExts := map[string]bool{
		".jpg":  true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	}
	return validExts[ext]
}

// detectMimeType detects the MIME type from file extension
func detectMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}