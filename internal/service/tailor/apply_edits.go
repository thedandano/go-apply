package tailor

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// ApplyEdits applies the edit envelope to a deep copy of sections.
// Edits are processed in order; each is independent.
// Rejections are collected in EditResult.EditsRejected — no error is ever returned.
// The input sections are not mutated.
func (s *Service) ApplyEdits(_ context.Context, sections model.SectionMap, edits []port.Edit) (port.EditResult, error) { //nolint:gocritic // hugeParam: interface constraint
	out := copySections(sections)
	result := port.EditResult{NewSections: out}

	for i, edit := range edits {
		var rejectReason string

		switch edit.Section {
		case "skills":
			rejectReason = applySkillsEdit(&result.NewSections, edit)
		case "experience":
			rejectReason = applyExperienceEdit(&result.NewSections, edit)
		default:
			rejectReason = fmt.Sprintf("unknown section %q", edit.Section)
		}

		if rejectReason != "" {
			result.EditsRejected = append(result.EditsRejected, port.EditRejection{Index: i, Reason: rejectReason})
		} else {
			result.EditsApplied = append(result.EditsApplied, edit)
		}
	}

	return result, nil
}

func applySkillsEdit(sections *model.SectionMap, edit port.Edit) string { //nolint:gocritic // hugeParam: value semantics match ApplyEdits loop, pointer would not reduce allocations
	if sections.Skills == nil {
		sections.Skills = &model.SkillsSection{Kind: model.SkillsKindFlat}
	}
	if sections.Skills.Kind == model.SkillsKindCategorized {
		cats := sections.Skills.Categorized
		if edit.Category == "" {
			return fmt.Sprintf("op %q on categorized skills requires a category; available: [%s]",
				edit.Op, sortedKeys(cats))
		}
		if _, ok := cats[edit.Category]; !ok {
			return fmt.Sprintf("category %q not found; available: [%s]",
				edit.Category, sortedKeys(cats))
		}
		switch edit.Op {
		case port.EditOpAdd:
			cats[edit.Category] = append(cats[edit.Category], splitTrim(edit.Value)...)
		case port.EditOpReplace:
			cats[edit.Category] = splitTrim(edit.Value)
		default:
			return fmt.Sprintf("unsupported op %q for section %q", edit.Op, edit.Section)
		}
		return ""
	}
	switch edit.Op {
	case port.EditOpReplace:
		sections.Skills.Flat = edit.Value
	case port.EditOpAdd:
		if sections.Skills.Flat == "" {
			sections.Skills.Flat = edit.Value
		} else {
			sections.Skills.Flat = sections.Skills.Flat + ", " + edit.Value
		}
	default:
		return fmt.Sprintf("unsupported op %q for section %q", edit.Op, edit.Section)
	}
	return ""
}

func applyExperienceEdit(sections *model.SectionMap, edit port.Edit) string { //nolint:gocritic // hugeParam: value semantics match ApplyEdits loop, pointer would not reduce allocations
	entryIdx, bulletIdx, err := parseBulletTarget(edit.Target)
	if err != nil {
		return err.Error()
	}
	if entryIdx < 0 || entryIdx >= len(sections.Experience) {
		return fmt.Sprintf("entry index %d out of bounds (len=%d)", entryIdx, len(sections.Experience))
	}
	bullets := sections.Experience[entryIdx].Bullets
	if bulletIdx < 0 || bulletIdx >= len(bullets) {
		return fmt.Sprintf("bullet index %d out of bounds (len=%d)", bulletIdx, len(bullets))
	}

	switch edit.Op {
	case port.EditOpReplace:
		sections.Experience[entryIdx].Bullets[bulletIdx] = edit.Value
	case port.EditOpRemove:
		b := sections.Experience[entryIdx].Bullets
		b = append(b[:bulletIdx], b[bulletIdx+1:]...)
		sections.Experience[entryIdx].Bullets = b
	default:
		return fmt.Sprintf("unsupported op %q for section %q", edit.Op, edit.Section)
	}
	return ""
}

func sortedKeys(m map[string][]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

// parseBulletTarget parses "exp-<i>-b<j>" into (entryIdx, bulletIdx).
func parseBulletTarget(target string) (int, int, error) {
	parts := strings.Split(target, "-")
	if len(parts) != 3 || parts[0] != "exp" || !strings.HasPrefix(parts[2], "b") {
		return 0, 0, fmt.Errorf("invalid target format %q (want exp-<i>-b<j>)", target)
	}
	entryIdx, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid entry index in target %q: %w", target, err)
	}
	bulletIdx, err := strconv.Atoi(parts[2][1:])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid bullet index in target %q: %w", target, err)
	}
	return entryIdx, bulletIdx, nil
}

// copySections returns a deep copy of sections so ApplyEdits does not mutate the caller's value.
func copySections(s model.SectionMap) model.SectionMap { //nolint:gocritic // hugeParam: internal helper, value semantics are intentional
	out := s

	if s.Experience != nil {
		out.Experience = make([]model.ExperienceEntry, len(s.Experience))
		copy(out.Experience, s.Experience)
		for i := range out.Experience {
			src := s.Experience[i].Bullets
			if src != nil {
				dst := make([]string, len(src))
				copy(dst, src)
				out.Experience[i].Bullets = dst
			}
		}
	}

	if s.Skills != nil {
		sk := *s.Skills
		if s.Skills.Categorized != nil {
			sk.Categorized = make(map[string][]string, len(s.Skills.Categorized))
			for cat, items := range s.Skills.Categorized {
				dst := make([]string, len(items))
				copy(dst, items)
				sk.Categorized[cat] = dst
			}
		}
		out.Skills = &sk
	}

	return out
}
