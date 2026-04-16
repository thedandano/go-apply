package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/updater"
)

const (
	releaseOwner = "thedandano"
	releaseRepo  = "go-apply"
)

// NewUpdateCommand returns the "update" cobra command.
func NewUpdateCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update go-apply to the latest release",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunUpdateWithChecker(
				cmd.Context(),
				version,
				updater.New(releaseOwner, releaseRepo, http.DefaultClient),
				cmd.OutOrStdout(),
			)
		},
	}
}

// RunUpdateWithChecker performs the update logic using the given checker.
// Exported to allow testing with a mock ReleaseChecker.
func RunUpdateWithChecker(ctx context.Context, currentVersion string, checker port.ReleaseChecker, out io.Writer) error {
	if currentVersion == "dev" {
		return fmt.Errorf("cannot self-update a development build")
	}

	fmt.Fprintln(out, "Checking for the latest release...")
	info, err := checker.LatestVersion(ctx)
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}

	if !model.IsNewer(currentVersion, info.Version) {
		fmt.Fprintf(out, "go-apply is already up to date (%s).\n", currentVersion)
		return nil
	}

	fmt.Fprintf(out, "Updating go-apply from %s to %s...\n", currentVersion, info.Version)

	rc, err := checker.DownloadRelease(ctx, info.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("download release: %w", err)
	}
	defer rc.Close()

	binaryBytes, err := extractBinary(rc, runtime.GOOS)
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	if err := replaceBinary(binaryBytes); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Fprintf(out, "Successfully updated to %s.\n", info.Version)
	return nil
}

// extractBinary reads the release archive stream and returns the raw bytes of the go-apply binary.
func extractBinary(r io.Reader, goos string) ([]byte, error) {
	if goos == "windows" {
		return extractFromZip(r)
	}
	return extractFromTarGz(r)
}

func extractFromTarGz(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Name == releaseRepo || hdr.Name == "./"+releaseRepo {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", releaseRepo)
}

func extractFromZip(r io.Reader) ([]byte, error) {
	// zip.NewReader requires io.ReaderAt + size. Buffer the stream into a temp file.
	tmp, err := os.CreateTemp("", "go-apply-download-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file for zip: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	size, err := io.Copy(tmp, r)
	if err != nil {
		return nil, fmt.Errorf("buffer zip: %w", err)
	}

	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	target := releaseRepo + ".exe"
	for _, f := range zr.File {
		if f.Name == target {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open zip entry %s: %w", f.Name, err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("binary %q not found in zip", target)
}

// replaceBinary atomically replaces the running executable with newBytes.
func replaceBinary(newBytes []byte) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}

	info, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("stat executable: %w", err)
	}

	// Write new binary to a temp file in the same directory (same filesystem → atomic rename).
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".go-apply-update-*")
	if err != nil {
		return fmt.Errorf("create temp binary: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(newBytes); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp binary: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpName, info.Mode()); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp binary: %w", err)
	}

	if err := os.Rename(tmpName, execPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// updateCheckerContext is a shared context type used to pass update notification
// between PersistentPreRun and PersistentPostRun hooks in root.go.
type updateCheckerContext struct {
	done    chan struct{}
	message string
}

func newUpdateCheckerContext() *updateCheckerContext {
	return &updateCheckerContext{done: make(chan struct{}, 1)}
}

func (u *updateCheckerContext) setMessage(msg string) {
	u.message = msg
	select {
	case u.done <- struct{}{}:
	default:
	}
}
