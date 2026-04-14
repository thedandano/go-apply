// Package llm provides an OpenAI-compatible HTTP client implementing both
// port.LLMClient (chat completions) and port.EmbeddingClient (embeddings).
// Two independent instances are used at runtime: one for the orchestrator
// provider, one for the embedder provider — each with their own base URL and model.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction checks.
var _ port.LLMClient = (*HTTPClient)(nil)
var _ port.EmbeddingClient = (*HTTPClient)(nil)

const (
	maxAttempts = 3
	baseBackoff = 500 * time.Millisecond
)

// HTTPClient implements port.LLMClient and port.EmbeddingClient using the
// OpenAI-compatible chat completions and embeddings endpoints.
type HTTPClient struct {
	baseURL string
	model   string
	apiKey  string
	http    *http.Client
	log     *slog.Logger
}

// New constructs an HTTPClient. Timeout comes from defaults.LLM.TimeoutMS.
// log may be nil — a discarding logger is used in that case.
func New(baseURL, model, apiKey string, defaults *config.AppDefaults, log *slog.Logger) *HTTPClient {
	if log == nil {
		log = slog.Default()
	}
	timeout := time.Duration(defaults.LLM.TimeoutMS) * time.Millisecond
	return &HTTPClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
		log:     log,
	}
}

// chatRequest is the OpenAI-compatible chat completions request body.
type chatRequest struct {
	Model       string              `json:"model"`
	Messages    []model.ChatMessage `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

// chatResponse is the subset of the OpenAI chat completions response we use.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// embedRequest is the OpenAI-compatible embeddings request body.
type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embedResponse is the subset of the OpenAI embeddings response we use.
type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// ChatComplete implements port.LLMClient using the /chat/completions endpoint.
// Retries on 429 and 503 with exponential backoff + full jitter.
func (c *HTTPClient) ChatComplete(ctx context.Context, messages []model.ChatMessage, opts model.ChatOptions) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("llm: marshal chat request: %w", err)
	}

	var result chatResponse
	if err := c.doWithRetry(ctx, c.baseURL+"/chat/completions", body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("llm: no choices in chat response")
	}
	return result.Choices[0].Message.Content, nil
}

// Embed implements port.EmbeddingClient using the /embeddings endpoint.
// Retries on 429 and 503 with exponential backoff + full jitter.
func (c *HTTPClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embedRequest{Model: c.model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("llm: marshal embed request: %w", err)
	}

	var result embedResponse
	if err := c.doWithRetry(ctx, c.baseURL+"/embeddings", body, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("llm: no embedding in response")
	}
	return result.Data[0].Embedding, nil
}

// doWithRetry executes a POST request, retrying on 429 and 503 with exponential
// backoff + full jitter. Non-retryable statuses are returned as errors immediately.
// Response bodies are closed explicitly on every code path.
func (c *HTTPClient) doWithRetry(ctx context.Context, url string, body []byte, out any) error {
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			// Exponential backoff with full jitter: sleep [0, base * 2^attempt)
			maxWait := baseBackoff * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Int64N(int64(maxWait)))
			c.log.DebugContext(ctx, "llm: retrying after backoff",
				"attempt", attempt,
				"jitter_ms", jitter.Milliseconds(),
				"url", url,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("llm: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("llm: http do: %w", err)
			c.log.WarnContext(ctx, "llm: request failed, will retry",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			decodeErr := json.NewDecoder(resp.Body).Decode(out)
			_ = resp.Body.Close()
			return decodeErr
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("llm: API returned %d", resp.StatusCode)
			c.log.WarnContext(ctx, "llm: retryable status received",
				"status", resp.StatusCode,
				"attempt", attempt+1,
			)
		default:
			_ = resp.Body.Close()
			return fmt.Errorf("llm: API returned status %d", resp.StatusCode)
		}
	}
	return fmt.Errorf("llm: all %d attempts failed: %w", maxAttempts, lastErr)
}
