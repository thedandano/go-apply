package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/onboardcheck"
)

// CheckOnboarded verifies that the user is ready to use pipeline tools.
// It delegates to the shared onboardcheck package.
//
// Exported so that tests can inject a stub ResumeRepository.
func CheckOnboarded(resumes port.ResumeRepository) error {
	return onboardcheck.CheckOnboarded(resumes)
}

// RequireOnboarded wraps an MCP tool handler to enforce onboarding before dispatch.
// resumeRepo is injected so the function is testable without filesystem I/O.
// In production use requireOnboarded (unexported) which wires the live dependencies.
func RequireOnboarded(
	resumeRepo port.ResumeRepository,
	inner func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error),
) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := CheckOnboarded(resumeRepo); err != nil {
			return errorResult(err.Error()), nil
		}
		return inner(ctx, req)
	}
}

// requireOnboarded is the production wrapper.
// It wires the live ResumeRepository on each invocation so that
// changes (new resumes added) take effect immediately.
func requireOnboarded(inner func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resumeRepo := fs.NewResumeRepository(config.DataDir())
		return RequireOnboarded(resumeRepo, inner)(ctx, req)
	}
}
