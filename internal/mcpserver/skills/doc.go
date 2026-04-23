// Package skills holds vendored external skill artifacts embedded into go-apply
// at build time. These files are not go-apply-authored source — they are
// refreshed from an external grimoire repository via `make sync-skill`.
//
// Do not edit files in this directory by hand. To update the vendored skill:
//
//	make sync-skill              # copies from RESUME_TAILOR_SKILL_SRC and regenerates .sha256
//
// The .sha256 sentinel is checked at test time (TestTailorSkillBodyIntegrityHash)
// to catch embedded-body/sentinel mismatches before they reach production.
package skills
