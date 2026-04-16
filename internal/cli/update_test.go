package cli_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/port"
)

// mockReleaseChecker implements port.ReleaseChecker for tests.
type mockReleaseChecker struct {
	latestVersion port.ReleaseInfo
	latestErr     error
	downloadErr   error
}

func (m *mockReleaseChecker) LatestVersion(_ context.Context) (port.ReleaseInfo, error) {
	return m.latestVersion, m.latestErr
}

func (m *mockReleaseChecker) DownloadRelease(_ context.Context, _, _, _ string) (io.ReadCloser, error) {
	return nil, m.downloadErr
}

func TestUpdateCommand_DevBuild(t *testing.T) {
	cmd := cli.NewUpdateCommand("dev")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for dev build, got nil")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestUpdateCommand_AlreadyUpToDate(t *testing.T) {
	mock := &mockReleaseChecker{
		latestVersion: port.ReleaseInfo{Version: "v0.1.0", ReleaseURL: "https://example.com"},
	}

	var buf bytes.Buffer
	err := cli.RunUpdateWithChecker(context.Background(), "v0.1.0", mock, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("already up to date")) {
		t.Errorf("expected 'already up to date' message, got: %s", buf.String())
	}
}

func TestUpdateCommand_LatestVersionError(t *testing.T) {
	mock := &mockReleaseChecker{
		latestErr: io.ErrUnexpectedEOF,
	}
	err := cli.RunUpdateWithChecker(context.Background(), "v0.1.0", mock, io.Discard)
	if err == nil {
		t.Fatal("expected error when LatestVersion fails")
	}
}
