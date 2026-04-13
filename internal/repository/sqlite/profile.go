package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"

	// modernc.org/sqlite/vec registers sqlite-vec into the "sqlite" driver's auto_extension list.
	// This is a pure-Go (no CGO) integration — no C compiler required.
	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.ProfileRepository = (*ProfileRepository)(nil)

// ProfileRepository stores resume/profile document chunks alongside their embedding vectors
// in a local SQLite database using the sqlite-vec virtual table extension.
type ProfileRepository struct {
	db *sql.DB
}

// NewProfileRepository opens (or creates) the SQLite database at dbPath, runs schema
// migrations, and returns a ready-to-use ProfileRepository.
// dim is the embedding vector dimension used to create the virtual table on first run.
func NewProfileRepository(dbPath string, dim int) (*ProfileRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db %s: %w", dbPath, err)
	}

	// SQLite performs best with a single writer connection; WAL mode improves concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL journal mode: %w", err)
	}

	if err := migrate(db, dim); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate profile db: %w", err)
	}

	return &ProfileRepository{db: db}, nil
}

// migrate runs all schema creation statements in order.
func migrate(db *sql.DB, dim int) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("create profile_docs table: %w", err)
	}
	if _, err := db.Exec(vecTableSQL(dim)); err != nil {
		return fmt.Errorf("create profile_embeddings virtual table: %w", err)
	}
	return nil
}

// UpsertDocument stores or replaces the document text and its embedding vector.
// It runs inside a transaction: upserts profile_docs (returning the row id), then
// deletes+inserts the embedding row in profile_embeddings (vec0 does not support
// ON CONFLICT, so we must delete first).
func (r *ProfileRepository) UpsertDocument(ctx context.Context, sourceDoc string, text string, vector []float32) error {
	blob, err := serializeFloat32(vector)
	if err != nil {
		return fmt.Errorf("serialize embedding vector: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var docID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO profile_docs(source, chunk)
		VALUES(?, ?)
		ON CONFLICT(source) DO UPDATE SET chunk=excluded.chunk, updated_at=CURRENT_TIMESTAMP
		RETURNING id
	`, sourceDoc, text).Scan(&docID)
	if err != nil {
		return fmt.Errorf("upsert profile_docs: %w", err)
	}

	// vec0 virtual tables do not support ON CONFLICT — delete first, then insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM profile_embeddings WHERE doc_id = ?`, docID); err != nil {
		return fmt.Errorf("delete old embedding: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO profile_embeddings(doc_id, embedding) VALUES(?, ?)`, docID, blob); err != nil {
		return fmt.Errorf("insert embedding: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert transaction: %w", err)
	}
	return nil
}

// FindSimilar returns the top-k profile document chunks most similar to queryVector
// using cosine distance. Weight is set to 1.0 - distance so that higher values mean
// more similar.
func (r *ProfileRepository) FindSimilar(ctx context.Context, queryVector []float32, k int) ([]model.ProfileEmbedding, error) {
	blob, err := serializeFloat32(queryVector)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT pd.id, pd.source, pd.chunk, pe.distance
		FROM profile_embeddings pe
		JOIN profile_docs pd ON pe.doc_id = pd.id
		WHERE pe.embedding MATCH ?
		  AND k = ?
		ORDER BY pe.distance
	`, blob, k)
	if err != nil {
		return nil, fmt.Errorf("find similar embeddings: %w", err)
	}
	defer rows.Close()

	var results []model.ProfileEmbedding
	for rows.Next() {
		var e model.ProfileEmbedding
		var distance float64
		if err := rows.Scan(&e.ID, &e.SourceDoc, &e.Term, &distance); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		e.Weight = 1.0 - distance
		results = append(results, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate embedding rows: %w", err)
	}
	return results, nil
}

// Close releases the underlying database connection.
func (r *ProfileRepository) Close() error {
	return r.db.Close()
}

// serializeFloat32 encodes a float32 slice as a little-endian binary blob
// that sqlite-vec accepts as a vector value.
func serializeFloat32(vector []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, vector); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
