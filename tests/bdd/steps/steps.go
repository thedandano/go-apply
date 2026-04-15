//go:build bdd

package steps

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

// InitializeScenario registers all step definitions on the provided ScenarioContext.
// Called once per scenario; s is freshly allocated for each scenario.
func InitializeScenario(ctx *godog.ScenarioContext) {
	s := &bddState{}

	ctx.Before(func(goCtx context.Context, sc *godog.Scenario) (context.Context, error) {
		var err error
		s.binary, err = buildBinary()
		if err != nil {
			return goCtx, err
		}
		s.tmpHome, err = os.MkdirTemp("", "go-apply-bdd-home-*")
		if err != nil {
			return goCtx, err
		}
		// Pre-create XDG directories so the binary can open its databases.
		for _, dir := range []string{
			filepath.Join(s.tmpHome, ".config", "go-apply"),
			filepath.Join(s.tmpHome, ".local", "share", "go-apply"),
			filepath.Join(s.tmpHome, ".local", "state", "go-apply", "logs"),
		} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return goCtx, err
			}
		}
		s.stubServer = newEmbedderStub()
		s.stubURL = s.stubServer.URL
		s.writeConfig(nil)
		return goCtx, nil
	})

	ctx.After(func(goCtx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if s.stubServer != nil {
			s.stubServer.Close()
		}
		if s.tmpHome != "" {
			os.RemoveAll(s.tmpHome) //nolint:errcheck
		}
		return goCtx, nil
	})

	// ── Onboarding: Given ──────────────────────────────────────────────────
	ctx.Given(`^no user profile exists$`, s.noUserProfileExists)
	ctx.Given(`^a resume labeled "([^"]*)" already exists$`, s.resumeLabeledExists)
	ctx.Given(`^the orchestrator API key is set$`, s.orchestratorAPIKeyIsSet)
	ctx.Given(`^the orchestrator API key is not set$`, s.orchestratorAPIKeyNotSet)

	// ── Onboarding: When (MCP) ─────────────────────────────────────────────
	ctx.When(`^Claude invokes the onboard_user tool with resume_content "([^"]*)" and resume_label "([^"]*)"$`, s.invokeOnboardUserWithResume)
	ctx.When(`^Claude invokes the onboard_user tool with:$`, s.invokeOnboardUserTable)
	ctx.When(`^Claude invokes the onboard_user tool with skills "([^"]*)" only$`, s.invokeOnboardUserSkillsOnly)
	ctx.When(`^Claude invokes the onboard_user tool with accomplishments text only$`, s.invokeOnboardUserAccomplishmentsOnly)
	ctx.When(`^Claude invokes the onboard_user tool with resume_content but no resume_label$`, s.invokeOnboardUserContentNoLabel)
	ctx.When(`^Claude invokes the onboard_user tool with resume_label but no resume_content$`, s.invokeOnboardUserLabelNoContent)
	ctx.When(`^Claude invokes the onboard_user tool with no arguments$`, s.invokeOnboardUserNoArgs)
	ctx.When(`^Claude invokes the add_resume tool with resume_content "([^"]*)" and resume_label "([^"]*)"$`, s.invokeAddResume)
	ctx.When(`^Claude invokes the add_resume tool with resume_label but no resume_content$`, s.invokeAddResumeNoContent)
	ctx.When(`^Claude invokes the update_config tool with key "([^"]*)" and value "([^"]*)"$`, s.invokeUpdateConfig)
	ctx.When(`^Claude invokes the update_config tool with key "([^"]*)" and any value$`, s.invokeUpdateConfigAnyValue)
	ctx.When(`^Claude invokes the get_config tool$`, s.invokeGetConfig)

	// ── Onboarding: When (CLI) ─────────────────────────────────────────────
	ctx.When(`^the user runs:$`, s.cliOnboardDocString)
	ctx.When(`^the user runs: go-apply onboard --resume (\S+)$`, s.cliOnboardResume)
	ctx.When(`^the user runs: go-apply onboard --resume (\S+) --resume (\S+)$`, s.cliOnboardTwoResumes)
	ctx.When(`^the user runs: go-apply onboard$`, s.cliOnboardNoFlags)
	ctx.When(`^the user runs: go-apply config set (\S+) (\S+)$`, s.cliConfigSet)
	ctx.When(`^the user runs: go-apply config show$`, s.cliConfigShow)

	// ── Onboarding: Then ──────────────────────────────────────────────────
	ctx.Then(`^go-apply stores the resume under the label "([^"]*)"$`, s.assertGoApplyStoresResumeLabel)
	ctx.Then(`^go-apply stores the resume, skills, and accomplishments$`, s.assertGoApplyStoresAll)
	ctx.Then(`^go-apply stores the skills reference$`, s.assertGoApplyStoresSkills)
	ctx.Then(`^go-apply stores the accomplishments$`, s.assertGoApplyStoresAccomplishments)
	ctx.Then(`^the response lists "([^"]*)" as stored$`, s.assertResponseLists)
	ctx.Then(`^the response lists "([^"]*)", "([^"]*)", and "([^"]*)" as stored$`, s.assertResponseListsThree)
	ctx.Then(`^go-apply replaces the existing "([^"]*)" resume$`, s.assertResumeReplaced)
	ctx.Then(`^go-apply returns an error: "([^"]*)"$`, s.assertError)
	ctx.Then(`^go-apply returns an error: '([^']*)'$`, s.assertErrorSingleQuote)
	ctx.Then(`^go-apply returns an error containing "([^"]*)"$`, s.assertErrorContaining)
	ctx.Then(`^go-apply saves the config$`, s.assertConfigSaved)
	ctx.Then(`^go-apply saves the API key$`, s.assertConfigSaved)
	ctx.Then(`^go-apply saves both settings$`, s.assertConfigSaved)
	ctx.Then(`^the response confirms key "([^"]*)" was updated$`, s.assertConfigKeyUpdated)
	ctx.Then(`^the response shows value "([^"]*)" instead of the plaintext key$`, s.assertValueRedacted)
	ctx.Then(`^go-apply returns all user-facing configuration fields$`, s.assertAllConfigFields)
	ctx.Then(`^the orchestrator\.api_key field is shown as "([^"]*)"$`, s.assertOrchestratorAPIKey)
	ctx.Then(`^the embedder\.api_key field is shown as "([^"]*)"$`, s.assertEmbedderAPIKey)
	ctx.Then(`^the orchestrator\.api_key field is shown as an empty string$`, s.assertOrchestratorAPIKeyEmpty)
	ctx.Then(`^prints a JSON result listing all stored keys$`, s.assertJSONResultStored)
	ctx.Then(`^prints a JSON result listing "([^"]*)" as stored$`, s.assertJSONResultKey)
	ctx.Then(`^prints a JSON result listing both stored keys$`, s.assertJSONResultBothKeys)
	ctx.Then(`^prints a confirmation of the updated key and value$`, s.assertConfigConfirmation)
	ctx.Then(`^the output contains all user-facing fields$`, s.assertAllUserFacingFields)
	ctx.Then(`^no resumes are stored$`, s.assertNoResumesStored)
	ctx.Then(`^the resume is stored under the label "([^"]*)"$`, s.assertResumeStoredLabel)
	ctx.Then(`^stores the resume under the label "([^"]*)"$`, s.assertStoresResumeLabel)
	ctx.Then(`^stores the skills and accomplishments$`, s.assertStoresSkillsAndAccomplishments)
	ctx.Then(`^go-apply loads each file$`, s.assertLoadsEachFile)
	ctx.Then(`^go-apply stores both resumes under labels "([^"]*)" and "([^"]*)"$`, s.assertStoresBothResumes)

	// ── Workflow: Given ───────────────────────────────────────────────────
	ctx.Given(`^a user profile exists with at least one stored resume$`, s.profileWithResume)
	ctx.Given(`^go-apply is configured with an orchestrator model and endpoint$`, s.configuredWithStub)
	ctx.Given(`^the job posting at a given URL was previously fetched$`, s.jobPreviouslyFetched)
	ctx.Given(`^the user profile contains resumes labeled "([^"]*)" and "([^"]*)"$`, s.profileWithTwoResumes)
	ctx.Given(`^accomplishments text is provided$`, s.accomplishmentsProvided)
	ctx.Given(`^no accomplishments text is provided$`, s.noAccomplishments)
	ctx.Given(`^the orchestrator LLM is unavailable$`, s.orchestratorUnavailable)
	ctx.Given(`^the orchestrator LLM fails during bullet rewriting$`, s.orchestratorFailsDuringRewrite)
	ctx.Given(`^the embedder endpoint is unavailable$`, s.embedderUnavailable)
	ctx.Given(`^no resume files can be read from disk$`, s.noResumesOnDisk)
	ctx.Given(`^a user profile exists$`, s.profileExists)

	// ── Workflow: When ────────────────────────────────────────────────────
	ctx.When(`^the user supplies a job posting URL$`, s.userSuppliesURL)
	ctx.When(`^the user supplies raw job description text$`, s.userSuppliesText)
	ctx.When(`^the user supplies the same URL again$`, s.userSuppliesSameURL)
	ctx.When(`^the user supplies a job description$`, s.userSuppliesText)
	ctx.When(`^the user supplies a job description with channel "([^"]*)"$`, s.userSuppliesTextWithChannel)
	ctx.When(`^Claude invokes the get_score tool with a job posting URL$`, s.mcpGetScoreURL)
	ctx.When(`^Claude invokes the get_score tool with raw job description text$`, s.mcpGetScoreText)
	ctx.When(`^Claude invokes the get_score tool with both url and text arguments$`, s.mcpGetScoreBothArgs)
	ctx.When(`^Claude invokes the get_score tool with neither url nor text$`, s.mcpGetScoreNoArgs)
	ctx.When(`^the user runs: go-apply run --url (\S+)$`, s.cliRunURL)
	ctx.When(`^the user runs: go-apply run --text "([^"]*)"$`, s.cliRunText)
	ctx.When(`^the user runs: go-apply run --url (\S+) --accomplishments (\S+)$`, s.cliRunURLWithAccomplishments)
	ctx.When(`^the user runs: go-apply run --url (\S+) --text "([^"]*)"$`, s.cliRunBothFlags)
	ctx.When(`^the user runs: go-apply run$`, s.cliRunNoArgs)
	ctx.When(`^the user runs: go-apply run --url (\S+) --channel (\S+)$`, s.cliRunUnknownChannel)

	// ── Workflow: Then ────────────────────────────────────────────────────
	ctx.Then(`^go-apply fetches the job description$`, s.assertJobFetched)
	ctx.Then(`^extracts required and preferred keywords$`, s.assertKeywordsExtracted)
	ctx.Then(`^scores all stored resumes against the job description$`, s.assertAllScored)
	ctx.Then(`^scores all stored resumes against the cached job description$`, s.assertAllScored)
	ctx.Then(`^the best resume score is >= (\d+)$`, s.assertScoreMeetsThreshold)
	ctx.Then(`^the best resume score is < (\d+)$`, s.assertScoreBelowThreshold)
	ctx.Then(`^a cover letter is generated for the best-matching resume$`, s.assertCoverLetterGenerated)
	ctx.Then(`^a cover letter is generated for the tailored resume$`, s.assertCoverLetterGenerated)
	ctx.Then(`^no cover letter is generated$`, s.assertNoCoverLetter)
	ctx.Then(`^a cover letter is generated styled for the "([^"]*)" channel$`, s.assertCoverLetterChannel)
	ctx.Then(`^the result contains the score breakdown, extracted keywords, and cover letter$`, s.assertFullResult)
	ctx.Then(`^the result contains the score breakdown and extracted keywords$`, s.assertPartialResult)
	ctx.Then(`^the job description is saved to the application record$`, s.assertJDSaved)
	ctx.Then(`^go-apply loads the job description from the local cache$`, s.assertLoadedFromCache)
	ctx.Then(`^does not make an HTTP request to the URL$`, s.assertNoHTTPRequest)
	ctx.Then(`^the resume with the higher total score is selected as the best match$`, s.assertBestResumeSelected)
	ctx.Then(`^the result contains scores for both resumes$`, s.assertBothScores)
	ctx.Then(`^the tailor step runs on the best-matching resume:$`, s.assertTailorRanWithTable)
	ctx.Then(`^the tailor step runs on the best-matching resume$`, s.assertTailorRan)
	ctx.Then(`^the tailored resume is re-scored$`, s.assertTailoredReScored)
	ctx.Then(`^the tailored score is >= (\d+)$`, s.assertTailoredMeetsThreshold)
	ctx.Then(`^the tailored score is < (\d+)$`, s.assertTailoredBelowThreshold)
	ctx.Then(`^the result contains the base score, tailored score, and cover letter$`, s.assertTailoredFullResult)
	ctx.Then(`^the result contains the base score and tailored score$`, s.assertTailoredPartialResult)
	ctx.Then(`^the tailor step does not run$`, s.assertTailorSkipped)
	ctx.Then(`^the result contains only base scores$`, s.assertOnlyBaseScores)
	ctx.Then(`^go-apply cannot extract structured keywords from the job description$`, s.assertKeywordExtractionFailed)
	ctx.Then(`^still scores all stored resumes using whatever job text is available$`, s.assertScoredDespiteFailure)
	ctx.Then(`^the result status is "([^"]*)"$`, s.assertResultStatus)
	ctx.Then(`^the result includes a message explaining the keyword extraction failure$`, s.assertDegradedMessage)
	ctx.Then(`^go-apply cannot retrieve profile context via vector search$`, s.assertVectorSearchFailed)
	ctx.Then(`^falls back to keyword-based profile retrieval$`, s.assertKeywordFallback)
	ctx.Then(`^if keyword retrieval also fails, scores the original resume text without augmentation$`, s.assertOriginalTextFallback)
	ctx.Then(`^the pipeline completes and returns a score result$`, s.assertPipelineCompletes)
	ctx.Then(`^go-apply attempts vector search for profile context$`, s.assertVectorSearchAttempted)
	ctx.Then(`^falls back to keyword matching when vector search fails$`, s.assertKeywordMatchFallback)
	ctx.Then(`^uses the keyword-matched profile chunks to augment the resume for scoring$`, s.assertAugmentedWithKeywords)
	ctx.Then(`^go-apply returns an error indicating all resumes failed to load or score$`, s.assertAllResumesFailed)
	ctx.Then(`^tier-1 keyword injection completes successfully$`, s.assertTier1Succeeded)
	ctx.Then(`^tier-2 bullet rewriting is skipped for affected bullets$`, s.assertTier2Skipped)
	ctx.Then(`^the result contains the tier-1 tailored resume$`, s.assertTier1Result)
	ctx.Then(`^a warning is recorded for the tier-2 failure$`, s.assertTier2Warning)
	ctx.Then(`^go-apply runs the full pipeline$`, s.assertPipelineRan)
	ctx.Then(`^go-apply runs the full pipeline including the tailor step$`, s.assertPipelineRan)
	ctx.Then(`^returns a JSON result with scores, extracted keywords, and \(if score >= 70\) a cover letter$`, s.assertMCPResult)
	ctx.Then(`^returns a JSON result with scores and extracted keywords$`, s.assertMCPPartialResult)
	ctx.Then(`^prints a JSON result to stdout$`, s.assertJSONStdout)
	ctx.Then(`^prints a JSON result to stdout with base and tailored scores$`, s.assertJSONWithTailoredScores)
	ctx.Then(`^go-apply returns an error indicating valid values are COLD, REFERRAL, and RECRUITER$`, s.assertChannelError)
	ctx.Then(`^go-apply scores all stored resumes against the job description$`, s.assertAllScored)
	ctx.Then(`^go-apply scores both resumes against the job description$`, s.assertAllScored)
	ctx.Then(`^go-apply scores all resumes against the job description$`, s.assertAllScored)
}
