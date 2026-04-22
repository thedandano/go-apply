// Package llm provides an OpenAI-compatible HTTP client implementing port.LLMClient.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.LLMClient = (*HTTPClient)(nil)

const (
	maxAttempts = 3
	baseBackoff = 500 * time.Millisecond
)

// HTTPClient implements port.LLMClient using the OpenAI-compatible chat completions endpoint.
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

	c.log.DebugContext(ctx, "llm request",
		slog.String("model", c.model),
		slog.Int("prompt_bytes", len(body)),
		logger.PayloadAttr("prompt", string(body), logger.Verbose()),
	)

	start := time.Now()
	var result chatResponse
	if err := c.doWithRetry(ctx, c.baseURL+"/chat/completions", body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("llm: no choices in chat response")
	}
	completion := result.Choices[0].Message.Content
	c.log.DebugContext(ctx, "llm response",
		slog.Int("response_bytes", len(completion)),
		slog.Int64("elapsed_ms", time.Since(start).Milliseconds()),
		logger.PayloadAttr("completion", completion, logger.Verbose()),
	)
	return completion, nil
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
			jitter := time.Duration(rand.Int64N(int64(maxWait))) // #nosec G404 -- jitter for backoff, not a security-sensitive operation
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
			if attempt+1 < maxAttempts {
				c.log.DebugContext(ctx, "llm: retrying after http error", slog.Int("attempt", attempt+1))
			} else {
				c.log.DebugContext(ctx, "llm: aborting after http error — max attempts reached", slog.Int("attempt", attempt+1))
			}
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
			if attempt+1 < maxAttempts {
				c.log.DebugContext(ctx, "llm: retrying after retryable status",
					slog.Int("attempt", attempt+1),
					slog.Int("status", resp.StatusCode),
				)
			} else {
				c.log.DebugContext(ctx, "llm: aborting — max attempts exceeded",
					slog.Int("attempt", attempt+1),
					slog.Int("status", resp.StatusCode),
				)
			}
		default:
			var bodySnippet string
			if b, readErr := io.ReadAll(io.LimitReader(resp.Body, 512)); readErr == nil {
				bodySnippet = string(b)
			}
			_ = resp.Body.Close()
			c.log.DebugContext(ctx, "llm: aborting — non-retryable status", slog.Int("status", resp.StatusCode))
			c.log.ErrorContext(ctx, "llm: non-retryable error", "status", resp.StatusCode, "body", bodySnippet, "url", url)
			return fmt.Errorf("llm: API returned status %d: %s", resp.StatusCode, bodySnippet)
		}
	}
	return fmt.Errorf("llm: all %d attempts failed: %w", maxAttempts, lastErr)
}
