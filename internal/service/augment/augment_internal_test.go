package augment

import (
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

// TestFilterCandidates_EmptyInput returns empty chunks and matched list
func TestFilterCandidates_EmptyInput(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{}
	seen := make(map[string]bool)
	threshold := 0.6

	chunks, matched := filterCandidates(candidates, threshold, seen)

	if len(chunks) != 0 {
		t.Errorf("expected empty chunks, got %d", len(chunks))
	}
	if len(matched) != 0 {
		t.Errorf("expected empty matched list, got %d", len(matched))
	}
}

// TestFilterCandidates_AllBelowThreshold returns empty results
func TestFilterCandidates_AllBelowThreshold(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{
		{ID: 1, SourceDoc: "resume:a", Term: "golang", Weight: 0.3},
		{ID: 2, SourceDoc: "resume:b", Term: "rust", Weight: 0.4},
	}
	seen := make(map[string]bool)
	threshold := 0.6

	chunks, matched := filterCandidates(candidates, threshold, seen)

	if len(chunks) != 0 {
		t.Errorf("expected empty chunks when all below threshold, got %d", len(chunks))
	}
	if len(matched) != 0 {
		t.Errorf("expected empty matched list when all below threshold, got %d", len(matched))
	}
}

// TestFilterCandidates_AllAboveThreshold returns all candidates as chunks
func TestFilterCandidates_AllAboveThreshold(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{
		{ID: 1, SourceDoc: "resume:a", Term: "golang", Weight: 0.8},
		{ID: 2, SourceDoc: "resume:b", Term: "rust", Weight: 0.9},
	}
	seen := make(map[string]bool)
	threshold := 0.6

	chunks, matched := filterCandidates(candidates, threshold, seen)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched terms, got %d", len(matched))
	}
	if chunks[0].Source != "resume:a" || chunks[0].Text != "golang" {
		t.Errorf("expected first chunk to match candidate 1, got %v", chunks[0])
	}
	if chunks[1].Source != "resume:b" || chunks[1].Text != "rust" {
		t.Errorf("expected second chunk to match candidate 2, got %v", chunks[1])
	}
	if matched[0] != "golang" || matched[1] != "rust" {
		t.Errorf("expected matched terms in order, got %v", matched)
	}
}

// TestFilterCandidates_DeduplicatesBySource returns one chunk per source
func TestFilterCandidates_DeduplicatesBySource(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{
		{ID: 1, SourceDoc: "resume:a", Term: "golang", Weight: 0.8},
		{ID: 2, SourceDoc: "resume:a", Term: "concurrency", Weight: 0.85},
		{ID: 3, SourceDoc: "resume:b", Term: "rust", Weight: 0.9},
	}
	seen := make(map[string]bool)
	threshold := 0.6

	chunks, matched := filterCandidates(candidates, threshold, seen)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (deduped by source), got %d", len(chunks))
	}
	if len(matched) != 3 {
		t.Errorf("expected 3 matched terms (no dedup), got %d", len(matched))
	}
	// First candidate from resume:a should be included
	if chunks[0].Source != "resume:a" {
		t.Errorf("expected first chunk from resume:a, got %s", chunks[0].Source)
	}
	// Second candidate from resume:a should be skipped
	if chunks[1].Source != "resume:b" {
		t.Errorf("expected second chunk from resume:b, got %s", chunks[1].Source)
	}
}

// TestFilterCandidates_MutatesSeen updates the seen map
func TestFilterCandidates_MutatesSeen(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{
		{ID: 1, SourceDoc: "resume:a", Term: "golang", Weight: 0.8},
		{ID: 2, SourceDoc: "resume:b", Term: "rust", Weight: 0.9},
	}
	seen := make(map[string]bool)
	threshold := 0.6

	filterCandidates(candidates, threshold, seen)

	if !seen["resume:a"] {
		t.Error("expected resume:a marked as seen")
	}
	if !seen["resume:b"] {
		t.Error("expected resume:b marked as seen")
	}
	if len(seen) != 2 {
		t.Errorf("expected exactly 2 entries in seen, got %d", len(seen))
	}
}

// TestFilterCandidates_AccumulatesCandidatesWithExistingSeen filters correctly
func TestFilterCandidates_AccumulatesCandidatesWithExistingSeen(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{
		{ID: 1, SourceDoc: "resume:a", Term: "golang", Weight: 0.8},
		{ID: 2, SourceDoc: "resume:b", Term: "rust", Weight: 0.9},
		{ID: 3, SourceDoc: "resume:c", Term: "python", Weight: 0.85},
	}
	seen := make(map[string]bool)
	seen["resume:a"] = true // resume:a already seen

	threshold := 0.6

	chunks, matched := filterCandidates(candidates, threshold, seen)

	if len(chunks) != 2 {
		t.Errorf("expected 2 new chunks (resume:a skipped), got %d", len(chunks))
	}
	if len(matched) != 3 {
		t.Errorf("expected 3 matched terms (no dedup), got %d", len(matched))
	}
	// resume:a should be skipped even though it's above threshold
	if chunks[0].Source != "resume:b" {
		t.Errorf("expected first chunk from resume:b (resume:a was pre-seen), got %s", chunks[0].Source)
	}
	if chunks[1].Source != "resume:c" {
		t.Errorf("expected second chunk from resume:c, got %s", chunks[1].Source)
	}
}

// TestFilterCandidates_ThresholdAtBoundary includes exactly at threshold
func TestFilterCandidates_ThresholdAtBoundary(t *testing.T) {
	t.Parallel()

	candidates := []model.ProfileEmbedding{
		{ID: 1, SourceDoc: "resume:a", Term: "golang", Weight: 0.6},   // exactly at threshold
		{ID: 2, SourceDoc: "resume:b", Term: "rust", Weight: 0.599},   // just below threshold
		{ID: 3, SourceDoc: "resume:c", Term: "python", Weight: 0.601}, // just above threshold
	}
	seen := make(map[string]bool)
	threshold := 0.6

	chunks, matched := filterCandidates(candidates, threshold, seen)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks (at-threshold and above included), got %d", len(chunks))
	}
	if len(matched) != 2 {
		t.Errorf("expected 2 matched terms, got %d", len(matched))
	}
	// verify which ones made it
	sources := map[string]bool{}
	for _, chunk := range chunks {
		sources[chunk.Source] = true
	}
	if !sources["resume:a"] {
		t.Error("expected resume:a (at threshold) to be included")
	}
	if sources["resume:b"] {
		t.Error("expected resume:b (below threshold) to be excluded")
	}
	if !sources["resume:c"] {
		t.Error("expected resume:c (above threshold) to be included")
	}
}
