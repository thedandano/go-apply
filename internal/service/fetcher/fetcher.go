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

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
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
	maxChars  int
	log       *slog.Logger
}

// New constructs a ChromedpFetcher. Timeout comes from defaults.Fetcher.ChromedpTimeoutMS.
func New(defaults *config.AppDefaults, log *slog.Logger) *ChromedpFetcher {
	if log == nil {
		log = slog.Default()
	}
	return &ChromedpFetcher{
		timeoutMS: defaults.Fetcher.ChromedpTimeoutMS,
		maxChars:  defaults.Fetcher.MaxJDTextLengthChars,
		log:       log,
	}
}

// Fetch navigates to url with a headless browser and returns the visible body text.
func (f *ChromedpFetcher) Fetch(ctx context.Context, url string) (string, error) {
	start := time.Now()
	f.log.DebugContext(ctx, "fetcher: chromedp fetch start", "url", url)
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

	// jobContentSelectors is the ordered list of CSS selectors to wait for
	// before capturing the page body. We try each one; if none appear within
	// the timeout we fall back to capturing whatever is in the body.
	jobContentSelectors := []string{
		"main",
		"article",
		"[role='main']",
		"[class*='job-description']",
		"[class*='job-detail']",
		"[class*='jobDescription']",
		"[class*='description']",
	}

	var body string
	actions := []chromedp.Action{
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		// Give JS-rendered pages 2 s to load their content before capturing.
		chromedp.Sleep(2 * time.Second),
	}

	// Best-effort: wait for a known job-content container to appear.
	// Failures are silently ignored — we still capture whatever is in body.
	for _, sel := range jobContentSelectors {
		sel := sel
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			_ = chromedp.WaitVisible(sel, chromedp.ByQuery).Do(waitCtx)
			return nil
		}))
		break // try only the first selector; break after one attempt
	}

	actions = append(actions, chromedp.InnerHTML("body", &body))

	err := chromedp.Run(timeoutCtx, actions...)
	if err != nil {
		return "", fmt.Errorf("chromedp fetch %s: %w", url, err)
	}
	text := ExtractJDMarkdown(body, f.maxChars)
	f.log.DebugContext(ctx, "fetcher: chromedp fetch end",
		"url", url,
		"response_bytes", len(text),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return text, nil
}

// GoqueryFetcher fetches JD text using a plain HTTP GET + HTML parsing.
// It does not execute JavaScript, making it fast but unsuitable for SPA pages.
type GoqueryFetcher struct {
	http     *http.Client
	maxChars int
	log      *slog.Logger
}

// NewGoquery constructs a GoqueryFetcher. maxChars is the max length of the returned
// Markdown string (0 = no limit). log may be nil — slog.Default() is used then.
func NewGoquery(maxChars int, log *slog.Logger) *GoqueryFetcher {
	if log == nil {
		log = slog.Default()
	}
	return &GoqueryFetcher{http: &http.Client{}, maxChars: maxChars, log: log}
}

// Fetch issues a GET request to url and returns the visible body text.
func (f *GoqueryFetcher) Fetch(ctx context.Context, url string) (string, error) {
	start := time.Now()
	f.log.DebugContext(ctx, "fetcher: goquery fetch start", "url", url)
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
	bodyHTML, err := doc.Find("body").Html()
	if err != nil {
		return "", fmt.Errorf("goquery extract body %s: %w", url, err)
	}
	text := ExtractJDMarkdown(bodyHTML, f.maxChars)
	f.log.DebugContext(ctx, "fetcher: goquery fetch end",
		"url", url,
		"status_code", resp.StatusCode,
		"response_bytes", len(text),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
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
		fallback:             NewGoquery(defaults.Fetcher.MaxJDTextLengthChars, log),
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
		f.log.DebugContext(ctx, "decision",
			slog.String("name", "fetcher.source"),
			slog.String("chosen", "fallback"),
			slog.String("reason", "primary fetch error"),
			slog.String("url", url),
		)
		f.log.WarnContext(ctx, "fetcher: primary failed, falling back to goquery",
			"url", url, "error", err)
		return f.fallback.Fetch(ctx, url)
	}
	if len(strings.TrimSpace(text)) < f.minJDTextLengthChars {
		f.log.DebugContext(ctx, "decision",
			slog.String("name", "fetcher.source"),
			slog.String("chosen", "fallback"),
			slog.String("reason", "primary returned thin content"),
			slog.String("url", url),
			slog.Int("chars", len(strings.TrimSpace(text))),
			slog.Int("min", f.minJDTextLengthChars),
		)
		f.log.WarnContext(ctx, "fetcher: primary returned thin content, falling back",
			"url", url, "chars", len(strings.TrimSpace(text)), "min", f.minJDTextLengthChars)
		return f.fallback.Fetch(ctx, url)
	}
	f.log.DebugContext(ctx, "decision",
		slog.String("name", "fetcher.source"),
		slog.String("chosen", "network"),
		slog.String("reason", "primary fetch succeeded"),
		slog.String("url", url),
	)
	return text, nil
}

// contentSelectors is the priority-ordered list of CSS selectors used to scope
// HTML to the most likely job-description container before converting to Markdown.
// The first selector that matches a non-empty element wins; body is the final fallback.
var contentSelectors = []string{
	"main",
	"article",
	"[role='main']",
	"#content",
	"[class*='job-description']",
	"[class*='job-detail']",
	"[class*='jobDescription']",
	"[class*='description']",
}

// ExtractJDMarkdown scopes raw HTML to the most specific job-description container,
// converts it to Markdown, and truncates the result to maxChars.
// If maxChars <= 0, no truncation is applied.
// Exported so fetcher_test.go (package fetcher_test) can call it directly.
func ExtractJDMarkdown(htmlStr string, maxChars int) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return truncate(htmlStr, maxChars)
	}

	// Remove noise unconditionally before scoping.
	doc.Find("script,style,nav,header,footer,aside").Remove()

	// Find the most specific semantic container.
	var scopedHTML string
	for _, sel := range contentSelectors {
		node := doc.Find(sel).First()
		if node.Length() == 0 {
			continue
		}
		h, err := node.Html()
		if err == nil && strings.TrimSpace(h) != "" {
			scopedHTML = h
			break
		}
	}

	// Fall back to full body if no scoped container found.
	if scopedHTML == "" {
		h, err := doc.Find("body").Html()
		if err != nil || strings.TrimSpace(h) == "" {
			return truncate(strings.TrimSpace(doc.Text()), maxChars)
		}
		scopedHTML = h
	}

	markdown, err := htmltomarkdown.ConvertString(scopedHTML)
	if err != nil {
		// Fallback: plain text from scoped node.
		return truncate(strings.TrimSpace(doc.Find("body").Text()), maxChars)
	}

	return truncate(strings.TrimSpace(markdown), maxChars)
}

// truncate returns s truncated to maxChars Unicode code points (runes).
// If maxChars <= 0, s is returned unchanged.
func truncate(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	i := 0
	for j := range s {
		if i == maxChars {
			return s[:j]
		}
		i++
	}
	return s
}
