package fetcher_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

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

	f := fetcher.NewGoquery(8000, nil)
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

	f := fetcher.NewGoquery(8000, nil)
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

	f := fetcher.NewGoquery(8000, nil)
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

// --- ExtractJDMarkdown tests ---

func TestExtractJDMarkdown_ScopesToMain(t *testing.T) {
	html := `<html><body>
		<nav>Site navigation noise</nav>
		<main><h1>Software Engineer</h1><p>We need Python and Go skills.</p></main>
		<footer>Footer noise</footer>
	</body></html>`

	result := fetcher.ExtractJDMarkdown(html, 8000)

	if strings.Contains(result, "Site navigation noise") {
		t.Errorf("nav content should be excluded, got: %q", result)
	}
	if strings.Contains(result, "Footer noise") {
		t.Errorf("footer content should be excluded, got: %q", result)
	}
	if !strings.Contains(result, "Software Engineer") {
		t.Errorf("main content should be included, got: %q", result)
	}
	if !strings.Contains(result, "Python") {
		t.Errorf("main content should be included, got: %q", result)
	}
}

func TestExtractJDMarkdown_ScopesToArticleWhenNoMain(t *testing.T) {
	html := `<html><body>
		<header>Header noise</header>
		<article><h2>Backend Engineer</h2><p>Required: Golang, Kubernetes.</p></article>
	</body></html>`

	result := fetcher.ExtractJDMarkdown(html, 8000)

	if strings.Contains(result, "Header noise") {
		t.Errorf("header content should be excluded, got: %q", result)
	}
	if !strings.Contains(result, "Backend Engineer") {
		t.Errorf("article content should be included, got: %q", result)
	}
}

func TestExtractJDMarkdown_FallsBackToBodyWhenNoSemanticContainer(t *testing.T) {
	html := `<html><body><p>This is the whole job description.</p></body></html>`

	result := fetcher.ExtractJDMarkdown(html, 8000)

	if !strings.Contains(result, "This is the whole job description") {
		t.Errorf("body content should be included, got: %q", result)
	}
}

func TestExtractJDMarkdown_TruncatesToMaxChars(t *testing.T) {
	longContent := strings.Repeat("Python Go Kubernetes ", 500) // ~10000 chars
	html := fmt.Sprintf(`<html><body><main><p>%s</p></main></body></html>`, longContent)

	result := fetcher.ExtractJDMarkdown(html, 200)

	if len(result) > 200 {
		t.Errorf("result should be truncated to 200 chars, got %d chars", len(result))
	}
}

func TestExtractJDMarkdown_PreservesMarkdownStructure(t *testing.T) {
	html := `<html><body><main>
		<h2>Requirements</h2>
		<ul>
			<li>5+ years Python</li>
			<li>AWS experience</li>
		</ul>
	</main></body></html>`

	result := fetcher.ExtractJDMarkdown(html, 8000)

	if !strings.Contains(result, "Requirements") {
		t.Errorf("heading should be present, got: %q", result)
	}
	if !strings.Contains(result, "Python") {
		t.Errorf("list items should be present, got: %q", result)
	}
	if !strings.Contains(result, "AWS") {
		t.Errorf("list items should be present, got: %q", result)
	}
}

func TestGoqueryFetcher_ScopesToMainAndProducesMarkdown(t *testing.T) {
	srv := htmlSrv(`<html><body>
		<nav>Site nav noise</nav>
		<main><h2>Backend Engineer</h2><ul><li>Python</li><li>AWS</li></ul></main>
		<footer>Footer noise</footer>
	</body></html>`)
	defer srv.Close()

	f := fetcher.NewGoquery(8000, nil)
	text, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "Site nav noise") {
		t.Errorf("nav content should be excluded, got: %q", text)
	}
	if strings.Contains(text, "Footer noise") {
		t.Errorf("footer content should be excluded, got: %q", text)
	}
	if !strings.Contains(text, "Backend Engineer") {
		t.Errorf("main content should be included, got: %q", text)
	}
}

func TestGoqueryFetcher_TruncatesLargePage(t *testing.T) {
	longText := strings.Repeat("word ", 5000) // ~25000 chars
	srv := htmlSrv(fmt.Sprintf(`<html><body><main><p>%s</p></main></body></html>`, longText))
	defer srv.Close()

	f := fetcher.NewGoquery(200, nil)
	text, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(text) > 200 {
		t.Errorf("result should be truncated to 200 chars, got %d", len(text))
	}
}

func TestExtractJDMarkdown_TruncatesAtRuneBoundary(t *testing.T) {
	// "aaaÉbc" encodes as 7 bytes: a(1)+a(1)+a(1)+É(2)+b(1)+c(1).
	// The byte-slice truncate at maxChars=4 would yield "aaa\xc3" — cutting É mid-rune.
	// Rune-safe truncation must yield a valid UTF-8 string of at most 4 runes.
	html := `<html><body><main><p>aaaÉbc</p></main></body></html>`

	result := fetcher.ExtractJDMarkdown(html, 4)

	// Verify result is valid UTF-8 (would be corrupted if sliced mid-rune by byte index).
	if !utf8.ValidString(result) {
		t.Errorf("truncated result is invalid UTF-8: %q", result)
	}
	// Verify it was actually truncated (4 runes max).
	if utf8.RuneCountInString(result) > 4 {
		t.Errorf("expected at most 4 runes, got %d: %q", utf8.RuneCountInString(result), result)
	}
}
