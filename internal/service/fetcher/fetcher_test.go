package fetcher_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/service/fetcher"
)

// mockFetcher is a test double for port.JDFetcher.
type mockFetcher struct {
	text  string
	err   error
	calls int
}

func (m *mockFetcher) Fetch(_ context.Context, _ string) (string, error) {
	m.calls++
	return m.text, m.err
}

// htmlSrv returns an httptest server that serves the given HTML body.
func htmlSrv(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(body)) //nolint:errcheck
	}))
}

// --- GoqueryFetcher tests ---

func TestGoqueryFetcher_ReturnsBodyText(t *testing.T) {
	srv := htmlSrv(`<html><body><p>golang engineer role</p></body></html>`)
	defer srv.Close()

	f := fetcher.NewGoquery(nil)
	text, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "golang engineer role") {
		t.Errorf("expected body text, got: %q", text)
	}
}

func TestGoqueryFetcher_StripsScriptAndStyle(t *testing.T) {
	srv := htmlSrv(`<html><body><script>alert(1)</script><style>body{}</style><p>job description</p></body></html>`)
	defer srv.Close()

	f := fetcher.NewGoquery(nil)
	text, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "alert") {
		t.Errorf("script tag should have been stripped, got: %q", text)
	}
	if !strings.Contains(text, "job description") {
		t.Errorf("expected body text, got: %q", text)
	}
}

func TestGoqueryFetcher_ErrorOnCancelledContext(t *testing.T) {
	srv := htmlSrv(`<html><body>text</body></html>`)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f := fetcher.NewGoquery(nil)
	_, err := f.Fetch(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

// --- FallbackFetcher tests ---

func TestFallbackFetcher_UsesPrimaryWhenSuccessful(t *testing.T) {
	primary := &mockFetcher{text: strings.Repeat("a", 200)}
	fallback := &mockFetcher{text: "fallback text"}

	f := fetcher.NewFallbackWith(primary, fallback, 100, nil)
	text, err := f.Fetch(context.Background(), "http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if text != primary.text {
		t.Errorf("expected primary text, got: %q", text)
	}
	if fallback.calls != 0 {
		t.Errorf("expected fallback not called, got %d calls", fallback.calls)
	}
}

func TestFallbackFetcher_FallsBackOnPrimaryError(t *testing.T) {
	primary := &mockFetcher{err: context.DeadlineExceeded}
	fallback := &mockFetcher{text: "fallback content here"}

	f := fetcher.NewFallbackWith(primary, fallback, 100, nil)
	text, err := f.Fetch(context.Background(), "http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if text != fallback.text {
		t.Errorf("expected fallback text, got: %q", text)
	}
}

func TestFallbackFetcher_FallsBackOnThinContent(t *testing.T) {
	primary := &mockFetcher{text: "short"}
	fallback := &mockFetcher{text: "much longer fallback content that exceeds the minimum"}

	f := fetcher.NewFallbackWith(primary, fallback, 100, nil)
	text, err := f.Fetch(context.Background(), "http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if text != fallback.text {
		t.Errorf("expected fallback text, got: %q", text)
	}
}

func TestFallbackFetcher_ErrorWhenBothFail(t *testing.T) {
	primary := &mockFetcher{err: context.DeadlineExceeded}
	fallback := &mockFetcher{err: context.DeadlineExceeded}

	f := fetcher.NewFallbackWith(primary, fallback, 100, nil)
	_, err := f.Fetch(context.Background(), "http://example.com")
	if err == nil {
		t.Fatal("expected error when both fetchers fail, got nil")
	}
}
