package updater_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/updater"
)

func TestReadCache_NonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	cache, err := updater.ReadCache(path)
	if err != nil {
		t.Fatalf("unexpected error reading non-existent cache: %v", err)
	}
	if cache != nil {
		t.Errorf("expected nil cache for non-existent file, got %+v", cache)
	}
}

func TestReadCache_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update-check.json")
	if err := os.WriteFile(path, []byte(`{not-valid`), config.FilePerm); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cache, err := updater.ReadCache(path)
	if err != nil {
		t.Fatalf("corrupt cache should not return error: %v", err)
	}
	if cache != nil {
		t.Errorf("corrupt cache should return nil, got %+v", cache)
	}
}

func TestWriteReadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check.json")
	now := time.Now().UTC().Truncate(time.Second)
	original := &model.UpdateCache{
		LatestVersion:  "v0.2.0",
		CurrentVersion: "v0.1.0",
		CheckedAt:      now,
	}
	if err := updater.WriteCache(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := updater.ReadCache(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil cache after write")
	}
	if got.LatestVersion != original.LatestVersion {
		t.Errorf("LatestVersion = %q, want %q", got.LatestVersion, original.LatestVersion)
	}
	if got.CurrentVersion != original.CurrentVersion {
		t.Errorf("CurrentVersion = %q, want %q", got.CurrentVersion, original.CurrentVersion)
	}
	if !got.CheckedAt.Equal(original.CheckedAt) {
		t.Errorf("CheckedAt = %v, want %v", got.CheckedAt, original.CheckedAt)
	}
}

func TestIsCacheFresh(t *testing.T) {
	t.Run("nil cache is not fresh", func(t *testing.T) {
		if updater.IsCacheFresh(nil, 24*time.Hour) {
			t.Error("nil cache should not be fresh")
		}
	})

	t.Run("recent cache is fresh", func(t *testing.T) {
		cache := &model.UpdateCache{CheckedAt: time.Now().Add(-1 * time.Hour)}
		if !updater.IsCacheFresh(cache, 24*time.Hour) {
			t.Error("1h old cache should be fresh within 24h TTL")
		}
	})

	t.Run("stale cache is not fresh", func(t *testing.T) {
		cache := &model.UpdateCache{CheckedAt: time.Now().Add(-25 * time.Hour)}
		if updater.IsCacheFresh(cache, 24*time.Hour) {
			t.Error("25h old cache should not be fresh within 24h TTL")
		}
	})
}
