package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/llm"
)

// testDefaults returns minimal AppDefaults for testing with a short timeout.
func testDefaults() *config.AppDefaults {
	return &config.AppDefaults{
		LLM: config.LLMDefaults{
			TimeoutMS: 5000,
		},
	}
}

// chatResponse builds a minimal /chat/completions response body.
func chatResponse(content string) []byte {
	resp := map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// embeddingResponse builds a minimal /embeddings response body.
func embeddingResponse(vec []float32) []byte {
	resp := map[string]any{
		"data": []any{
			map[string]any{
				"embedding": vec,
				"index":     0,
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// TestHTTPClient_ChatComplete_Success verifies that a normal chat response is
// parsed and the content string is returned without modification.
func TestHTTPClient_ChatComplete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatResponse("hello world"))
	}))
	defer server.Close()

	cfg := config.LLMProviderConfig{BaseURL: server.URL, Model: "test-model"}
	client := llm.New(cfg, testDefaults())

	got, err := client.ChatComplete(context.Background(), nil, port.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

// TestHTTPClient_ChatComplete_MarkdownWrapped verifies that markdown-fenced JSON
// in the response content is stripped and the inner JSON returned.
func TestHTTPClient_ChatComplete_MarkdownWrapped(t *testing.T) {
	wrapped := "```json\n{\"key\":\"val\"}\n```"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatResponse(wrapped))
	}))
	defer server.Close()

	cfg := config.LLMProviderConfig{BaseURL: server.URL, Model: "test-model"}
	client := llm.New(cfg, testDefaults())

	got, err := client.ChatComplete(context.Background(), nil, port.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return the inner JSON with curly braces.
	if got != `{"key":"val"}` {
		t.Errorf("expected %q, got %q", `{"key":"val"}`, got)
	}
}

// TestHTTPClient_ChatComplete_RetriesOn429 verifies that a 429 response triggers
// a retry and the second (200) response is ultimately returned.
func TestHTTPClient_ChatComplete_RetriesOn429(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatResponse("retried ok"))
	}))
	defer server.Close()

	cfg := config.LLMProviderConfig{BaseURL: server.URL, Model: "test-model"}
	client := llm.New(cfg, testDefaults())

	got, err := client.ChatComplete(context.Background(), nil, port.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if got != "retried ok" {
		t.Errorf("expected %q, got %q", "retried ok", got)
	}
	if callCount.Load() < 2 {
		t.Errorf("expected at least 2 calls (1 retry), got %d", callCount.Load())
	}
}

// TestHTTPClient_Embed_Success verifies that the embedding endpoint is called and
// the float32 slice is returned correctly.
func TestHTTPClient_Embed_Success(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(embeddingResponse(want))
	}))
	defer server.Close()

	cfg := config.LLMProviderConfig{BaseURL: server.URL, Model: "test-model"}
	client := llm.New(cfg, testDefaults())

	got, err := client.Embed(context.Background(), "some text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d floats, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: expected %v, got %v", i, want[i], got[i])
		}
	}
}

// TestHTTPClient_ChatComplete_ContextCancel verifies that cancelling the context
// during the backoff sleep returns context.Canceled or context.DeadlineExceeded.
func TestHTTPClient_ChatComplete_ContextCancel(t *testing.T) {
	// Server always returns 429 so the client enters backoff.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	cfg := config.LLMProviderConfig{BaseURL: server.URL, Model: "test-model"}
	client := llm.New(cfg, testDefaults())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.ChatComplete(ctx, nil, port.ChatOptions{})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context error, got: %v", err)
	}
}

// TestHTTPClient_ChatComplete_ServerError verifies that a 500 response (not
// retriable) returns an error immediately without exhausting retries.
func TestHTTPClient_ChatComplete_ServerError(t *testing.T) {
	var callCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.LLMProviderConfig{BaseURL: server.URL, Model: "test-model"}
	client := llm.New(cfg, testDefaults())

	_, err := client.ChatComplete(context.Background(), nil, port.ChatOptions{})
	if err == nil {
		t.Fatal("expected an error on 500, got nil")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected exactly 1 call (no retry on 500), got %d", callCount.Load())
	}
}
