package sqlite

const schemaSQL = `
CREATE TABLE IF NOT EXISTS profile_docs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source     TEXT    NOT NULL UNIQUE,
    chunk      TEXT    NOT NULL,
    embedding  BLOB    NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
