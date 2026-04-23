package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

// NewSubmitTailoredResumeCommand returns the cobra command for "go-apply submit-tailored-resume".
// Reads tailored resume text from a file, optionally reads a changelog file, and writes scores JSON to stdout.
func NewSubmitTailoredResumeCommand() *cobra.Command {
	var sessionID string
	var tailoredTextFile string
	var changelogFile string

	cmd := &cobra.Command{
		Use:   "submit-tailored-resume",
		Short: "Submit a tailored resume for rescoring",
		Long: `submit-tailored-resume reads the tailored resume from a file and rescores it.
Requires --session and --tailored-text-file. --changelog-file is optional.
Writes rescore JSON to stdout on success.
Writes {"status":"error","code":"...","message":"..."} to stderr and exits non-zero on error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSubmitTailoredResume(cmd.Context(), sessionID, tailoredTextFile, changelogFile)
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID returned by load-jd")
	cmd.Flags().StringVar(&tailoredTextFile, "tailored-text-file", "", "Path to the tailored resume text file")
	cmd.Flags().StringVar(&changelogFile, "changelog-file", "", "Path to the JSON changelog file (optional)")
	_ = cmd.MarkFlagRequired("session")
	_ = cmd.MarkFlagRequired("tailored-text-file")

	return cmd
}

func runSubmitTailoredResume(ctx context.Context, sessionID, tailoredTextFile, changelogFile string) error {
	tailoredBytes, err := os.ReadFile(tailoredTextFile) // #nosec G304 -- path explicitly provided by the user via --tailored-text-file flag
	if err != nil {
		return writeError("file_read_error", fmt.Sprintf("read tailored text file: %v", err))
	}
	tailoredText := string(tailoredBytes)

	changelogJSON := ""
	if changelogFile != "" {
		changelogBytes, err := os.ReadFile(changelogFile) // #nosec G304 -- path explicitly provided by the user via --changelog-file flag
		if err != nil {
			return writeError("file_read_error", fmt.Sprintf("read changelog file: %v", err))
		}
		changelogJSON = string(changelogBytes)
	}

	store, err := openDiskStore()
	if err != nil {
		return writeError("store_error", err.Error())
	}

	deps, err := loadCLIDeps()
	if err != nil {
		return writeError("config_error", err.Error())
	}

	args := map[string]any{
		"session_id":    sessionID,
		"tailored_text": tailoredText,
	}
	if changelogJSON != "" {
		args["changelog"] = changelogJSON
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "submit_tailored_resume", Arguments: args},
	}

	result := mcpserver.HandleSubmitTailoredResumeWithConfig(ctx, &req, deps, nil, store)
	return writeHeadlessResult(result, func(env map[string]any) (any, error) {
		return env["data"], nil
	})
}
