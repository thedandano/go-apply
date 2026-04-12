// Package fetcher provides implementations of port.JDFetcher.
//
// GoqueryFetcher uses a plain HTTP GET and parses the HTML body via goquery.
// ChromedpFetcher uses a headless browser for JavaScript-rendered pages.
// FallbackFetcher tries ChromedpFetcher first and falls back to GoqueryFetcher.
package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// ─── GoqueryFetcher ──────────────────────────────────────────────────────────

var _ port.JDFetcher = (*GoqueryFetcher)(nil)

// GoqueryFetcher fetches a URL with a plain HTTP GET and extracts body text.
type GoqueryFetcher struct {
	defaults *config.AppDefaults
	client   *http.Client
}

// NewGoqueryFetcher creates a GoqueryFetcher.
func NewGoqueryFetcher(defaults *config.AppDefaults) *GoqueryFetcher {
	return &GoqueryFetcher{
		defaults: defaults,
		client:   &http.Client{Timeout: time.Duration(defaults.LLM.TimeoutMS) * time.Millisecond},
	}
}

// Fetch downloads url, strips script/style/nav/footer/header nodes,
// and returns cleaned body text. Returns an error if the result is shorter
// than defaults.Fetcher.MinJDTextLengthChars.
func (f *GoqueryFetcher) Fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("goquery fetch %s: build request: %w", url, err)
	}
	req.Header.Set("User-Agent", "go-apply/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("goquery fetch %s: http get: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("goquery fetch %s: status %d", url, resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("goquery fetch %s: parse html: %w", url, err)
	}

	// Remove noise nodes before text extraction.
	doc.Find("script, style, nav, footer, header").Remove()

	text := extractText(doc.Find("body"))
	text = collapseWhitespace(text)

	if len(text) < f.defaults.Fetcher.MinJDTextLengthChars {
		return "", fmt.Errorf("goquery fetch %s: content too short (%d chars)", url, len(text))
	}
	return text, nil
}

// extractText collects visible text from a goquery selection, inserting a
// space between block-level elements so words don't run together.
func extractText(sel *goquery.Selection) string {
	var b strings.Builder
	sel.Find("*").Each(func(_ int, s *goquery.Selection) {
		// Only emit text nodes (leaf-level text).
		s.Contents().Each(func(_ int, c *goquery.Selection) {
			if goquery.NodeName(c) == "#text" {
				t := strings.TrimSpace(c.Text())
				if t != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(t)
				}
			}
		})
	})
	return b.String()
}

var multiSpace = regexp.MustCompile(`\s+`)

func collapseWhitespace(s string) string {
	return strings.TrimSpace(multiSpace.ReplaceAllString(s, " "))
}

// ─── ChromedpFetcher ─────────────────────────────────────────────────────────

var _ port.JDFetcher = (*ChromedpFetcher)(nil)

// ChromedpFetcher fetches a URL using a headless browser, suitable for
// JavaScript-rendered pages.
type ChromedpFetcher struct {
	defaults *config.AppDefaults
}

// NewChromedpFetcher creates a ChromedpFetcher.
func NewChromedpFetcher(defaults *config.AppDefaults) *ChromedpFetcher {
	return &ChromedpFetcher{defaults: defaults}
}

// Fetch navigates to url in a headless browser, waits for the document to be
// ready, and returns document.body.innerText. Returns an error if the text is
// shorter than defaults.Fetcher.MinJDTextLengthChars.
func (f *ChromedpFetcher) Fetch(ctx context.Context, url string) (string, error) {
	timeout := time.Duration(f.defaults.Fetcher.ChromedpTimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, chromedp.DefaultExecAllocatorOptions[:]...)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	var bodyText string
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(`document.readyState`, nil),
		chromedp.Evaluate(`document.body.innerText`, &bodyText),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp fetch %s: %w", url, err)
	}

	text := collapseWhitespace(bodyText)
	if len(text) < f.defaults.Fetcher.MinJDTextLengthChars {
		return "", fmt.Errorf("chromedp fetch %s: content too short (%d chars)", url, len(text))
	}
	return text, nil
}

// ─── FallbackFetcher ─────────────────────────────────────────────────────────

var _ port.JDFetcher = (*FallbackFetcher)(nil)

// FallbackFetcher tries a primary JDFetcher first, then falls back to a secondary.
// By default constructed with ChromedpFetcher as primary and GoqueryFetcher as fallback.
type FallbackFetcher struct {
	primary   port.JDFetcher
	secondary port.JDFetcher
}

// NewFallbackFetcher creates a FallbackFetcher backed by chromedp (primary) and
// goquery (fallback).
func NewFallbackFetcher(defaults *config.AppDefaults) *FallbackFetcher {
	return &FallbackFetcher{
		primary:   NewChromedpFetcher(defaults),
		secondary: NewGoqueryFetcher(defaults),
	}
}

// Fetch tries the primary fetcher. If it returns an error, falls back to the
// secondary. Both fetchers enforce their own MinJDTextLengthChars check internally.
func (f *FallbackFetcher) Fetch(ctx context.Context, url string) (string, error) {
	text, err := f.primary.Fetch(ctx, url)
	if err != nil {
		return f.secondary.Fetch(ctx, url)
	}
	return text, nil
}
