package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/repository/fs"
)

func testSectionMap() model.SectionMap {
	return model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Alice"},
		Experience: []model.ExperienceEntry{
			{Company: "Acme", Role: "Eng", StartDate: "2020-01", Bullets: []string{"Did stuff"}},
		},
	}
}

func setupRepo(t *testing.T) (repo *fs.ResumeRepository, inputsDir string) {
	t.Helper()
	tmpDir := t.TempDir()
	inputsDir = filepath.Join(tmpDir, "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll inputs: %v", err)
	}
	return fs.NewResumeRepository(tmpDir), inputsDir
}

func TestResumeSections(t *testing.T) {
	t.Run("round_trip", func(t *testing.T) {
		repo, _ := setupRepo(t)
		sm := testSectionMap()

		if err := repo.SaveSections("myresume", sm); err != nil {
			t.Fatalf("SaveSections: %v", err)
		}
		got, err := repo.LoadSections("myresume")
		if err != nil {
			t.Fatalf("LoadSections: %v", err)
		}
		if !reflect.DeepEqual(sm, got) {
			t.Errorf("round-trip mismatch\n  want: %+v\n   got: %+v", sm, got)
		}
	})

	t.Run("enoent", func(t *testing.T) {
		repo, _ := setupRepo(t)

		_, err := repo.LoadSections("nonexistent")
		if err == nil {
			t.Fatal("expected error for missing sections file, got nil")
		}
		if !errors.Is(err, model.ErrSectionsMissing) {
			t.Errorf("want errors.Is(err, model.ErrSectionsMissing); got: %v", err)
		}
	})

	t.Run("schema_version_mismatch", func(t *testing.T) {
		repo, inputsDir := setupRepo(t)

		sectionsFile := filepath.Join(inputsDir, "myresume.sections.json")
		if err := os.WriteFile(sectionsFile, []byte(`{"schema_version": 999}`), 0o644); err != nil {
			t.Fatalf("WriteFile sections file: %v", err)
		}

		_, err := repo.LoadSections("myresume")
		if err == nil {
			t.Fatal("expected error for schema version mismatch, got nil")
		}
		if !errors.Is(err, model.ErrSchemaVersionUnsupported) {
			t.Errorf("want errors.Is(err, model.ErrSchemaVersionUnsupported); got: %v", err)
		}
	})

	t.Run("atomic_write_no_tmp_leak", func(t *testing.T) {
		repo, inputsDir := setupRepo(t)

		if err := repo.SaveSections("myresume", testSectionMap()); err != nil {
			t.Fatalf("SaveSections: %v", err)
		}

		entries, err := os.ReadDir(inputsDir)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Errorf("tmp file leaked after SaveSections: %s", e.Name())
			}
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		repo, _ := setupRepo(t)

		first := testSectionMap()
		second := model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "Bob"},
			Experience: []model.ExperienceEntry{
				{Company: "Globex", Role: "SRE", StartDate: "2022-06", Bullets: []string{"Did other stuff"}},
			},
		}

		if err := repo.SaveSections("myresume", first); err != nil {
			t.Fatalf("SaveSections (first): %v", err)
		}
		if err := repo.SaveSections("myresume", second); err != nil {
			t.Fatalf("SaveSections (second): %v", err)
		}

		got, err := repo.LoadSections("myresume")
		if err != nil {
			t.Fatalf("LoadSections after overwrite: %v", err)
		}
		if !reflect.DeepEqual(second, got) {
			t.Errorf("overwrite: want second value\n  want: %+v\n   got: %+v", second, got)
		}
	})
}
