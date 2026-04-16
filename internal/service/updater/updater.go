package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/thedandano/go-apply/internal/port"
)

const defaultAPIBase = "https://api.github.com"

// Service implements port.ReleaseChecker against the GitHub Releases API.
type Service struct {
	owner   string
	repo    string
	client  *http.Client
	apiBase string
}

// New returns a new Service for the given GitHub owner/repo.
func New(owner, repo string, client *http.Client) *Service {
	return &Service{
		owner:   owner,
		repo:    repo,
		client:  client,
		apiBase: defaultAPIBase,
	}
}

// Compile-time interface check.
var _ port.ReleaseChecker = (*Service)(nil)

// SetAPIBase overrides the GitHub API base URL. Intended for testing only.
func (s *Service) SetAPIBase(base string) {
	s.apiBase = base
}

// LatestVersion queries the GitHub Releases API and returns info about the latest release.
func (s *Service) LatestVersion(ctx context.Context) (port.ReleaseInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", s.apiBase, s.owner, s.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return port.ReleaseInfo{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.client.Do(req)
	if err != nil {
		return port.ReleaseInfo{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return port.ReleaseInfo{}, fmt.Errorf("unexpected status from GitHub API: %d", resp.StatusCode)
	}

	var payload struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return port.ReleaseInfo{}, fmt.Errorf("decode release response: %w", err)
	}

	info := port.ReleaseInfo{
		Version:    payload.TagName,
		ReleaseURL: payload.HTMLURL,
	}
	return info, nil
}

// DownloadRelease streams the release archive for the given version, OS, and architecture.
// Asset naming follows the GoReleaser convention: {repo}_{version}_{os}_{arch}.{ext}
func (s *Service) DownloadRelease(ctx context.Context, version, goos, goarch string) (io.ReadCloser, error) {
	assetName := assetFilename(s.repo, version, goos, goarch)
	// GoReleaser uploads assets to: https://github.com/{owner}/{repo}/releases/download/{version}/{asset}
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", s.owner, s.repo, version, assetName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download release asset %s: %w", assetName, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close() // #nosec G104 -- best-effort close on error path
		return nil, fmt.Errorf("unexpected status downloading %s: %d", assetName, resp.StatusCode)
	}

	return resp.Body, nil
}

// assetFilename returns the GoReleaser archive filename for the given parameters.
func assetFilename(project, version, goos, goarch string) string {
	// GoReleaser strips the leading "v" from the version in asset names.
	ver := version
	if len(ver) > 0 && ver[0] == 'v' {
		ver = ver[1:]
	}
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", project, ver, goos, goarch, ext)
}
