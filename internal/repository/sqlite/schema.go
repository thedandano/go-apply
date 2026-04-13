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

const keywordEmbeddingsCacheSQL = `
CREATE TABLE IF NOT EXISTS keyword_embeddings_cache (
    keyword    TEXT NOT NULL PRIMARY KEY,
    vector     BLOB NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func vecTableSQL(dim int) string {
	return fmt.Sprintf(`
CREATE VIRTUAL TABLE IF NOT EXISTS profile_docs_embeddings USING vec0(
    doc_id    INTEGER PRIMARY KEY,
    embedding FLOAT[%d] distance_metric=cosine
);`, dim)
}
