package mcpserver_test

// Tests for issue #117: T1/T2 must not overwrite the base resume sections file.
//
// Before the fix, session_tools.go called deps.Resumes.SaveSections after
// T1 and T2, persisting tailored sections back to the base resume label and
// corrupting subsequent T0 scoring.
//
// After the fix:
//   - T1 stores edited sections in sess.TailoredSections (a new Session field)
//     and does NOT call SaveSections.
//   - T2 reads sess.TailoredSections set by T1; never calls SaveSections.
//   - The base sections file on disk is unchanged across a tailoring session.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// ── spy resume repo ───────────────────────────────────────────────────────────

// spyResumeRepo records LoadSections / SaveSections invocations and dispatches
// LoadSections behaviour through an injectable hook keyed by call number.
type spyResumeRepo struct {
	loadCallCount      int
	saveSectionsCalled bool
	saveSectionsCount  int
	loadSectionsFunc   func(callNum int) (model.SectionMap, error)
}

func (r *spyResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "main", Path: "/fake/main.txt"}}, nil
}

func (r *spyResumeRepo) LoadSections(_ string) (model.SectionMap, error) {
	r.loadCallCount++
	if r.loadSectionsFunc != nil {
		return r.loadSectionsFunc(r.loadCallCount)
	}
	return model.SectionMap{}, model.ErrSectionsMissing
}

func (r *spyResumeRepo) SaveSections(_ string, _ model.SectionMap) error { //nolint:gocritic // hugeParam: interface constraint
	r.saveSectionsCalled = true
	r.saveSectionsCount++
	return nil
}

// sectionsForTailoring returns a SectionMap with both Skills (for T1) and one
// Experience entry (for T2). Mirrors stubResumeRepoWithExperienceSections.
func sectionsForTailoring() model.SectionMap {
	return model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindFlat,
			Flat: "Go, Python",
		},
		Experience: []model.ExperienceEntry{
			{
				Company:   "Acme Corp",
				Role:      "Engineer",
				StartDate: "2020-01",
				Bullets:   []string{"Built distributed systems in Go", "Led migration to Kubernetes"},
			},
		},
	}
}

// applyConfigWithSpy returns an ApplyConfig wired to the supplied spy repo and
// otherwise uses the same stubs as stubApplyConfigForSession.
func applyConfigWithSpy(spy *spyResumeRepo) pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:        &stubJDFetcher{},
		Scorer:         &stubScorer{},
		Resumes:        spy,
		Loader:         &stubDocumentLoader{},
		AppRepo:        &stubApplicationRepository{},
		Defaults:       &config.AppDefaults{},
		PDFRenderer:    &stubPDFRenderer{failRender: false},
		Extractor:      &stubExtractor{failExtract: false},
		SurvivalDiffer: &stubSurvivalDiffer{},
	}
}

// scoredSessionWithSpy walks through load_jd + submit_keywords using the spy
// repo and returns the resulting session ID.
func scoredSessionWithSpy(t *testing.T, cfg *pipeline.ApplyConfig) string {
	t.Helper()
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v — raw: %s", err, loadText)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("load_jd returned no session_id — raw: %s", loadText)
	}

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{YearsOfExperience: 5})
	kwText := extractText(t, kwResult)
	var kwEnv map[string]any
	if err := json.Unmarshal([]byte(kwText), &kwEnv); err != nil {
		t.Fatalf("submit_keywords not JSON: %v — raw: %s", err, kwText)
	}
	if kwEnv["status"] != "ok" {
		t.Fatalf("submit_keywords status = %v, want ok — raw: %s", kwEnv["status"], kwText)
	}
	return sessionID
}

// ── Test 1: T1 must not call SaveSections ─────────────────────────────────────

