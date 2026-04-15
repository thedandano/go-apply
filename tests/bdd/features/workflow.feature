# Workflow Feature
#
# Covers the full job application pipeline:
#   fetch/text → keyword extraction → score resumes → tailor (T1+T2 when inputs allow) → cover letter → result
#
# Tags:
#   @mcp    — invoked via Claude using the get_score MCP tool
#   @cli    — invoked via go-apply run
#   @future — not yet implemented
#
# Score threshold (configurable via defaults.json): 70.0
# Always-tailor design:
#   T1 (keyword injection into Skills section): runs when score >= 40 AND skills present
#   T2 (bullet rewrites in Experience section): runs when score >= 40 AND accomplishments present
#   If score < 40: skip tailoring; advisory "structural mismatch — tailoring cannot close this gap"
# Cover letter: generated if final score >= 70 OR channel = REFERRAL/RECRUITER
# Orchestrator unavailable → INOPERABLE hard error (pipeline cannot run at all)
# Embedder unavailable → orchestrator performs keyword matching as fallback

Feature: Job Application Workflow

  Background:
    Given a user profile exists with at least one stored resume
    And go-apply is configured with an orchestrator model and endpoint

  # ─────────────────────────────────────────────────────────────────────────────
  # Base scoring — always-tailor design
  # ─────────────────────────────────────────────────────────────────────────────

  Scenario Outline: Job description input — pipeline runs with tailoring when profile is complete
    Given the user profile contains a resume, skills, and accomplishments
    When the user supplies a job description via <input_type>
    Then go-apply extracts required and preferred keywords
    And scores all stored resumes against the job description
    And the best resume score is >= 40
    And T1 keyword injection runs against the Skills section
    And T2 bullet rewrites run using the accomplishments
    And the tailored resume is re-scored
    And if the final score is >= 70, a cover letter is generated for the best-matching resume
    And the result contains the base score, tailored score, extracted keywords, and cover letter if generated

    Examples:
      | input_type |
      | URL        |
      | raw text   |

  Scenario: No skills or accomplishments — tailoring skipped, cover letter based on base score
    Given the user profile contains a resume only (no skills, no accomplishments)
    When the user supplies a job description
    Then go-apply scores all stored resumes against the job description
    And the tailoring step is skipped
    And if the base score is >= 70, a cover letter is generated
    And the result contains the base score, extracted keywords, and cover letter if generated

  Scenario: Score below 40 — structural mismatch, tailoring skipped
    Given the user profile contains a resume, skills, and accomplishments
    When the user supplies a job description
    And the best resume score is < 40
    Then go-apply skips the tailoring step
    And the result includes an advisory: "structural mismatch — tailoring cannot close this gap"
    And no cover letter is generated

  Scenario: Second run on the same URL uses the cached job description
    Given the job posting at a given URL was previously fetched
    When the user supplies the same URL again
    Then go-apply loads the job description from the local cache
    And does not make an HTTP request to the URL
    And the result indicates the job description was loaded from cache
    And scores all stored resumes against the cached job description

  Scenario: Multiple resumes — all scored — best one selected — pipeline continues
    Given the user profile contains resumes labeled "backend" and "frontend"
    And the profile contains skills and accomplishments
    When the user supplies a job description
    Then go-apply scores both resumes against the job description
    And the resume with the higher total score is selected as the best match
    And the result contains scores for both resumes
    And the tailoring and cover letter steps run on the best-matching resume

  Scenario Outline: Cover letter channel affects letter style
    When the user supplies a job description with channel "<channel>"
    And the best resume score is >= 70
    Then a cover letter is generated styled for the "<channel>" channel

    Examples:
      | channel   |
      | COLD      |
      | REFERRAL  |
      | RECRUITER |

  # ─────────────────────────────────────────────────────────────────────────────
  # Degraded paths
  # Orchestrator unavailable → INOPERABLE hard error.
  # Embedder unavailable → orchestrator performs keyword matching as fallback.
  # ─────────────────────────────────────────────────────────────────────────────

  Scenario: Pipeline inoperative — orchestrator unreachable
    Given the orchestrator LLM is unavailable
    When the user supplies a job description
    Then go-apply returns an error indicating the pipeline cannot run without the orchestrator
    And the result status is "error"
    And no scores or cover letter are returned

  Scenario: Embedder unavailable — orchestrator performs keyword matching as fallback
    Given the embedder endpoint is unavailable
    When the user supplies a job description
    Then go-apply cannot retrieve profile context via vector search
    And the orchestrator performs keyword matching to retrieve relevant profile chunks
    And uses the keyword-matched profile chunks to augment the resume for scoring
    And the pipeline completes and returns a score result

  Scenario: All resumes fail to load — pipeline returns an error
    Given no resume files can be read from disk
    When the user supplies a job description
    Then go-apply returns an error indicating all resumes failed to load or score
    And the result status is "error"

  # ─────────────────────────────────────────────────────────────────────────────
  # MCP-specific invocation
  # ─────────────────────────────────────────────────────────────────────────────

  @mcp
  Scenario: Score resume via get_score MCP tool with a URL
    When Claude invokes the get_score tool with a job posting URL
    Then go-apply runs the full pipeline
    And returns a JSON result with scores, extracted keywords, and (if score >= 70) a cover letter

  @mcp
  Scenario: Score resume via get_score MCP tool with raw text
    When Claude invokes the get_score tool with raw job description text
    Then go-apply runs the full pipeline
    And returns a JSON result with scores and extracted keywords

  @mcp
  Scenario: get_score rejects both url and text provided together
    When Claude invokes the get_score tool with both url and text arguments
    Then go-apply returns an error: "exactly one of url or text is required"

  @mcp
  Scenario: get_score rejects neither url nor text provided
    When Claude invokes the get_score tool with neither url nor text
    Then go-apply returns an error: "exactly one of url or text is required"

  # ─────────────────────────────────────────────────────────────────────────────
  # CLI-specific invocation
  # ─────────────────────────────────────────────────────────────────────────────

  @cli
  Scenario: Score resume via CLI with a URL
    When the user runs: go-apply run --url https://example.com/job
    Then go-apply runs the full pipeline
    And prints a JSON result to stdout

  @cli
  Scenario: Score resume via CLI with raw text
    When the user runs: go-apply run --text "Senior Go engineer wanted..."
    Then go-apply runs the full pipeline
    And prints a JSON result to stdout

  @cli
  Scenario: Tailor resume via CLI with accomplishments file
    When the user runs: go-apply run --url https://example.com/job --accomplishments accomplishments.md
    Then go-apply runs the full pipeline including the tailor step
    And prints a JSON result to stdout with base and tailored scores

  @cli
  Scenario: CLI rejects both --url and --text together
    When the user runs: go-apply run --url https://example.com/job --text "raw jd"
    Then go-apply returns an error: "--url and --text are mutually exclusive"

  @cli
  Scenario: CLI rejects neither --url nor --text
    When the user runs: go-apply run
    Then go-apply returns an error: "one of --url or --text is required"

  @cli
  Scenario: CLI rejects unknown channel value
    When the user runs: go-apply run --url https://example.com/job --channel UNKNOWN
    Then go-apply returns an error indicating valid values are COLD, REFERRAL, and RECRUITER

  # ─────────────────────────────────────────────────────────────────────────────
  # Future work
  # ─────────────────────────────────────────────────────────────────────────────

  @future
  Scenario: PDF export of tailored resume (future)
    Given the tailor step produces a tailored resume
    When the result is emitted
    Then the user receives a PDF version of the tailored resume alongside the JSON result

  @tui @future
  Scenario: TUI workflow screen (Epic 6)
    Given a user profile exists
    When the user runs go-apply in a terminal without --headless
    Then the TUI main screen is displayed
    And the user can input a job posting URL or paste raw job description text
    And the pipeline progress is shown step by step in the TUI
    And the final result is displayed in the TUI including scores and cover letter

  @future
  Scenario: User confirms whether they applied (future)
    Given the pipeline result has been shown to the user
    When the user is asked whether they submitted an application
    Then the application record is updated with the user's response
    And the record is stored for future reference
