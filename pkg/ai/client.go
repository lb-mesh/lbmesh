// Package ai implements the LLM client engine, recipe catalog summarizer,
// and structured execution planner for `lbmesh ask`.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lbmesh/lbmesh-v2/pkg/config"
)

// Supported LLM provider names.
const (
	ProviderOllama    = "ollama"
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderLiteLLM   = "litellm"
)

// Default API endpoints and models.
const (
	DefaultOllamaURL   = "http://localhost:11434"
	DefaultOllamaModel = "llama3"

	DefaultOpenAIURL   = "https://api.openai.com/v1"
	DefaultOpenAIModel = "gpt-4o"

	DefaultAnthropicURL     = "https://api.anthropic.com/v1"
	DefaultAnthropicModel   = "claude-sonnet-5-5"
	AnthropicVersionHeader  = "2023-06-01"

	DefaultLiteLLMURL   = "http://localhost:4000/v1"
	DefaultLiteLLMModel = "gpt-4o"
)

// Client defines the common interface for sending system and user prompts to an LLM.
type Client interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// HTTPClient allows injecting custom HTTP clients (e.g. for unit testing).
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewClient constructs an appropriate Client implementation based on the AI configuration.
func NewClient(cfg config.AIConfig) (Client, error) {
	return NewClientWithHTTP(cfg, &http.Client{Timeout: 90 * time.Second})
}

// NewClientWithHTTP constructs a Client with a custom HTTPClient for testing or custom timeouts.
func NewClientWithHTTP(cfg config.AIConfig, httpClient HTTPClient) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		// Default to ollama if unspecified
		provider = ProviderOllama
	}

	switch provider {
	case ProviderOllama:
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			baseURL = DefaultOllamaURL
		}
		model := strings.TrimSpace(cfg.Model)
		if model == "" {
			model = DefaultOllamaModel
		}
		apiKey := resolveAPIKey(cfg.APIKey, "OLLAMA_API_KEY")
		return &ollamaClient{
			baseURL:    strings.TrimSuffix(baseURL, "/"),
			model:      model,
			apiKey:     apiKey,
			httpClient: httpClient,
		}, nil

	case ProviderOpenAI, ProviderLiteLLM:
		baseURL := strings.TrimSpace(cfg.BaseURL)
		defaultModel := DefaultOpenAIModel
		defaultURL := DefaultOpenAIURL
		envKey := "OPENAI_API_KEY"

		if provider == ProviderLiteLLM {
			defaultModel = DefaultLiteLLMModel
			defaultURL = DefaultLiteLLMURL
			envKey = "LITELLM_API_KEY"
		}

		if baseURL == "" {
			baseURL = defaultURL
		}
		model := strings.TrimSpace(cfg.Model)
		if model == "" {
			model = defaultModel
		}
		apiKey := resolveAPIKey(cfg.APIKey, envKey, "OPENAI_API_KEY")
		if apiKey == "" && provider == ProviderOpenAI {
			return nil, errors.New("OpenAI API key is required (set in config or via OPENAI_API_KEY environment variable)")
		}

		return &openAICompatibleClient{
			provider:   provider,
			baseURL:    strings.TrimSuffix(baseURL, "/"),
			apiKey:     apiKey,
			model:      model,
			httpClient: httpClient,
		}, nil

	case ProviderAnthropic:
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			baseURL = DefaultAnthropicURL
		}
		model := strings.TrimSpace(cfg.Model)
		if model == "" {
			model = DefaultAnthropicModel
		}
		apiKey := resolveAPIKey(cfg.APIKey, "ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, errors.New("Anthropic API key is required (set in config or via ANTHROPIC_API_KEY environment variable)")
		}

		return &anthropicClient{
			baseURL:    strings.TrimSuffix(baseURL, "/"),
			apiKey:     apiKey,
			model:      model,
			httpClient: httpClient,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported AI provider %q (supported: ollama, openai, anthropic, litellm)", provider)
	}
}

func resolveAPIKey(cfgKey string, envKeys ...string) string {
	if cfgKey = strings.TrimSpace(cfgKey); cfgKey != "" {
		return cfgKey
	}
	for _, env := range envKeys {
		if val := strings.TrimSpace(os.Getenv(env)); val != "" {
			return val
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Ollama Client
// ---------------------------------------------------------------------------

type ollamaClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient HTTPClient
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Format   string              `json:"format,omitempty"`
}

type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Error string `json:"error,omitempty"`
}

func (c *ollamaClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	endpoint := c.baseURL
	if !strings.HasSuffix(endpoint, "/api/chat") {
		endpoint += "/api/chat"
	}

	reqBody := ollamaChatRequest{
		Model: c.model,
		Messages: []ollamaChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
		Format: "json",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connecting to Ollama at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading Ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ollamaChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("parsing Ollama response JSON: %w", err)
	}

	if chatResp.Error != "" {
		return "", fmt.Errorf("Ollama error: %s", chatResp.Error)
	}

	return chatResp.Message.Content, nil
}

// ---------------------------------------------------------------------------
// OpenAI / LiteLLM Client (OpenAI-compatible)
// ---------------------------------------------------------------------------

type openAICompatibleClient struct {
	provider   string
	baseURL    string
	apiKey     string
	model      string
	httpClient HTTPClient
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIChatMessage   `json:"messages"`
	Temperature    float64               `json:"temperature"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *openAICompatibleClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	endpoint := c.baseURL
	if strings.HasSuffix(endpoint, "/chat/completions") {
		// already has full endpoint
	} else if strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/chat/completions"
	} else {
		endpoint += "/chat/completions"
	}

	reqBody := openAIChatRequest{
		Model: c.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.1,
		ResponseFormat: &openAIResponseFormat{Type: "json_object"},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connecting to %s at %s: %w", c.provider, endpoint, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading %s response: %w", c.provider, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s API error (HTTP %d): %s", c.provider, resp.StatusCode, string(bodyBytes))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("parsing %s response JSON: %w", c.provider, err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return "", fmt.Errorf("%s error: %s", c.provider, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("%s returned no completion choices", c.provider)
	}

	return chatResp.Choices[0].Message.Content, nil
}

// ---------------------------------------------------------------------------
// Anthropic Client
// ---------------------------------------------------------------------------

type anthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient HTTPClient
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *anthropicClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	endpoint := c.baseURL
	if strings.HasSuffix(endpoint, "/messages") {
		// already has messages endpoint
	} else if strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/messages"
	} else {
		endpoint += "/v1/messages"
	}

	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", AnthropicVersionHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connecting to Anthropic at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading Anthropic response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Anthropic API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(bodyBytes, &anthropicResp); err != nil {
		return "", fmt.Errorf("parsing Anthropic response JSON: %w", err)
	}

	if anthropicResp.Error != nil && anthropicResp.Error.Message != "" {
		return "", fmt.Errorf("Anthropic error: %s", anthropicResp.Error.Message)
	}

	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", errors.New("Anthropic returned no text content blocks")
}
