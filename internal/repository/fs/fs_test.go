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

	// Supported: .docx, .pdf, .txt, .md, .markdown, .text. Ignored: .xlsx, .csv.
	for _, name := range []string{"resume.docx", "resume.pdf", "resume.txt", "resume.md", "ignore.xlsx", "ignore.csv"} {
		os.WriteFile(filepath.Join(inputsDir, name), []byte("content"), config.FilePerm) //nolint:errcheck
	}

	repo := fs.NewResumeRepository(dir)
	resumes, err := repo.ListResumes()
	if err != nil {
		t.Fatal(err)
	}
	if len(resumes) != 4 {
		t.Errorf("expected 4 resumes (.docx + .pdf + .txt + .md), got %d", len(resumes))
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

// --- ApplicationRepository tests ---

func newRecord(url string) *model.ApplicationRecord {
	return &model.ApplicationRecord{
		URL:     url,
		RawText: "senior golang engineer remote",
		JD: model.JDData{
			Title:   "Senior Engineer",
			Company: "Acme",
		},
	}
}

func TestApplicationRepository_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	rec := newRecord("https://example.com/job/1")
	rec.JD.PayRangeMin = 150_000
	rec.JD.PayRangeMax = 220_000

	if err := repo.Put(rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, found, err := repo.Get("https://example.com/job/1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected record to be found")
	}
	if got.RawText != rec.RawText {
		t.Errorf("RawText: got %q, want %q", got.RawText, rec.RawText)
	}
	if got.JD.Title != rec.JD.Title {
		t.Errorf("JD.Title: got %q, want %q", got.JD.Title, rec.JD.Title)
	}
	if got.JD.PayRangeMax != 220_000 {
		t.Errorf("JD.PayRangeMax: got %v, want 220000", got.JD.PayRangeMax)
	}
}

func TestApplicationRepository_GetMissing(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	got, found, err := repo.Get("https://example.com/nonexistent")
	if err != nil {
		t.Fatalf("Get on missing: unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}
	if got != nil {
		t.Fatal("expected nil record for missing entry")
	}
}

func TestApplicationRepository_GetCorrupted(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	// Seed a valid record so we know the filename, then corrupt it.
	rec := newRecord("https://example.com/job/corrupt")
	if err := repo.Put(rec); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Find the written file and overwrite with garbage.
	entries, _ := os.ReadDir(filepath.Join(dir, "applications"))
	if len(entries) == 0 {
		t.Fatal("expected at least one file in applications dir")
	}
	path := filepath.Join(dir, "applications", entries[0].Name())
	os.WriteFile(path, []byte("not json"), config.FilePerm) //nolint:errcheck

	_, _, err := repo.Get("https://example.com/job/corrupt")
	if err == nil {
		t.Fatal("expected error on corrupted record, got nil")
	}
}

func TestApplicationRepository_Update(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	rec := newRecord("https://example.com/job/2")
	if err := repo.Put(rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rec.Outcome = model.OutcomeInterview
	rec.Applied = "2026-04-13"
	if err := repo.Update(rec); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, found, err := repo.Get("https://example.com/job/2")
	if err != nil || !found {
		t.Fatalf("Get after Update: found=%v err=%v", found, err)
	}
	if got.Outcome != model.OutcomeInterview {
		t.Errorf("Outcome: got %q, want %q", got.Outcome, model.OutcomeInterview)
	}
	if got.Applied != "2026-04-13" {
		t.Errorf("Applied: got %q, want %q", got.Applied, "2026-04-13")
	}
	// RawText must be preserved
	if got.RawText != rec.RawText {
		t.Errorf("RawText must survive Update: got %q", got.RawText)
	}
}

func TestApplicationRepository_UpdateMissing(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	rec := newRecord("https://example.com/nonexistent")
	err := repo.Update(rec)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist for missing record, got: %v", err)
	}
}

func TestApplicationRepository_List(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	urls := []string{
		"https://example.com/job/a",
		"https://example.com/job/b",
		"https://example.com/job/c",
	}
	for _, u := range urls {
		if err := repo.Put(newRecord(u)); err != nil {
			t.Fatalf("Put %s: %v", u, err)
		}
	}

	records, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("List: got %d records, want 3", len(records))
	}
}

func TestApplicationRepository_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	repo := fs.NewApplicationRepository(dir)

	records, err := repo.List()
	if err != nil {
		t.Fatalf("List on empty repo: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}