// TestT1_DoesNotCallSaveSections asserts the base sections file is not overwritten by
// a successful T1 pass.
func TestT1_DoesNotCallSaveSections(t *testing.T) {
	spy := &spyResumeRepo{
		loadSectionsFunc: func(_ int) (model.SectionMap, error) {
			return sectionsForTailoring(), nil
		},
	}
	cfg := applyConfigWithSpy(spy)

	sessionID := scoredSessionWithSpy(t, &cfg)

	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"skills","op":"add","value":"Kubernetes"}]`,
	})
	t1Result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})
	t1Text := extractText(t, t1Result)

	var env map[string]any
	if err := json.Unmarshal([]byte(t1Text), &env); err != nil {
		t.Fatalf("submit_tailor_t1 not JSON: %v — raw: %s", err, t1Text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — raw: %s", env["status"], t1Text)
	}
	if spy.saveSectionsCalled {
		t.Errorf("SaveSections was called %d time(s) during T1; expected 0 — base sections file must not be overwritten",
			spy.saveSectionsCount)
	}
}

// ── Test 2: T2 must not call SaveSections ─────────────────────────────────────

// TestT2_DoesNotCallSaveSections asserts the base sections file is not overwritten by
// a successful T2 pass. Runs the full T0→T1→T2 cascade and verifies SaveSections
// is never called, and that T2 does not call LoadSections (reads from
// sess.TailoredSections set by T1).
func TestT2_DoesNotCallSaveSections(t *testing.T) {
	spy := &spyResumeRepo{
		loadSectionsFunc: func(_ int) (model.SectionMap, error) {
			return sectionsForTailoring(), nil
		},
	}
	cfg := applyConfigWithSpy(spy)

	sessionID := scoredSessionWithSpy(t, &cfg)

	// T1 — must run before T2
	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"skills","op":"add","value":"Kubernetes"}]`,
	})
	mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})

	loadCountAfterT1 := spy.loadCallCount

	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"Built distributed systems in Go and Kubernetes"}]`,
	})
	t2Result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{})
	t2Text := extractText(t, t2Result)

	var env map[string]any
	if err := json.Unmarshal([]byte(t2Text), &env); err != nil {
		t.Fatalf("submit_tailor_t2 not JSON: %v — raw: %s", err, t2Text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — raw: %s", env["status"], t2Text)
	}
	if spy.saveSectionsCalled {
		t.Errorf("SaveSections was called %d time(s); expected 0 — base sections file must not be overwritten",
			spy.saveSectionsCount)
	}
	// T2 must NOT call LoadSections — it reads from sess.TailoredSections set by T1.
	if spy.loadCallCount > loadCountAfterT1 {
		t.Errorf("T2 must not call LoadSections when TailoredSections is available; load count was %d after T1 and %d after T2",
			loadCountAfterT1, spy.loadCallCount)
	}
}

// ── Test 3: T2 after T1 must use session chaining, not disk reload ────────────

// TestT2AfterT1_UsesSessChaining_NotDiskReload asserts that once T1 has applied
// edits, T2 does NOT need to re-read the sections file from disk: it must consume
// the in-memory tailored sections that T1 stored on the Session.
//
// Call counting on the happy path with one resume:
//
//  1. submit_keywords fan-out scoring loop  (session_tools.go:177)
//  2. submit_keywords skills_section block  (session_tools.go:250)
//  3. submit_tailor_t1 LoadSections
//
// T2 no longer calls LoadSections — it reads sess.TailoredSections set by T1.
// We let calls 1–3 succeed and force call 4+ to fail with ErrSectionsMissing.
// T2 must succeed by reading sess.TailoredSections without touching disk.
func TestT2AfterT1_UsesSessChaining_NotDiskReload(t *testing.T) {
	spy := &spyResumeRepo{
		loadSectionsFunc: func(callNum int) (model.SectionMap, error) {
			if callNum >= 4 {
				return model.SectionMap{}, model.ErrSectionsMissing
			}
			return sectionsForTailoring(), nil
		},
	}
	cfg := applyConfigWithSpy(spy)

	sessionID := scoredSessionWithSpy(t, &cfg)

	// T1 — call #3 to LoadSections, must succeed.
	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"skills","op":"add","value":"Kubernetes"}]`,
	})
	t1Text := extractText(t, mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{}))
	var t1Env map[string]any
	if err := json.Unmarshal([]byte(t1Text), &t1Env); err != nil {
		t.Fatalf("submit_tailor_t1 not JSON: %v — raw: %s", err, t1Text)
	}
	if t1Env["status"] != "ok" {
		t.Fatalf("T1 status = %v, want ok — raw: %s", t1Env["status"], t1Text)
	}

	// T2 — must succeed by reading sess.TailoredSections (chained from T1)
	// rather than calling LoadSections (which would now return ErrSectionsMissing).
	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"Built distributed systems in Go and Kubernetes"}]`,
	})
	t2Text := extractText(t, mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{}))
	var t2Env map[string]any
	if err := json.Unmarshal([]byte(t2Text), &t2Env); err != nil {
		t.Fatalf("submit_tailor_t2 not JSON: %v — raw: %s", err, t2Text)
	}
	if t2Env["status"] != "ok" {
		t.Errorf("T2 status = %v, want ok — T2 must consume sess.TailoredSections from T1, not reload from disk; raw: %s",
			t2Env["status"], t2Text)
	}
}

// ── Test 4: cross-session isolation (regression guard) ────────────────────────

// TestT1_CrossSession_IsolatesEditedSections asserts that running T1 on session
// A does not perturb T2 running on session B against the same repo. Sessions
// are keyed by random ID and the spy repo is stateless apart from call counts,
// so this is GREEN today; it documents the post-fix invariant that tailored
// state is per-session and never leaks into another session's view of the
// resume.
func TestT1_CrossSession_IsolatesEditedSections(t *testing.T) {
	spy := &spyResumeRepo{
		loadSectionsFunc: func(_ int) (model.SectionMap, error) {
			return sectionsForTailoring(), nil
		},
	}
	cfg := applyConfigWithSpy(spy)

	sessionA := scoredSessionWithSpy(t, &cfg)
	sessionB := scoredSessionWithSpy(t, &cfg)
	if sessionA == sessionB {
		t.Fatalf("expected distinct session IDs, got %q twice", sessionA)
	}

	// T1 on session A.
	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionA,
		"edits":      `[{"section":"skills","op":"add","value":"Kubernetes"}]`,
	})
	t1Text := extractText(t, mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{}))
	var t1Env map[string]any
	if err := json.Unmarshal([]byte(t1Text), &t1Env); err != nil {
		t.Fatalf("session A T1 not JSON: %v — raw: %s", err, t1Text)
	}
	if t1Env["status"] != "ok" {
		t.Fatalf("session A T1 status = %v, want ok — raw: %s", t1Env["status"], t1Text)
	}

	// T1 on session B — required before T2.
	t1BReq := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionB,
		"edits":      `[{"section":"skills","op":"add","value":"Docker"}]`,
	})
	t1BText := extractText(t, mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1BReq, &cfg, &config.Config{}))
	var t1BEnv map[string]any
	if err := json.Unmarshal([]byte(t1BText), &t1BEnv); err != nil {
		t.Fatalf("session B T1 not JSON: %v — raw: %s", err, t1BText)
	}
	if t1BEnv["status"] != "ok" {
		t.Fatalf("session B T1 status = %v, want ok — raw: %s", t1BEnv["status"], t1BText)
	}

	// T2 on session B (chained from session B's T1 — must not use session A's tailored sections).
	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": sessionB,
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"Built distributed systems in Go and Kubernetes"}]`,
	})
	t2Text := extractText(t, mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{}))
	var t2Env map[string]any
	if err := json.Unmarshal([]byte(t2Text), &t2Env); err != nil {
		t.Fatalf("session B T2 not JSON: %v — raw: %s", err, t2Text)
	}
	if t2Env["status"] != "ok" {
		t.Errorf("session B T2 status = %v, want ok — session A's T1 must not affect session B; raw: %s",
			t2Env["status"], t2Text)
	}
}
