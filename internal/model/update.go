package model

import (
	"strconv"
	"strings"
	"time"
)

// UpdateCache is the persisted state of the last update check.
type UpdateCache struct {
	LatestVersion  string    `json:"latest_version"`
	CurrentVersion string    `json:"current_version"`
	CheckedAt      time.Time `json:"checked_at"`
}

// IsNewer reports whether latest is a higher semver than current.
// Returns false if either version cannot be parsed or current is "dev".
func IsNewer(current, latest string) bool {
	if current == "dev" {
		return false
	}
	curParts, ok := parseSemver(current)
	if !ok {
		return false
	}
	latParts, ok := parseSemver(latest)
	if !ok {
		return false
	}
	for i := 0; i < 3; i++ {
		if latParts[i] > curParts[i] {
			return true
		}
		if latParts[i] < curParts[i] {
			return false
		}
	}
	return false
}

// parseSemver parses a version string like "v1.2.3" or "1.2.3" into [major, minor, patch].
// Pre-release suffixes (e.g. "-rc1") are stripped before parsing.
func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	// Strip pre-release suffix.
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		result[i] = n
	}
	return result, true
}
