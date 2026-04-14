// Package fetcher provides JD fetcher implementations.
// ChromedpFetcher uses a headless browser for JS-rendered pages.
// GoqueryFetcher is a lightweight HTTP + HTML parser fallback.
// FallbackFetcher tries the primary and falls back to secondary when it fails
// or returns fewer than MinJDTextLengthChars characters.
package fetcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction checks.
var _ port.JDFetcher = (*ChromedpFetcher)(nil)
var _ port.JDFetcher = (*GoqueryFetcher)(nil)
var _ port.JDFetcher = (*FallbackFetcher)(nil)

// ChromedpFetcher fetches JD text using a headless browser.
// It handles JavaScript-rendered pages that GoqueryFetcher cannot parse.
type ChromedpFetcher struct {
	timeoutMS int
	log       *slog.Logger
}

// New constructs a ChromedpFetcher. Timeout comes from defaults.Fetcher.ChromedpTimeoutMS.
func New(defaults *config.AppDefaults, log *slog.Logger) *ChromedpFetcher {
	if log == nil {
		log = slog.Default()
	}
	return &ChromedpFetcher{timeoutMS: defaults.Fetcher.ChromedpTimeoutMS, log: log}
}

// Fetch navigates to url with a headless browser and returns the visible body text.
func (f *ChromedpFetcher) Fetch(ctx context.Context, url string) (string, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()
	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx)
	defer chromeCancel()
	timeout := time.Duration(f.timeoutMS) * time.Millisecond
	timeoutCtx, timeoutCancel := context.WithTimeout(chromeCtx, timeout)
	defer timeoutCancel()

	var body string
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.InnerHTML("body", &body),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp fetch %s: %w", url, err)
	}
	f.log.DebugContext(ctx, "fetcher: chromedp fetched page", "url", url, "bytes", len(body))
	return extractTextFromHTML(body), nil
}

// GoqueryFetcher fetches JD text using a plain HTTP GET + HTML parsing.
// It does not execute JavaScript, making it fast but unsuitable for SPA pages.
type GoqueryFetcher struct {
	http *http.Client
	log  *slog.Logger
}

// NewGoquery constructs a GoqueryFetcher. log may be nil — slog.Default() is used then.
func NewGoquery(log *slog.Logger) *GoqueryFetcher {
	if log == nil {
		log = slog.Default()
	}
	return &GoqueryFetcher{http: &http.Client{}, log: log}
}

// Fetch issues a GET request to url and returns the visible body text.
func (f *GoqueryFetcher) Fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("goquery fetch %s: create request: %w", url, err)
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("goquery fetch %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("goquery parse %s: %w", url, err)
	}
	doc.Find("script,style,nav,header,footer").Remove()
	text := strings.TrimSpace(doc.Find("body").Text())
	f.log.DebugContext(ctx, "fetcher: goquery fetched page", "url", url, "chars", len(text))
	return text, nil
}

// FallbackFetcher tries the primary fetcher first. If it fails or returns fewer than
// minJDTextLengthChars characters, it logs a warning and falls back to the secondary.
type FallbackFetcher struct {
	primary              port.JDFetcher
	fallback             port.JDFetcher
	minJDTextLengthChars int
	log                  *slog.Logger
}

// NewFallback constructs a FallbackFetcher using ChromedpFetcher as primary
// and GoqueryFetcher as fallback. log may be nil — slog.Default() is used then.
func NewFallback(defaults *config.AppDefaults, log *slog.Logger) *FallbackFetcher {
	if log == nil {
		log = slog.Default()
	}
	return &FallbackFetcher{
		primary:              New(defaults, log),
		fallback:             NewGoquery(log),
		minJDTextLengthChars: defaults.Fetcher.MinJDTextLengthChars,
		log:                  log,
	}
}

// NewFallbackWith constructs a FallbackFetcher with explicit primary and fallback implementations.
// Intended for dependency injection and testing.
func NewFallbackWith(primary, fallback port.JDFetcher, minChars int, log *slog.Logger) *FallbackFetcher {
	if log == nil {
		log = slog.Default()
	}
	return &FallbackFetcher{
		primary:              primary,
		fallback:             fallback,
		minJDTextLengthChars: minChars,
		log:                  log,
	}
}

// Fetch tries the primary fetcher. If it errors or the result is shorter than
// minJDTextLengthChars, it logs a warning and falls through to the fallback.
func (f *FallbackFetcher) Fetch(ctx context.Context, url string) (string, error) {
	text, err := f.primary.Fetch(ctx, url)
	if err != nil {
		f.log.WarnContext(ctx, "fetcher: primary failed, falling back to goquery",
			"url", url, "error", err)
		return f.fallback.Fetch(ctx, url)
	}
	if len(strings.TrimSpace(text)) < f.minJDTextLengthChars {
		f.log.WarnContext(ctx, "fetcher: primary returned thin content, falling back",
			"url", url, "chars", len(strings.TrimSpace(text)), "min", f.minJDTextLengthChars)
		return f.fallback.Fetch(ctx, url)
	}
	return text, nil
}

// extractTextFromHTML parses an HTML string and returns visible body text
// with scripts, styles, nav, headers, and footers removed.
func extractTextFromHTML(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}
	doc.Find("script,style,nav,header,footer").Remove()
	return strings.TrimSpace(doc.Find("body").Text())
}
