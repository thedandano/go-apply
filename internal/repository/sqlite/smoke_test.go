//go:build integration

package sqlite_test

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestModernSQLiteSmoke(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE docs (id INTEGER PRIMARY KEY, text TEXT NOT NULL, embedding BLOB NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	vec := []byte{0x00, 0x00, 0x80, 0x3f} // 1.0 as little-endian float32
	_, err = db.Exec(`INSERT INTO docs(text, embedding) VALUES (?, ?)`, "hello world", vec)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var id int64
	var text string
	var blob []byte
	err = db.QueryRow(`SELECT id, text, embedding FROM docs WHERE id=1`).Scan(&id, &text, &blob)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if id != 1 || text != "hello world" || len(blob) != 4 {
		t.Errorf("unexpected row: id=%d text=%q bloblen=%d", id, text, len(blob))
	}
	t.Logf("modernc sqlite smoke: ok — id=%d text=%q bloblen=%d", id, text, len(blob))
}
