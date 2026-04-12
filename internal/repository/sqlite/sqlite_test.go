package sqlite_test

import (
	"context"
	"testing"

	"github.com/thedandano/go-apply/internal/repository/sqlite"
)

func TestProfileRepository_UpsertAndFindSimilar(t *testing.T) {
	repo, err := sqlite.NewProfileRepository(":memory:", 3)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	vec1 := []float32{1.0, 0.0, 0.0}
	if err := repo.UpsertDocument(ctx, "doc:a", "document A", vec1); err != nil {
		t.Fatalf("UpsertDocument doc:a: %v", err)
	}

	vec2 := []float32{0.0, 1.0, 0.0}
	if err := repo.UpsertDocument(ctx, "doc:b", "document B", vec2); err != nil {
		t.Fatalf("UpsertDocument doc:b: %v", err)
	}

	// Query with a vector close to vec1
	query := []float32{0.9, 0.1, 0.0}
	results, err := repo.FindSimilar(ctx, query, 1)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].SourceDoc != "doc:a" {
		t.Errorf("expected doc:a to be most similar, got %q", results[0].SourceDoc)
	}
	if results[0].Term != "document A" {
		t.Errorf("expected term 'document A', got %q", results[0].Term)
	}
	if results[0].Weight <= 0 {
		t.Errorf("expected positive similarity weight, got %f", results[0].Weight)
	}
}

func TestProfileRepository_UpsertUpdatesExisting(t *testing.T) {
	repo, err := sqlite.NewProfileRepository(":memory:", 2)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	if err := repo.UpsertDocument(ctx, "doc:x", "original text", []float32{1.0, 0.0}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := repo.UpsertDocument(ctx, "doc:x", "updated text", []float32{0.0, 1.0}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// Query with vector close to updated [0, 1]
	results, err := repo.FindSimilar(ctx, []float32{0.1, 0.9}, 1)
	if err != nil {
		t.Fatalf("FindSimilar after upsert: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Term != "updated text" {
		t.Errorf("expected updated text, got %q", results[0].Term)
	}
}

func TestProfileRepository_FindSimilar_ReturnsTopK(t *testing.T) {
	repo, err := sqlite.NewProfileRepository(":memory:", 2)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	docs := []struct {
		source string
		text   string
		vec    []float32
	}{
		{"doc:1", "text one", []float32{1.0, 0.0}},
		{"doc:2", "text two", []float32{0.9, 0.1}},
		{"doc:3", "text three", []float32{0.0, 1.0}},
	}
	for _, d := range docs {
		if err := repo.UpsertDocument(ctx, d.source, d.text, d.vec); err != nil {
			t.Fatalf("upsert %s: %v", d.source, err)
		}
	}

	results, err := repo.FindSimilar(ctx, []float32{1.0, 0.0}, 2)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Top 2 should be doc:1 and doc:2 (both close to [1,0])
	sources := map[string]bool{results[0].SourceDoc: true, results[1].SourceDoc: true}
	if !sources["doc:1"] || !sources["doc:2"] {
		t.Errorf("expected doc:1 and doc:2 in top-2, got %q and %q", results[0].SourceDoc, results[1].SourceDoc)
	}
}

func TestProfileRepository_FindSimilar_EmptyDB(t *testing.T) {
	repo, err := sqlite.NewProfileRepository(":memory:", 3)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	results, err := repo.FindSimilar(context.Background(), []float32{1.0, 0.0, 0.0}, 5)
	if err != nil {
		t.Fatalf("FindSimilar on empty db: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}
