// Package mcpserver provides the MCP stdio server for Claude Code integration.
package mcpserver

import _ "embed"

//go:embed skills/resume-tailor.md
var tailorSkillBody string

// tailorPreludeText is the go-apply-specific context that is prepended to the
// vendored resume-tailor skill body to form the complete tailor_resume prompt.
const tailorPreludeText = `You are operating inside the go-apply MCP session. All resume tailoring work happens here through go-apply tools.

## Path and Script Overrides

Do not invoke any skills via /mnt/skills/... paths. Do not run resume_modifier.py from this MCP session. These paths and scripts are not available in go-apply's execution environment and MUST NOT be called.

## Scoring

go-apply scores automatically via ` + "`submit_tailored_resume`" + `; do not call score.py or any external scoring script.

## Input Prerequisites

Use the ` + "`get_config`" + ` MCP tool to check ` + "`has_skills`" + ` and ` + "`has_accomplishments`" + `. If either is false, stop and tell the user. Do not proceed with tailoring until both are present.

## PDF Rendering (Externalized)

After ` + "`submit_tailored_resume`" + ` returns success, tell the user their tailored resume text is stored and that they can optionally run the ` + "`resume-tailor`" + ` skill separately to produce a rendered PDF. The skill lives at the user's grimoire path (typically ` + "`~/workplace/the-scriptorium/grimoire/skills/resume-tailor/`" + `); it has its own ` + "`setup.sh`" + ` for LaTeX/Python prereqs.

PDF rendering is not part of go-apply. Do not produce a modification_spec. Do not describe PDF output in your response. Do not invoke ` + "`present_files`" + `, ` + "`resume_modifier.py`" + `, ` + "`template_generator.py`" + `, ` + "`latex_converter.py`" + `, or any rendering script from this MCP session. Your deliverable ends at ` + "`submit_tailored_resume`" + `.

## Submission Contract

Submit the full rewritten resume text as a single string to ` + "`submit_tailored_resume`" + ` with an optional ` + "`changelog`" + ` array of ` + "`{action, target, keyword, reason}`" + ` entries (reason <= 512 bytes, keyword <= 128 bytes).

## Conflict Resolution

Where this document conflicts with the overrides above, the overrides win.`

// tailorResumePromptText is the complete tailor_resume prompt: the go-apply
// context prelude followed by the vendored resume-tailor skill body.
var tailorResumePromptText string

func init() {
	tailorResumePromptText = tailorPreludeText + "\n\n" + tailorSkillBody
}
