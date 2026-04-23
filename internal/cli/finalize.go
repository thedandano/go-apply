package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

// NewFinalizeCommand returns the cobra command for "go-apply finalize".
// Persists the application record and deletes the session file on success.
func NewFinalizeCommand() *cobra.Command {
	var sessionID string
	var coverLetterFile string

	cmd := &cobra.Command{
		Use:   "finalize",
		Short: "Finalize a session: persist the application record and clean up",
		Long: `finalize persists the application record and deletes the session file on success.
Requires --session. --cover-letter-file is optional.
Writes summary JSON to stdout on success.
Writes {"status":"error","code":"...","message":"..."} to stderr and exits non-zero on error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFinalize(cmd.Context(), sessionID, coverLetterFile)
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "Session ID returned by load-jd")
	cmd.Flags().StringVar(&coverLetterFile, "cover-letter-file", "", "Path to the cover letter text file (optional)")
	_ = cmd.MarkFlagRequired("session")

	return cmd
}

func runFinalize(ctx context.Context, sessionID, coverLetterFile string) error {
	coverLetter := ""
	if coverLetterFile != "" {
		data, err := os.ReadFile(coverLetterFile)
		if err != nil {
			return writeError("file_read_error", fmt.Sprintf("read cover letter file: %v", err))
		}
		coverLetter = string(data)
	}

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
			Name: "finalize",
			Arguments: map[string]any{
				"session_id":   sessionID,
				"cover_letter": coverLetter,
			},
		},
	}

	result := mcpserver.HandleFinalizeWithConfig(ctx, &req, deps, store)
	err = writeHeadlessResult(result, func(env map[string]any) (any, error) {
		return env["data"], nil
	})
	if err != nil {
		return err
	}

	// Delete the session file on success (finalize is terminal).
	if delErr := store.Delete(ctx, sessionID); delErr != nil {
		// Non-fatal: log but don't fail the command.
		fmt.Fprintf(os.Stderr, `{"status":"warn","code":"session_cleanup_failed","message":%q}`+"\n",
			delErr.Error())
	}
	return nil
}
