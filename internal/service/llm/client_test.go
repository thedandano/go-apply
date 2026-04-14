package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/llm"
)

// newTestClient builds a client pointed at srv with embedded defaults.
func newTestClient(t *testing.T, baseURL string) *llm.HTTPClient {
	t.Helper()
	return llm.New(baseURL, "test-model", "test-key", config.EmbeddedDefaults(), nil)
}

// chatOK returns an httptest server that always responds with the given content.
func chatOK(content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
		})
	}))
}

// embedOK returns an httptest server that always responds with the given vector.
func embedOK(vec []float32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": vec},
			},
		})
	}))
}

// --- ChatComplete tests ---

func TestChatComplete_ReturnsAssistantContent(t *testing.T) {
	srv := chatOK("hello from llm")
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	resp, err := client.ChatComplete(context.Background(), []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if resp != "hello from llm" {
		t.Errorf("got %q, want %q", resp, "hello from llm")
	}
}

func TestChatComplete_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.ChatComplete(context.Background(), []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{})
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestChatComplete_ErrorOnEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.ChatComplete(context.Background(), []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{})
	if err == nil {
		t.Fatal("expected error on empty choices, got nil")
	}
}

func TestChatComplete_RetriesOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok after retry"}},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	resp, err := client.ChatComplete(context.Background(), []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{MaxTokens: 100})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if resp != "ok after retry" {
		t.Errorf("got %q, want %q", resp, "ok after retry")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestChatComplete_RetriesOn503(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "recovered"}},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	resp, err := client.ChatComplete(context.Background(), []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{})
	if err != nil {
		t.Fatalf("expected success after 503 retry, got: %v", err)
	}
	if resp != "recovered" {
		t.Errorf("got %q, want %q", resp, "recovered")
	}
}

func TestChatComplete_ExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.ChatComplete(context.Background(), []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{})
	if err == nil {
		t.Fatal("expected error after exhausted retries, got nil")
	}
}

func TestChatComplete_RespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := newTestClient(t, srv.URL)
	_, err := client.ChatComplete(ctx, []model.ChatMessage{
		{Role: "user", Content: "hi"},
	}, model.ChatOptions{})
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

// --- Embed tests ---

func TestEmbed_ReturnsVector(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3}
	srv := embedOK(want)
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	got, err := client.Embed(context.Background(), "golang engineer")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got len %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%v, want %v", i, got[i], want[i])
		}
	}
}

func TestEmbed_ErrorOnEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Embed(context.Background(), "some text")
	if err == nil {
		t.Fatal("expected error on empty data, got nil")
	}
}

func TestEmbed_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Embed(context.Background(), "some text")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestEmbed_RetriesOn429(t *testing.T) {
	attempts := 0
	want := []float32{0.9, 0.8}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": want},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	got, err := client.Embed(context.Background(), "golang")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("got len %d, want %d", len(got), len(want))
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}
