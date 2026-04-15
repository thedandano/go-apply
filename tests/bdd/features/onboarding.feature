# Onboarding Feature
#
# Covers storing resumes, skills, and accomplishments into the profile database,
# and configuring the orchestrator/embedder endpoints and models.
#
# Modes:
#   @mcp  — via Claude invoking MCP tools (onboard_user, add_resume, update_config, get_config)
#   @cli  — via go-apply onboard and go-apply config subcommands
#   @tui  — TUI wizard (Epic 6, not yet implemented)
#   @future — not yet implemented

Feature: User Onboarding

  # ─────────────────────────────────────────────────────────────────────────────
  # MCP
  # When using go-apply via Claude Code, Claude is the wizard.
  # go-apply exposes tools; Claude drives the conversation and invokes them.
  # ─────────────────────────────────────────────────────────────────────────────

  @mcp
  Scenario: Store a resume via onboard_user
    Given no user profile exists
    When Claude invokes the onboard_user tool with resume_content "Go engineer resume" and resume_label "backend"
    Then go-apply stores the resume under the label "backend"
    And the response lists "resume:backend" as stored

  @mcp
  Scenario: Store resume, skills, and accomplishments via onboard_user
    Given no user profile exists
    When Claude invokes the onboard_user tool with:
      | resume_content  | Go engineer resume             |
      | resume_label    | backend                        |
      | skills          | Go, Python, Docker             |
      | accomplishments | Led team of 5 for 2 years      |
    Then go-apply stores the resume, skills, and accomplishments
    And the response lists "resume:backend", "ref:skills", and "accomplishments" as stored

  @mcp
  Scenario: Store skills only via onboard_user
    Given no user profile exists
    When Claude invokes the onboard_user tool with skills "Go, Python, Docker" only
    Then go-apply stores the skills reference
    And the response lists "ref:skills" as stored

  @mcp
  Scenario: Store accomplishments only via onboard_user
    Given no user profile exists
    When Claude invokes the onboard_user tool with accomplishments text only
    Then go-apply stores the accomplishments
    And the response lists "accomplishments" as stored

  @mcp
  Scenario: Add or replace a resume via add_resume
    Given a resume labeled "backend" already exists
    When Claude invokes the add_resume tool with resume_content "Updated resume" and resume_label "backend"
    Then go-apply replaces the existing "backend" resume
    And the response lists "resume:backend" as stored

  @mcp
  Scenario: resume_label missing when resume_content is provided
    When Claude invokes the onboard_user tool with resume_content but no resume_label
    Then go-apply returns an error: "resume_content and resume_label must both be provided or both omitted"

  @mcp
  Scenario: resume_content missing when resume_label is provided
    When Claude invokes the onboard_user tool with resume_label but no resume_content
    Then go-apply returns an error: "resume_content and resume_label must both be provided or both omitted"

  @mcp
  Scenario: No input provided to onboard_user
    When Claude invokes the onboard_user tool with no arguments
    Then go-apply returns an error: "at least one of resume_content, skills, or accomplishments is required"

  @mcp
  Scenario: add_resume rejects missing resume_content
    When Claude invokes the add_resume tool with resume_label but no resume_content
    Then go-apply returns an error: "resume_content and resume_label are both required"

  @mcp
  Scenario: Configure orchestrator model and endpoint via update_config
    When Claude invokes the update_config tool with key "orchestrator.model" and value "claude-opus-4-6"
    Then go-apply saves the config
    And the response confirms key "orchestrator.model" was updated

  @mcp
  Scenario: API key value is redacted in update_config response
    When Claude invokes the update_config tool with key "orchestrator.api_key" and value "sk-super-secret"
    Then go-apply saves the API key
    And the response shows value "***" instead of the plaintext key

  @mcp
  Scenario: Internal fields cannot be set via update_config
    When Claude invokes the update_config tool with key "db_path" and any value
    Then go-apply returns an error: 'unknown config key "db_path"'

  @mcp
  Scenario: View current configuration via get_config
    Given the orchestrator API key is set
    When Claude invokes the get_config tool
    Then go-apply returns all user-facing configuration fields
    And the orchestrator.api_key field is shown as "***"
    And the embedder.api_key field is shown as "***"

  @mcp
  Scenario: Empty API key is not redacted in get_config response
    Given the orchestrator API key is not set
    When Claude invokes the get_config tool
    Then the orchestrator.api_key field is shown as an empty string

  # ─────────────────────────────────────────────────────────────────────────────
  # CLI
  # go-apply onboard is a one-shot command (not interactive).
  # go-apply config set/get/show manage configuration.
  # ─────────────────────────────────────────────────────────────────────────────

  @cli
  Scenario: Onboard with resume, skills, and accomplishments via CLI
    Given no user profile exists
    When the user runs:
      """
      go-apply onboard --resume backend.pdf --skills skills.md --accomplishments accomplishments.md
      """
    Then go-apply loads each file
    And stores the resume under the label "backend"
    And stores the skills and accomplishments
    And prints a JSON result listing all stored keys

  @cli
  Scenario: Onboard with resume only via CLI
    Given no user profile exists
    When the user runs: go-apply onboard --resume backend.pdf
    Then go-apply stores the resume under the label "backend"
    And prints a JSON result listing "resume:backend" as stored

  @cli
  Scenario: Onboard with multiple resumes via CLI
    Given no user profile exists
    When the user runs: go-apply onboard --resume backend.pdf --resume frontend.pdf
    Then go-apply stores both resumes under labels "backend" and "frontend"
    And prints a JSON result listing both stored keys

  @cli
  Scenario: Duplicate resume label is rejected
    When the user runs: go-apply onboard --resume backend.pdf --resume ./other/backend.pdf
    Then go-apply returns an error containing "duplicate resume label"
    And no resumes are stored

  @cli
  Scenario: No flags provided to go-apply onboard
    When the user runs: go-apply onboard
    Then go-apply returns an error: "at least one of --resume, --skills, or --accomplishments is required"

  @cli
  Scenario: Label is derived from the filename stem
    When the user runs: go-apply onboard --resume my-resume-2024.pdf
    Then the resume is stored under the label "my-resume-2024"

  @cli
  Scenario: Set orchestrator model via CLI config
    When the user runs: go-apply config set orchestrator.model claude-opus-4-6
    Then go-apply saves the config
    And prints a confirmation of the updated key and value

  @cli
  Scenario: Set orchestrator endpoint via CLI config
    When the user runs: go-apply config set orchestrator.base_url https://api.anthropic.com/v1
    Then go-apply saves the config
    And prints a confirmation of the updated key and value

  @cli
  Scenario: Set embedding model and endpoint via CLI config
    When the user runs: go-apply config set embedder.model nomic-embed-text
    And the user runs: go-apply config set embedder.base_url http://localhost:11434/v1
    Then go-apply saves both settings

  @cli
  Scenario: Internal fields cannot be set via CLI config
    When the user runs: go-apply config set db_path /custom/path
    Then go-apply returns an error: 'unknown config key "db_path"'

  @cli
  Scenario: Show configuration via CLI redacts API keys
    Given the orchestrator API key is set
    When the user runs: go-apply config show
    Then the output contains all user-facing fields
    And the orchestrator.api_key field is shown as "***"

  @cli
  Scenario: Show configuration via CLI does not redact empty API keys
    Given the orchestrator API key is not set
    When the user runs: go-apply config show
    Then the orchestrator.api_key field is shown as an empty string

  # ─────────────────────────────────────────────────────────────────────────────
  # TUI — Epic 6, not yet implemented
  # ─────────────────────────────────────────────────────────────────────────────

  @tui @future
  Scenario: TUI onboarding wizard loads when no profile exists (Epic 6)
    Given no user profile exists
    When the user runs go-apply in a terminal (without --headless)
    Then a TUI onboarding wizard screen is displayed
    And the wizard prompts for one or more resume files
    And the wizard prompts for a skills reference file
    And the wizard prompts for an accomplishments file
    And the wizard prompts for the orchestrator model endpoint and name
    And the wizard prompts for the embedding model endpoint and name
    And on completion go-apply stores all provided inputs into the profile database
    And the wizard transitions to the main TUI screen
