package port

import (
	"context"
	"io"
	"time"
)

// ReleaseInfo describes a published release.
type ReleaseInfo struct {
	Version     string
	ReleaseURL  string
	PublishedAt time.Time
}

// ReleaseChecker looks up the latest release and downloads release assets.
type ReleaseChecker interface {
	LatestVersion(ctx context.Context) (ReleaseInfo, error)
	DownloadRelease(ctx context.Context, version, goos, goarch string) (io.ReadCloser, error)
}
