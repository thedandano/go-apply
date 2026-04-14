package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/repository/fs"
)

// --- ResumeRepository tests ---

func TestResumeRepository_ListResumes_Empty(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "inputs"), config.DirPerm) //nolint:errcheck

	repo := fs.NewResumeRepository(dir)
	resumes, err := repo.ListResumes()
	if err != nil {
		t.Fatal(err)
	}
	if len(resumes) != 0 {
		t.Errorf("expected 0 resumes, got %d", len(resumes))
	}
}

func TestResumeRepository_ListResumes_FiltersExtensions(t *testing.T) {
	dir := t.TempDir()
	inputsDir := filepath.Join(dir, "inputs")
	os.MkdirAll(inputsDir, config.DirPerm) //nolint:errcheck

	// Write files of various extensions; only .docx and .pdf should be listed.
	for _, name := range []string{"resume.docx", "resume.pdf", "readme.txt", "ignore.xlsx"} {
		os.WriteFile(filepath.Join(inputsDir, name), []byte("content"), 0644) //nolint:errcheck
	}

	repo := fs.NewResumeRepository(dir)
	resumes, err := repo.ListResumes()
	if err != nil {
		t.Fatal(err)
	}
	if len(resumes) != 2 {
		t.Errorf("expected 2 resumes (.docx + .pdf), got %d", len(resumes))
	}
	for _, r := range resumes {
		if r.Label == "" {
			t.Error("expected non-empty label")
		}
		if r.Path == "" {
			t.Error("expected non-empty path")
		}
	}
}

func TestResumeRepository_ListResumes_ErrorOnMissingDir(t *testing.T) {
	repo := fs.NewResumeRepository("/nonexistent/path")
	_, err := repo.ListResumes()
	if err == nil {
		t.Fatal("expected error for missing inputs dir, got nil")
	}
}

// --- JDCacheRepository tests ---

func TestJDCacheRepository_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewJDCacheRepository(dir)

	jd := model.JDData{Title: "Senior Gopher", Company: "Acme"}
	if err := repo.Put("https://example.com/job/1", "raw job text", jd); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rawText, got, found := repo.Get("https://example.com/job/1")
	if !found {
		t.Fatal("expected entry to be found")
	}
	if rawText != "raw job text" {
		t.Errorf("raw text: got %q, want %q", rawText, "raw job text")
	}
	if got.Title != jd.Title || got.Company != jd.Company {
		t.Errorf("jd mismatch: got %+v, want %+v", got, jd)
	}
}

func TestJDCacheRepository_GetMissing(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewJDCacheRepository(dir)

	_, _, found := repo.Get("https://example.com/nonexistent")
	if found {
		t.Fatal("expected not found for missing entry")
	}
}

func TestJDCacheRepository_Update(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewJDCacheRepository(dir)

	original := model.JDData{Title: "Engineer", Company: "OldCo"}
	if err := repo.Put("https://example.com/job/2", "raw text", original); err != nil {
		t.Fatalf("Put: %v", err)
	}

	updated := model.JDData{Title: "Senior Engineer", Company: "NewCo"}
	if err := repo.Update("https://example.com/job/2", updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	rawText, got, found := repo.Get("https://example.com/job/2")
	if !found {
		t.Fatal("expected entry to be found after update")
	}
	// raw text must be preserved by Update
	if rawText != "raw text" {
		t.Errorf("Update must preserve raw_text, got %q", rawText)
	}
	if got.Title != updated.Title || got.Company != updated.Company {
		t.Errorf("jd mismatch after update: got %+v, want %+v", got, updated)
	}
}

func TestJDCacheRepository_UpdateMissing(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewJDCacheRepository(dir)

	err := repo.Update("https://example.com/nonexistent", model.JDData{})
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist for missing entry, got: %v", err)
	}
}
