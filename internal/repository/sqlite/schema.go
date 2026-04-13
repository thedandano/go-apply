package sqlite

import "fmt"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS profile_docs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source     TEXT NOT NULL UNIQUE,
    chunk      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func vecTableSQL(dim int) string {
	return fmt.Sprintf(`
CREATE VIRTUAL TABLE IF NOT EXISTS profile_embeddings USING vec0(
    doc_id    INTEGER PRIMARY KEY,
    embedding FLOAT[%d] distance_metric=cosine
);`, dim)
}
