package ai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lbmesh/lbmesh-v2/pkg/config"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestOllamaClient(t *testing.T) {
	mock := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(req.URL.Path, "/api/chat") {
				t.Fatalf("unexpected URL path: %s", req.URL.Path)
			}
			bodyBytes, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(bodyBytes), `"model":"llama3"`) {
				t.Errorf("expected model llama3, got %s", string(bodyBytes))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"message": {
						"role": "assistant",
						"content": "{\"plan\":[{\"recipe\":\"postgresql\"}]}"
					}
				}`)),
			}, nil
		},
	}

	cfg := config.AIConfig{
		Provider: "ollama",
		BaseURL:  "http://localhost:11434",
		Model:    "llama3",
	}

	client, err := NewClientWithHTTP(cfg, mock)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	res, err := client.Generate(context.Background(), "sys prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error generating: %v", err)
	}

	if !strings.Contains(res, "postgresql") {
		t.Errorf("unexpected result: %s", res)
	}
}

func TestOpenAIClient(t *testing.T) {
	mock := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.HasSuffix(req.URL.Path, "/chat/completions") {
				t.Fatalf("unexpected URL path: %s", req.URL.Path)
			}
			if req.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("unexpected auth header: %s", req.Header.Get("Authorization"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"choices": [{
						"message": {
							"role": "assistant",
							"content": "{\"plan\":[{\"recipe\":\"redis\"}]}"
						}
					}]
				}`)),
			}, nil
		},
	}

	cfg := config.AIConfig{
		Provider: "openai",
		APIKey:   "test-key",
		Model:    "gpt-4o",
	}

	client, err := NewClientWithHTTP(cfg, mock)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	res, err := client.Generate(context.Background(), "sys prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error generating: %v", err)
	}

	if !strings.Contains(res, "redis") {
		t.Errorf("unexpected result: %s", res)
	}
}

func TestLiteLLMClient(t *testing.T) {
	mock := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") != "Bearer sk-litellm-123" {
				t.Errorf("unexpected auth header: %s", req.Header.Get("Authorization"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"choices": [{
						"message": {
							"role": "assistant",
							"content": "{\"plan\":[{\"recipe\":\"nginx\"}]}"
						}
					}]
				}`)),
			}, nil
		},
	}

	cfg := config.AIConfig{
		Provider: "litellm",
		BaseURL:  "http://localhost:4000/v1",
		APIKey:   "sk-litellm-123",
		Model:    "gemini/gemini-1.5-pro",
	}

	client, err := NewClientWithHTTP(cfg, mock)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	res, err := client.Generate(context.Background(), "sys prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error generating: %v", err)
	}

	if !strings.Contains(res, "nginx") {
		t.Errorf("unexpected result: %s", res)
	}
}

func TestAnthropicClient(t *testing.T) {
	mock := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("x-api-key") != "ant-test-key" {
				t.Errorf("unexpected api key: %s", req.Header.Get("x-api-key"))
			}
			if req.Header.Get("anthropic-version") != "2023-06-01" {
				t.Errorf("unexpected anthropic-version: %s", req.Header.Get("anthropic-version"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"content": [{
						"type": "text",
						"text": "{\"plan\":[{\"recipe\":\"postgresql\"}]}"
					}]
				}`)),
			}, nil
		},
	}

	cfg := config.AIConfig{
		Provider: "anthropic",
		APIKey:   "ant-test-key",
		Model:    "claude-3-5-sonnet-20241022",
	}

	client, err := NewClientWithHTTP(cfg, mock)
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}

	res, err := client.Generate(context.Background(), "sys prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error generating: %v", err)
	}

	if !strings.Contains(res, "postgresql") {
		t.Errorf("unexpected result: %s", res)
	}
}
