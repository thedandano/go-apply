package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	_ "modernc.org/sqlite" // register "sqlite" driver for database/sql

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// ProfileRepository stores document embeddings in SQLite and performs
// Go-side cosine similarity search. Uses modernc.org/sqlite (pure-Go,
// no CGO) for cross-platform builds.
type ProfileRepository struct {
	db *sql.DB
}

var _ port.ProfileRepository = (*ProfileRepository)(nil)

// NewProfileRepository opens (or creates) the SQLite database at dbPath
// and initialises the schema. embeddingDim is recorded for future use
// (e.g. schema migrations) but not yet stored in the schema.
// Use ":memory:" for dbPath in tests.
func NewProfileRepository(dbPath string, _ int) (*ProfileRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db %s: %w", dbPath, err)
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("configure pragmas: %w; close db: %v", err, closeErr)
		}
		return nil, fmt.Errorf("configure pragmas: %w", err)
	}
	if _, err = db.Exec(schemaSQL); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("init schema: %w; close db: %v", err, closeErr)
		}
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &ProfileRepository{db: db}, nil
}

// Close releases the underlying database connection.
func (r *ProfileRepository) Close() error {
	return r.db.Close()
}

// UpsertDocument stores or updates the embedding for a named source document.
func (r *ProfileRepository) UpsertDocument(ctx context.Context, sourceDoc string, text string, vector []float32) error {
	blob := serializeFloat32(vector)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO profile_docs(source, chunk, embedding)
		VALUES(?, ?, ?)
		ON CONFLICT(source) DO UPDATE SET
			chunk      = excluded.chunk,
			embedding  = excluded.embedding,
			updated_at = CURRENT_TIMESTAMP
	`, sourceDoc, text, blob)
	if err != nil {
		return fmt.Errorf("upsert document %q: %w", sourceDoc, err)
	}
	return nil
}

// FindSimilar returns the top-k most similar documents to queryVector
// using cosine similarity computed in Go.
func (r *ProfileRepository) FindSimilar(ctx context.Context, queryVector []float32, k int) ([]model.ProfileEmbedding, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, source, chunk, embedding FROM profile_docs`)
	if err != nil {
		return nil, fmt.Errorf("query profile docs: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id        int64
		source    string
		chunk     string
		embedding []float32
	}

	var candidates []candidate
	for rows.Next() {
		var (
			id    int64
			src   string
			chunk string
			blob  []byte
		)
		if err := rows.Scan(&id, &src, &chunk, &blob); err != nil {
			return nil, fmt.Errorf("scan profile doc: %w", err)
		}
		vec, err := deserializeFloat32(blob)
		if err != nil {
			return nil, fmt.Errorf("deserialize embedding for %q: %w", src, err)
		}
		candidates = append(candidates, candidate{id, src, chunk, vec})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile docs: %w", err)
	}

	type scored struct {
		cand       candidate
		similarity float64
	}

	results := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		sim := cosineSimilarity(queryVector, c.embedding)
		results = append(results, scored{c, sim})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].similarity > results[j].similarity
	})

	if k < len(results) {
		results = results[:k]
	}

	out := make([]model.ProfileEmbedding, len(results))
	for i, r := range results {
		out[i] = model.ProfileEmbedding{
			ID:        r.cand.id,
			SourceDoc: r.cand.source,
			Term:      r.cand.chunk,
			Weight:    r.similarity,
		}
	}
	return out, nil
}

// serializeFloat32 converts a float32 slice to a little-endian byte slice.
func serializeFloat32(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// deserializeFloat32 converts a little-endian byte slice back to float32 slice.
func deserializeFloat32(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("blob length %d is not a multiple of 4", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v, nil
}

// cosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
