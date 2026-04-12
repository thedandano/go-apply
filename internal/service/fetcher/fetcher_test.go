package fetcher_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/service/fetcher"
)

func testDefaults() *config.AppDefaults {
	d := config.EmbeddedDefaults()
	// Set a small threshold so most tests pass; specific tests override
	d.Fetcher.MinJDTextLengthChars = 10
	return d
}

func TestGoqueryFetcher_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><body>
<p>We are looking for a talented Go developer to join our team.</p>
<p>Requirements: Go 1.20+, Docker, Kubernetes.</p>
</body></html>`))
	}))
	defer srv.Close()

	f := fetcher.NewGoqueryFetcher(testDefaults())
	text, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Go developer") {
		t.Errorf("expected body text in result; got: %q", text)
	}
}

func TestGoqueryFetcher_StripsScriptTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><body>
<p>Job description here for a software role.</p>
<script>var secret = "should not appear";</script>
<style>.hidden { display: none }</style>
</body></html>`))
	}))
	defer srv.Close()

	f := fetcher.NewGoqueryFetcher(testDefaults())
	text, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(text, "should not appear") {
		t.Errorf("script content should be stripped; got: %q", text)
	}
	if strings.Contains(text, "hidden") {
		t.Errorf("style content should be stripped; got: %q", text)
	}
	if !strings.Contains(text, "Job description") {
		t.Errorf("body text should be present; got: %q", text)
	}
}

func TestGoqueryFetcher_TooShort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>short</p></body></html>`))
	}))
	defer srv.Close()

	d := testDefaults()
	d.Fetcher.MinJDTextLengthChars = 1000 // force failure
	f := fetcher.NewGoqueryFetcher(d)

	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error for too-short content")
	}
}

func TestGoqueryFetcher_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// slow response; context will be cancelled before handler writes
		_, _ = w.Write([]byte(`<html><body>content</body></html>`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	f := fetcher.NewGoqueryFetcher(testDefaults())
	_, err := f.Fetch(ctx, srv.URL)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
