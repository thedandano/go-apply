//go:build integration

package fetcher_test

import "testing"

func TestChromedpFetcher_Integration(t *testing.T) {
	t.Skip("requires Chrome — run with -tags integration and real URL")
}
