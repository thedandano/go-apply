// Package llm provides an OpenAI-compatible HTTP client that satisfies both
// port.LLMClient (chat completions) and port.EmbeddingClient (embeddings).
// Any provider exposing the OpenAI-compatible protocol can be used by
// pointing BaseURL and Model at the right endpoint.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction checks.
var _ port.LLMClient = (*HTTPClient)(nil)
var _ port.EmbeddingClient = (*HTTPClient)(nil)

const (
	maxRetries  = 3
	baseBackoff = 500 * time.Millisecond
)

// HTTPClient implements port.LLMClient and port.EmbeddingClient using the
// OpenAI-compatible REST protocol. One instance per provider endpoint.
type HTTPClient struct {
	baseURL string
	model   string
	apiKey  string
	http    *http.Client
}

// New constructs an HTTPClient for the given provider config and application defaults.
// The HTTP client timeout is derived from defaults.LLM.TimeoutMS — never hardcoded.
func New(cfg config.LLMProviderConfig, defaults *config.AppDefaults) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		http: &http.Client{
			Timeout: time.Duration(defaults.LLM.TimeoutMS) * time.Millisecond,
		},
	}
}

// ChatComplete sends a chat completion request and returns the assistant's reply.
// It retries on 429 (rate limited) and 503 (service unavailable) with exponential
// backoff and full jitter. Other non-2xx status codes are returned as errors
// immediately without retry.
func (c *HTTPClient) ChatComplete(ctx context.Context, messages []port.ChatMessage, opts port.ChatOptions) (string, error) {
	type requestBody struct {
		Model       string             `json:"model"`
		Messages    []port.ChatMessage `json:"messages"`
		Temperature float64            `json:"temperature,omitempty"`
		MaxTokens   int                `json:"max_tokens,omitempty"`
	}

	type responseChoice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	type responseBody struct {
		Choices []responseChoice `json:"choices"`
	}

	payload := requestBody{
		Model:       c.model,
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	}

	var result responseBody
	if err := c.doWithRetry(ctx, "/v1/chat/completions", payload, &result); err != nil {
		return "", fmt.Errorf("chat complete: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("chat complete: empty choices in response")
	}

	content := result.Choices[0].Message.Content
	return extractJSON(content), nil
}

// Embed sends an embedding request and returns the float32 vector.
// Retries on 429/503 with exponential backoff and full jitter.
func (c *HTTPClient) Embed(ctx context.Context, text string) ([]float32, error) {
	type requestBody struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}

	type embeddingItem struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}
	type responseBody struct {
		Data []embeddingItem `json:"data"`
	}

	payload := requestBody{Model: c.model, Input: text}

	var result responseBody
	if err := c.doWithRetry(ctx, "/v1/embeddings", payload, &result); err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embed: empty data in response")
	}

	return result.Data[0].Embedding, nil
}

// doWithRetry executes a POST request to the given path, retrying on 429/503.
// It serializes payload as JSON and deserializes the response into out.
func (c *HTTPClient) doWithRetry(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + path

	for attempt := range maxRetries {
		if attempt > 0 {
			maxWait := baseBackoff * time.Duration(1<<attempt)
			//nolint:gosec // non-cryptographic jitter for backoff
			jitter := time.Duration(rand.Int63n(int64(maxWait)))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			// Context cancellation is not retriable.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Network/timeout errors: retry if attempts remain.
			if attempt < maxRetries-1 {
				continue
			}
			return fmt.Errorf("http request: %w", err)
		}

		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			_ = resp.Body.Close()
			// Retry unless this was the last attempt.
			if attempt == maxRetries-1 {
				return fmt.Errorf("status %d after %d attempts", resp.StatusCode, maxRetries)
			}
			continue
		default:
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				_ = resp.Body.Close()
				return fmt.Errorf("status %d", resp.StatusCode)
			}
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}

		return nil
	}

	return fmt.Errorf("exhausted %d attempts", maxRetries)
}

// extractJSON strips markdown code fences (```json ... ``` or ``` ... ```) and
// returns the innermost JSON object or array. If no JSON delimiters are found
// the original string is returned unchanged so plain-text responses are safe.
func extractJSON(s string) string {
	// Strip markdown fences if present.
	stripped := s
	for _, fence := range []string{"```json", "```"} {
		if idx := strings.Index(stripped, fence); idx != -1 {
			stripped = stripped[idx+len(fence):]
			if end := strings.LastIndex(stripped, "```"); end != -1 {
				stripped = stripped[:end]
			}
			stripped = strings.TrimSpace(stripped)
			break
		}
	}

	// Find outermost JSON object or array.
	start := -1
	var openChar, closeChar byte
	for i := 0; i < len(stripped); i++ {
		if stripped[i] == '{' {
			start = i
			openChar = '{'
			closeChar = '}'
			break
		}
		if stripped[i] == '[' {
			start = i
			openChar = '['
			closeChar = ']'
			break
		}
	}
	if start == -1 {
		return s // no JSON found — return original
	}

	_ = openChar // used implicitly via start index

	// Find the matching closing delimiter from the end.
	end := -1
	for i := len(stripped) - 1; i >= start; i-- {
		if stripped[i] == closeChar {
			end = i
			break
		}
	}
	if end == -1 {
		return s
	}

	return stripped[start : end+1]
}
