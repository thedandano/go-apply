package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	fsrepo "github.com/thedandano/go-apply/internal/repository/fs"
)

// ─── ResumeRepository tests ───────────────────────────────────────────────────

func TestResumeRepository_ListResumes_Empty(t *testing.T) {
	dir := t.TempDir()
	repo := fsrepo.NewResumeRepository(dir)

	results, err := repo.ListResumes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	if results == nil {
		t.Error("expected non-nil slice")
	}
}

func TestResumeRepository_ListResumes_FiltersExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"resume.pdf", "cover.docx", "bio.txt", "photo.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o600); err != nil {
			t.Fatalf("create test file: %v", err)
		}
	}

	repo := fsrepo.NewResumeRepository(dir)
	results, err := repo.ListResumes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestResumeRepository_ListResumes_Label(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "my-resume.pdf"), []byte("data"), 0o600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	repo := fsrepo.NewResumeRepository(dir)
	results, err := repo.ListResumes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.Label != "my-resume" {
		t.Errorf("expected Label %q, got %q", "my-resume", got.Label)
	}
	if got.FileType != ".pdf" {
		t.Errorf("expected FileType %q, got %q", ".pdf", got.FileType)
	}
	if got.Path != filepath.Join(dir, "my-resume.pdf") {
		t.Errorf("expected Path %q, got %q", filepath.Join(dir, "my-resume.pdf"), got.Path)
	}
}

// ─── JDCacheRepository tests ──────────────────────────────────────────────────

func TestJDCacheRepository_PutGet(t *testing.T) {
	dir := t.TempDir()
	repo := fsrepo.NewJDCacheRepository(dir)

	const testURL = "https://example.com/job/123"
	const rawText = "Software Engineer at Example Corp"

	if err := repo.Put(testURL, rawText, sampleJD()); err != nil {
		t.Fatalf("Put: %v", err)
	}

	gotRaw, gotJD, found := repo.Get(testURL)
	if !found {
		t.Fatal("expected found=true")
	}
	if gotRaw != rawText {
		t.Errorf("rawText mismatch: got %q, want %q", gotRaw, rawText)
	}
	if gotJD.Title != sampleJD().Title {
		t.Errorf("JD.Title mismatch: got %q, want %q", gotJD.Title, sampleJD().Title)
	}
	if gotJD.Company != sampleJD().Company {
		t.Errorf("JD.Company mismatch: got %q, want %q", gotJD.Company, sampleJD().Company)
	}
}

func TestJDCacheRepository_GetMissing(t *testing.T) {
	dir := t.TempDir()
	repo := fsrepo.NewJDCacheRepository(dir)

	rawText, jd, found := repo.Get("https://nonexistent.com/job/999")
	if found {
		t.Error("expected found=false")
	}
	if rawText != "" {
		t.Errorf("expected empty rawText, got %q", rawText)
	}
	if jd.Title != "" {
		t.Errorf("expected empty JD, got title %q", jd.Title)
	}
}

func TestJDCacheRepository_Update(t *testing.T) {
	dir := t.TempDir()
	repo := fsrepo.NewJDCacheRepository(dir)

	const testURL = "https://example.com/job/update"
	const rawText = "Some job description"

	if err := repo.Put(testURL, rawText, sampleJD()); err != nil {
		t.Fatalf("Put: %v", err)
	}

	updated := sampleJD()
	updated.Title = "Updated Engineer"
	updated.Company = "NewCo"

	if err := repo.Update(testURL, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	gotRaw, gotJD, found := repo.Get(testURL)
	if !found {
		t.Fatal("expected found=true after update")
	}
	if gotRaw != rawText {
		t.Errorf("rawText should be preserved; got %q, want %q", gotRaw, rawText)
	}
	if gotJD.Title != "Updated Engineer" {
		t.Errorf("JD.Title mismatch: got %q", gotJD.Title)
	}
	if gotJD.Company != "NewCo" {
		t.Errorf("JD.Company mismatch: got %q", gotJD.Company)
	}
}

func TestJDCacheRepository_Put_CreatesDir(t *testing.T) {
	base := t.TempDir()
	subdir := filepath.Join(base, "deep", "nested", "cache")
	repo := fsrepo.NewJDCacheRepository(subdir)

	if err := repo.Put("https://example.com/job/dir", "text", sampleJD()); err != nil {
		t.Fatalf("Put into non-existent dir: %v", err)
	}

	_, _, found := repo.Get("https://example.com/job/dir")
	if !found {
		t.Error("expected found=true after put into created dir")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func sampleJD() model.JDData {
	return model.JDData{
		Title:         "Software Engineer",
		Company:       "ExampleCorp",
		Required:      []string{"Go", "Docker"},
		Preferred:     []string{"Kubernetes"},
		Location:      "Remote",
		Seniority:     model.SenioritySenior,
		RequiredYears: 3.0,
	}
}
