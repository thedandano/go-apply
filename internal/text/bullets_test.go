package text_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/text"
)

func TestExtractExperienceBullets_WithExperienceSection(t *testing.T) {
	resume := `John Doe
Software Engineer

Experience
- Led a team of 5 engineers
- Reduced latency by 30%

Education
- BS Computer Science, MIT
`
	bullets := text.ExtractExperienceBullets(resume)
	if len(bullets) != 2 {
		t.Fatalf("want 2 bullets, got %d: %v", len(bullets), bullets)
	}
	if bullets[0] != "Led a team of 5 engineers" {
		t.Errorf("bullet[0]: want 'Led a team of 5 engineers', got %q", bullets[0])
	}
	if bullets[1] != "Reduced latency by 30%" {
		t.Errorf("bullet[1]: want 'Reduced latency by 30%%', got %q", bullets[1])
	}
}

func TestExtractExperienceBullets_NoExperienceHeader_ScanAll(t *testing.T) {
	resume := `- Built distributed systems
- Increased revenue by 20%
`
	bullets := text.ExtractExperienceBullets(resume)
	if len(bullets) != 2 {
		t.Fatalf("want 2 bullets, got %d: %v", len(bullets), bullets)
	}
}

func TestExtractExperienceBullets_StopsAtStopHeader(t *testing.T) {
	resume := `Experience
- Backend engineer at ACME

Education
- BS CS
- MS CS

Skills
- Go, Python
`
	bullets := text.ExtractExperienceBullets(resume)
	// Only bullets from Experience section, not Education/Skills
	if len(bullets) != 1 {
		t.Fatalf("want 1 bullet from experience, got %d: %v", len(bullets), bullets)
	}
	if bullets[0] != "Backend engineer at ACME" {
		t.Errorf("bullet[0]: want 'Backend engineer at ACME', got %q", bullets[0])
	}
}

func TestExtractExperienceBullets_MultipleBulletStyles(t *testing.T) {
	resume := `Experience
• Bullet with dot
- Bullet with dash
* Bullet with star
`
	bullets := text.ExtractExperienceBullets(resume)
	if len(bullets) != 3 {
		t.Fatalf("want 3 bullets, got %d: %v", len(bullets), bullets)
	}
}

func TestExtractExperienceBullets_EmptyText(t *testing.T) {
	bullets := text.ExtractExperienceBullets("")
	if len(bullets) != 0 {
		t.Errorf("want 0 bullets for empty text, got %d", len(bullets))
	}
}

func TestExtractExperienceBullets_NoBullets(t *testing.T) {
	resume := `John Doe
Software Engineer at Company
Led multiple projects across teams.
`
	bullets := text.ExtractExperienceBullets(resume)
	if len(bullets) != 0 {
		t.Errorf("want 0 bullets when no bullet lines, got %d: %v", len(bullets), bullets)
	}
}
