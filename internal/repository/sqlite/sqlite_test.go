//go:build integration

package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/repository/sqlite"
)

// TestProfileRepository_UpsertAndFindSimilar verifies the UpsertDocument + FindSimilar
// round-trip against a real SQLite + sqlite-vec database.
func TestProfileRepository_UpsertAndFindSimilar(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test-profile.db")
	const dim = 4

	repo, err := sqlite.NewProfileRepository(dbPath, dim)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Insert two documents with distinct embeddings.
	doc1Vector := []float32{1.0, 0.0, 0.0, 0.0}
	doc2Vector := []float32{0.0, 1.0, 0.0, 0.0}

	if err := repo.UpsertDocument(ctx, "resume:backend", "Led backend systems at scale", doc1Vector); err != nil {
		t.Fatalf("UpsertDocument doc1: %v", err)
	}
	if err := repo.UpsertDocument(ctx, "resume:frontend", "Built React-based dashboards", doc2Vector); err != nil {
		t.Fatalf("UpsertDocument doc2: %v", err)
	}

	// Query with a vector close to doc1 — expect doc1 first.
	queryVector := []float32{0.9, 0.1, 0.0, 0.0}
	results, err := repo.FindSimilar(ctx, queryVector, 2)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("FindSimilar returned no results")
	}

	// The first result should be the backend doc (closest to query).
	if results[0].SourceDoc != "resume:backend" {
		t.Errorf("expected first result to be resume:backend, got %q", results[0].SourceDoc)
	}
	if results[0].Term != "Led backend systems at scale" {
		t.Errorf("expected Term to be the chunk text, got %q", results[0].Term)
	}
	if results[0].Weight <= 0 || results[0].Weight > 1.0 {
		t.Errorf("expected Weight in (0, 1.0], got %f", results[0].Weight)
	}
}

// TestKeywordCache_SetAndGet verifies the keyword vector cache round-trip against
// a real SQLite database.
func TestKeywordCache_SetAndGet(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test-cache.db")
	const dim = 4

	repo, err := sqlite.NewProfileRepository(dbPath, dim)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	keyword := "golang kubernetes distributed"
	vector := []float32{0.1, 0.2, 0.3, 0.4}

	// Cache miss before any set.
	got, ok, err := repo.GetVector(ctx, keyword)
	if err != nil {
		t.Fatalf("GetVector (miss): %v", err)
	}
	if ok {
		t.Fatal("expected cache miss, got hit")
	}
	if got != nil {
		t.Errorf("expected nil vector on miss, got %v", got)
	}

	// Store then retrieve.
	if err := repo.SetVector(ctx, keyword, vector); err != nil {
		t.Fatalf("SetVector: %v", err)
	}

	got, ok, err = repo.GetVector(ctx, keyword)
	if err != nil {
		t.Fatalf("GetVector (hit): %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit after SetVector, got miss")
	}
	if len(got) != len(vector) {
		t.Fatalf("expected vector length %d, got %d", len(vector), len(got))
	}
	for i, v := range vector {
		if got[i] != v {
			t.Errorf("vector[%d]: expected %f, got %f", i, v, got[i])
		}
	}

	// Overwrite with new vector.
	newVector := []float32{0.9, 0.8, 0.7, 0.6}
	if err := repo.SetVector(ctx, keyword, newVector); err != nil {
		t.Fatalf("SetVector (overwrite): %v", err)
	}

	got, ok, err = repo.GetVector(ctx, keyword)
	if err != nil {
		t.Fatalf("GetVector (after overwrite): %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit after overwrite")
	}
	if got[0] != newVector[0] {
		t.Errorf("expected overwritten vector, got[0]=%f", got[0])
	}
}

// TestListDocuments_EmptyAndPopulated verifies that ListDocuments returns an empty
// slice when the database is empty and all stored documents when populated.
func TestListDocuments_EmptyAndPopulated(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test-list-docs.db")
	const dim = 4

	repo, err := sqlite.NewProfileRepository(dbPath, dim)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Empty case — no documents stored yet.
	docs, err := repo.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("ListDocuments (empty): %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 documents on empty db, got %d", len(docs))
	}

	// Insert two documents.
	vec := []float32{1.0, 0.0, 0.0, 0.0}
	if err := repo.UpsertDocument(ctx, "resume:backend", "Led backend systems at scale", vec); err != nil {
		t.Fatalf("UpsertDocument doc1: %v", err)
	}
	vec2 := []float32{0.0, 1.0, 0.0, 0.0}
	if err := repo.UpsertDocument(ctx, "resume:frontend", "Built React dashboards", vec2); err != nil {
		t.Fatalf("UpsertDocument doc2: %v", err)
	}

	// Populated case — both documents should be returned.
	docs, err = repo.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("ListDocuments (populated): %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	// Verify fields on returned documents.
	sourceSet := make(map[string]string)
	for _, d := range docs {
		sourceSet[d.Source] = d.Text
		if d.ID == 0 {
			t.Errorf("expected non-zero ID for document %q", d.Source)
		}
	}
	if text, ok := sourceSet["resume:backend"]; !ok {
		t.Error("expected resume:backend in results")
	} else if text != "Led backend systems at scale" {
		t.Errorf("unexpected text for resume:backend: %q", text)
	}
	if text, ok := sourceSet["resume:frontend"]; !ok {
		t.Error("expected resume:frontend in results")
	} else if text != "Built React dashboards" {
		t.Errorf("unexpected text for resume:frontend: %q", text)
	}
}

// TestProfileRepository_UpsertDocument_IdempotentUpdate verifies that upserting
// the same sourceDoc twice updates the text and embedding rather than creating a duplicate.
func TestProfileRepository_UpsertDocument_IdempotentUpdate(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "test-idempotent.db")
	const dim = 4

	repo, err := sqlite.NewProfileRepository(dbPath, dim)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	vec := []float32{1.0, 0.0, 0.0, 0.0}

	if err := repo.UpsertDocument(ctx, "resume:skills", "Python, Go, Kubernetes", vec); err != nil {
		t.Fatalf("first UpsertDocument: %v", err)
	}

	vec2 := []float32{0.5, 0.5, 0.0, 0.0}
	if err := repo.UpsertDocument(ctx, "resume:skills", "Go, Rust, WebAssembly", vec2); err != nil {
		t.Fatalf("second UpsertDocument (update): %v", err)
	}

	results, err := repo.FindSimilar(ctx, vec2, 5)
	if err != nil {
		t.Fatalf("FindSimilar after update: %v", err)
	}

	// Expect exactly one result (the upsert should not have duplicated the row).
	if len(results) != 1 {
		t.Errorf("expected 1 result after idempotent upsert, got %d", len(results))
	}
	if len(results) > 0 && results[0].Term != "Go, Rust, WebAssembly" {
		t.Errorf("expected updated chunk text, got %q", results[0].Term)
	}
}
