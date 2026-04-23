package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

// NewLoadJDCommand returns the cobra command for "go-apply load-jd".
// Accepts --url or --text, writes {session_id, jd_text} JSON to stdout.
// Rejects --session if passed (unsupported_flag).
func NewLoadJDCommand() *cobra.Command {
	var jdURL string
	var jdText string

	var sessionFlagSentinel string

	cmd := &cobra.Command{
		Use:   "load-jd",
		Short: "Load a job description from a URL or raw text and create a session",
		Long: `load-jd fetches or accepts a job description and creates a disk-backed session.
Exactly one of --url or --text must be provided.
Writes {"session_id":"...", "jd_text":"..."} to stdout on success.
Writes {"status":"error","code":"...","message":"..."} to stderr and exits non-zero on error.`,
		Args: cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if sessionFlagSentinel != "" {
				return writeError("unsupported_flag", "--session is not supported by load-jd; session IDs are generated automatically")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLoadJD(cmd.Context(), jdURL, jdText)
		},
	}

	cmd.Flags().StringVar(&jdURL, "url", "", "URL of the job posting to fetch")
	cmd.Flags().StringVar(&jdText, "text", "", "Raw job description text")
	// --session is defined as a hidden flag that is explicitly rejected in PreRunE.
	// This produces a structured error (unsupported_flag) rather than cobra's generic "unknown flag" message.
	cmd.Flags().StringVar(&sessionFlagSentinel, "session", "", "")
	_ = cmd.Flags().MarkHidden("session")

	return cmd
}

func runLoadJD(ctx context.Context, jdURL, jdText string) error {
	if (jdURL != "") == (jdText != "") {
		return writeError("invalid_input", "exactly one of --url or --text is required")
	}

	store, err := openDiskStore()
	if err != nil {
		return writeError("store_error", err.Error())
	}

	deps, err := loadCLIDeps()
	if err != nil {
		return writeError("config_error", err.Error())
	}

	// Build a synthetic MCP request to reuse the shared handler.
	args := map[string]any{}
	if jdURL != "" {
		args["jd_url"] = jdURL
	} else {
		args["jd_raw_text"] = jdText
	}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "load_jd", Arguments: args},
	}

	result := mcpserver.HandleLoadJDWithConfig(ctx, &req, deps, store)
	return writeHeadlessResult(result, func(env map[string]any) (any, error) {
		sessionID, _ := env["session_id"].(string)
		data, _ := env["data"].(map[string]any)
		jdTextOut, _ := data["jd_text"].(string)
		return map[string]any{
			"session_id": sessionID,
			"jd_text":    jdTextOut,
		}, nil
	})
}

// writeHeadlessResult extracts the envelope from an MCP handler result, checks status, and
// writes the transformed success payload to stdout. On error, writes CLI error JSON to stderr.
func writeHeadlessResult(result *mcp.CallToolResult, transform func(map[string]any) (any, error)) error {
	if len(result.Content) == 0 {
		return writeError("handler_error", "handler returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return writeError("handler_error", "handler returned unexpected content type")
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &env); err != nil {
		return writeError("handler_error", fmt.Sprintf("parse handler response: %v", err))
	}

	if env["status"] != "ok" {
		// Extract error code and message from MCP envelope.
		code := "unknown_error"
		message := "unknown error"
		if errObj, ok := env["error"].(map[string]any); ok {
			if c, ok := errObj["code"].(string); ok && c != "" {
				code = c
			}
			if m, ok := errObj["message"].(string); ok && m != "" {
				message = m
			}
		}
		return writeError(code, message)
	}

	payload, err := transform(env)
	if err != nil {
		return writeError("transform_error", err.Error())
	}

	out, _ := json.Marshal(payload)
	fmt.Fprintf(os.Stdout, "%s\n", out)
	return nil
}
