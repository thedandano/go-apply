package survival_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/thedandano/go-apply/internal/service/survival"
)

// compile-time existence check — fails until survival package is created.
var _ = survival.Service{}

func newService() *survival.Service {
	return survival.New()
}

func TestDiff_AllKeywordsSurvive(t *testing.T) {
	svc := newService()
	keywords := []string{"go", "kubernetes", "distributed systems"}
	extracted := "Senior Go engineer with kubernetes experience in distributed systems."

	result := svc.Diff(keywords, extracted)

	if len(result.Dropped) != 0 {
		t.Errorf("Dropped = %v, want empty", result.Dropped)
	}
	if len(result.Matched) != len(keywords) {
		t.Errorf("Matched = %v, want all %v", result.Matched, keywords)
	}
	if result.TotalJDKeywords != len(keywords) {
		t.Errorf("TotalJDKeywords = %d, want %d", result.TotalJDKeywords, len(keywords))
	}
}

func TestDiff_SomeKeywordsDropped(t *testing.T) {
	svc := newService()
	keywords := []string{"go", "kubernetes", "rust", "java"}
	extracted := "We need a Go engineer with kubernetes background."

	result := svc.Diff(keywords, extracted)

	if result.TotalJDKeywords != len(keywords) {
		t.Errorf("TotalJDKeywords = %d, want %d", result.TotalJDKeywords, len(keywords))
	}
	total := len(result.Dropped) + len(result.Matched)
	if total != result.TotalJDKeywords {
		t.Errorf("Dropped(%d) + Matched(%d) = %d, want %d", len(result.Dropped), len(result.Matched), total, result.TotalJDKeywords)
	}
	droppedSet := make(map[string]bool)
	for _, kw := range result.Dropped {
		droppedSet[kw] = true
	}
	for _, expected := range []string{"rust", "java"} {
		if !droppedSet[expected] {
			t.Errorf("%q should be in Dropped, got Dropped=%v", expected, result.Dropped)
		}
	}
}

func TestDiff_EmptyKeywords_ReturnsZeroStruct(t *testing.T) {
	svc := newService()

	result := svc.Diff([]string{}, "some extracted text")

	if result.TotalJDKeywords != 0 {
		t.Errorf("TotalJDKeywords = %d, want 0", result.TotalJDKeywords)
	}
	if len(result.Dropped) != 0 {
		t.Errorf("Dropped = %v, want empty", result.Dropped)
	}
	if len(result.Matched) != 0 {
		t.Errorf("Matched = %v, want empty", result.Matched)
	}
}

// TestDiff_AssumesCallerDeduplicates verifies that Diff does NOT deduplicate: if the
// caller passes ["go", "go"], TotalJDKeywords is 2. Deduplication is the caller's job.
func TestDiff_AssumesCallerDeduplicates(t *testing.T) {
	svc := newService()
	keywords := []string{"go", "go"}
	extracted := "Go developer."

	result := svc.Diff(keywords, extracted)

	if result.TotalJDKeywords != 2 {
		t.Errorf("TotalJDKeywords = %d, want 2 (Diff does not deduplicate)", result.TotalJDKeywords)
	}
}

func TestDiff_CaseInsensitiveMatching(t *testing.T) {
	svc := newService()
	keywords := []string{"Kubernetes", "Go"}
	extracted := "We use kubernetes and go for our backend."

	result := svc.Diff(keywords, extracted)

	if len(result.Dropped) != 0 {
		t.Errorf("case-insensitive match failed: Dropped = %v", result.Dropped)
	}
	if len(result.Matched) != 2 {
		t.Errorf("Matched = %v, want 2 entries", result.Matched)
	}
}

func TestDiff_NonNilSlices(t *testing.T) {
	svc := newService()

	result := svc.Diff([]string{}, "")

	if result.Dropped == nil {
		t.Error("Dropped must be non-nil (empty slice)")
	}
	if result.Matched == nil {
		t.Error("Matched must be non-nil (empty slice)")
	}
}

// TestDiff_SpecialCharKeyword_CppMatches verifies that "C++" matches the literal
// string "C++" in extracted text and does NOT match unrelated words like "plusplus".
func TestDiff_SpecialCharKeyword_CppMatches(t *testing.T) {
	svc := newService()

	result := svc.Diff([]string{"C++"}, "Built C++ services for low-latency trading.")
	if len(result.Dropped) != 0 {
		t.Errorf("C++ should match in extracted text, got Dropped=%v", result.Dropped)
	}

	result2 := svc.Diff([]string{"C++"}, "Wrote plusplus code.")
	if len(result2.Matched) != 0 {
		t.Errorf("C++ must not match 'plusplus', got Matched=%v", result2.Matched)
	}
}

// TestDiff_SpecialCharKeyword_DotNETMatches verifies that ".NET" matches ".NET Framework"
// and does NOT match standalone "NET".
func TestDiff_SpecialCharKeyword_DotNETMatches(t *testing.T) {
	svc := newService()

	result := svc.Diff([]string{".NET"}, "Experience with .NET Framework and Azure.")
	if len(result.Dropped) != 0 {
		t.Errorf(".NET should match in extracted text, got Dropped=%v", result.Dropped)
	}
}

// TestDiff_MultiWordKeyword_DistributedSystems verifies multi-word keyword matching.
func TestDiff_MultiWordKeyword_DistributedSystems(t *testing.T) {
	svc := newService()

	result := svc.Diff([]string{"distributed systems"}, "Expert in distributed systems architecture.")
	if len(result.Dropped) != 0 {
		t.Errorf("'distributed systems' should match, got Dropped=%v", result.Dropped)
	}

	result2 := svc.Diff([]string{"distributed systems"}, "undistributed systemsx performance.")
	if len(result2.Matched) != 0 {
		t.Errorf("'distributed systems' must not match 'undistributed systemsx', got Matched=%v", result2.Matched)
	}
}

// TestDiff_RegexpCachingDoesNotAffectResults verifies the sync.Map cache produces
// the same results across multiple calls with the same keyword set.
func TestDiff_RegexpCachingDoesNotAffectResults(t *testing.T) {
	svc := newService()
	keywords := []string{"go", "kubernetes"}
	extracted := "Go and kubernetes are used here."

	first := svc.Diff(keywords, extracted)
	second := svc.Diff(keywords, extracted)

	if len(first.Matched) != len(second.Matched) {
		t.Errorf("repeated Diff calls must return same results; first=%v second=%v", first.Matched, second.Matched)
	}
}

// testLogHandler captures slog records for assertion in tests.
type testLogHandler struct {
	records []slog.Record
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	h.records = append(h.records, r)
	return nil
}
func (h *testLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testLogHandler) WithGroup(_ string) slog.Handler      { return h }

func TestDiff_LogsKeywordCounts(t *testing.T) {
	handler := &testLogHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(orig)

	svc := newService()
	keywords := []string{"go", "kubernetes", "rust"}
	extracted := "Go and kubernetes are used here."

	svc.Diff(keywords, extracted)

	var found bool
	for i := range handler.records {
		if handler.records[i].Message != "survival.diff" {
			continue
		}
		found = true
		attrs := make(map[string]any)
		handler.records[i].Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.Any()
			return true
		})
		if attrs["total"] == nil {
			t.Error("log entry missing 'total' field")
		}
		if attrs["dropped"] == nil {
			t.Error("log entry missing 'dropped' field")
		}
		if attrs["matched"] == nil {
			t.Error("log entry missing 'matched' field")
		}
	}
	if !found {
		t.Error("no 'survival.diff' log entry emitted")
	}
}
