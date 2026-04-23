package cli

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

// NewScoreCommand returns the cobra command for "go-apply score".
// Accepts --session and --jd-json, writes scores JSON to stdout.
func NewScoreCommand() *cobra.Command {
	var sessionID string
	var jdJSON string

	cmd := &cobra.Command{
		Use:   "score",
		Short: "Score resumes against a job description for an existing session",
		Long: `score submits extracted JD keywords and scores all resumes.
Requires --session (from load-jd) and --jd-json (structured JD keywords).
Writes scores JSON to stdout on success.
Writes {"status":"error","code":"...","message":"..."} to stderr and exits non-zero on error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScore(cmd.Context(), sessionID, jdJSON)
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID returned by load-jd")
	cmd.Flags().StringVar(&jdJSON, "jd-json", "", "JSON-encoded JDData with title, company, required, preferred, etc.")
	_ = cmd.MarkFlagRequired("session")
	_ = cmd.MarkFlagRequired("jd-json")

	return cmd
}

func runScore(ctx context.Context, sessionID, jdJSON string) error {
	store, err := openDiskStore()
	if err != nil {
		return writeError("store_error", err.Error())
	}

	deps, err := loadCLIDeps()
	if err != nil {
		return writeError("config_error", err.Error())
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "submit_keywords",
			Arguments: map[string]any{
				"session_id": sessionID,
				"jd_json":    jdJSON,
			},
		},
	}

	result := mcpserver.HandleSubmitKeywordsWithConfig(ctx, &req, deps, nil, store)
	return writeHeadlessResult(result, func(env map[string]any) (any, error) {
		return env["data"], nil
	})
}
