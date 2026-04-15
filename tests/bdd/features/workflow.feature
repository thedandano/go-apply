# Workflow Feature
#
# Covers the full job application pipeline:
#   fetch/text → keyword extraction → score resumes → tailor (optional) → cover letter → result
#
# Tags:
#   @mcp    — invoked via Claude using the get_score MCP tool
#   @cli    — invoked via go-apply run
#   @future — not yet implemented
#
# Score threshold (configurable via defaults.json): 70.0
# Tailor step: only runs when --accomplishments / accomplishments_text is provided.
#              Tier-1 (keyword injection) always runs.
#              Tier-2 (bullet rewriting) always runs when accomplishments are provided.
# Augmentation: profile context is retrieved and blended into each resume before scoring.
#               Falls back to keyword matching if vector search fails.
#               Falls back to original resume text if both retrieval methods fail.

Feature: Job Application Workflow

  Background:
    Given a user profile exists with at least one stored resume
    And go-apply is configured with an orchestrator model and endpoint

  # ─────────────────────────────────────────────────────────────────────────────
  # Base scoring — no tailoring
  # ─────────────────────────────────────────────────────────────────────────────

  Scenario: URL input — score meets threshold — cover letter generated
    When the user supplies a job posting URL
    Then go-apply fetches the job description
    And extracts required and preferred keywords
    And scores all stored resumes against the job description
    And the best resume score is >= 70
    Then a cover letter is generated for the best-matching resume
    And the result contains the score breakdown, extracted keywords, and cover letter
    And the job description is saved to the application record

  Scenario: URL input — score below threshold — no cover letter
    When the user supplies a job posting URL
    Then go-apply fetches the job description
    And scores all stored resumes against the job description
    And the best resume score is < 70
    Then no cover letter is generated
    And the result contains the score breakdown and extracted keywords
    And the job description is saved to the application record

  Scenario: Text input — score meets threshold — cover letter generated
    When the user supplies raw job description text
    Then go-apply scores all stored resumes against the job description
    And the best resume score is >= 70
    Then a cover letter is generated for the best-matching resume
    And the result contains the score breakdown, extracted keywords, and cover letter

  Scenario: Text input — score below threshold — no cover letter
    When the user supplies raw job description text
    Then go-apply scores all stored resumes against the job description
    And the best resume score is < 70
    Then no cover letter is generated
    And the result contains the score breakdown and extracted keywords

  Scenario: Second run on the same URL uses the cached job description
    Given the job posting at a given URL was previously fetched
    When the user supplies the same URL again
    Then go-apply loads the job description from the local cache
    And does not make an HTTP request to the URL
    And scores all stored resumes against the cached job description

  Scenario: Multiple resumes — all scored — best one selected
    Given the user profile contains resumes labeled "backend" and "frontend"
    When the user supplies a job description
    Then go-apply scores both resumes against the job description
    And the resume with the higher total score is selected as the best match
    And the result contains scores for both resumes

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
  # With tailoring (accomplishments provided)
  # Tier-1 (keyword injection into Skills section) always runs.
  # Tier-2 (bullet rewriting in Experience section) always runs when accomplishments are provided.
  # The best score is updated if the tailored score is higher.
  # ─────────────────────────────────────────────────────────────────────────────

  Scenario: Tailoring — tailored score meets threshold — cover letter generated
    Given accomplishments text is provided
    When the user supplies a job description
    Then go-apply scores all resumes against the job description
    And the tailor step runs on the best-matching resume:
      | tier | action                                              |
      | T1   | injects missing JD keywords into the Skills section |
      | T2   | rewrites relevant Experience bullets grounded in accomplishments |
    And the tailored resume is re-scored
    And the tailored score is >= 70
    Then a cover letter is generated for the tailored resume
    And the result contains the base score, tailored score, and cover letter

  Scenario: Tailoring — tailored score still below threshold — no cover letter
    Given accomplishments text is provided
    When the user supplies a job description
    Then go-apply scores all resumes against the job description
    And the tailor step runs on the best-matching resume
    And the tailored resume is re-scored
    And the tailored score is < 70
    Then no cover letter is generated
    And the result contains the base score and tailored score

  Scenario: No accomplishments provided — tailor step is skipped
    Given no accomplishments text is provided
    When the user supplies a job description
    Then go-apply scores all resumes against the job description
    And the tailor step does not run
    And the result contains only base scores

  # ─────────────────────────────────────────────────────────────────────────────
  # Degraded paths
  # The pipeline never aborts on non-fatal failures — it degrades and continues.
  # ─────────────────────────────────────────────────────────────────────────────

  Scenario: Keyword extraction fails — pipeline continues in degraded mode
    Given the orchestrator LLM is unavailable
    When the user supplies a job description
    Then go-apply cannot extract structured keywords from the job description
    And still scores all stored resumes using whatever job text is available
    And the result status is "degraded"
    And the result includes a message explaining the keyword extraction failure

  Scenario: Profile context retrieval fails — original resume text used for scoring
    Given the embedder endpoint is unavailable
    When the user supplies a job description
    Then go-apply cannot retrieve profile context via vector search
    And falls back to keyword-based profile retrieval
    And if keyword retrieval also fails, scores the original resume text without augmentation
    And the pipeline completes and returns a score result

  Scenario: Vector search falls back to keyword matching when embedder is unavailable
    Given the embedder endpoint is unavailable
    When the user supplies a job description
    Then go-apply attempts vector search for profile context
    And falls back to keyword matching when vector search fails
    And uses the keyword-matched profile chunks to augment the resume for scoring

  Scenario: All resumes fail to load — pipeline returns an error
    Given no resume files can be read from disk
    When the user supplies a job description
    Then go-apply returns an error indicating all resumes failed to load or score
    And the result status is "error"

  Scenario: Tailor tier-2 LLM call fails — result degrades to tier-1
    Given accomplishments text is provided
    And the orchestrator LLM fails during bullet rewriting
    When the user supplies a job description
    Then tier-1 keyword injection completes successfully
    And tier-2 bullet rewriting is skipped for affected bullets
    And the result contains the tier-1 tailored resume
    And a warning is recorded for the tier-2 failure

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
