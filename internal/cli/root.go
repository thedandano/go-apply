package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/updater"
)

const updateCheckTTL = 24 * time.Hour

// skipUpdateCheckCommands are commands where a startup update check would be
// disruptive (serve outputs structured data; update and version handle versioning directly).
var skipUpdateCheckCommands = map[string]bool{
	"serve":   true,
	"update":  true,
	"version": true,
}

// NewRootCommand returns the root cobra command for go-apply.
func NewRootCommand(version string) *cobra.Command {
	notifier := newUpdateCheckerContext()

	cmd := &cobra.Command{
		Use:   "go-apply",
		Short: "AI-powered job application assistant",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			scheduleUpdateCheck(cmd.Context(), version, notifier, cmd.Name())
			return nil
		},
		PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
			printUpdateNotification(notifier)
			return nil
		},
	}
	cmd.AddCommand(NewApplyCommand())
	cmd.AddCommand(NewServeCommand())
	cmd.AddCommand(NewConfigCommand())
	cmd.AddCommand(NewOnboardCommand())
	cmd.AddCommand(NewVersionCommand(version))
	cmd.AddCommand(NewSetupCommand())
	cmd.AddCommand(NewUpdateCommand(version))
	return cmd
}

// scheduleUpdateCheck launches a background goroutine to check for updates.
// It reads the cache first; if fresh, it signals immediately with any pending notification.
// Results are communicated through the notifier.
func scheduleUpdateCheck(ctx context.Context, version string, notifier *updateCheckerContext, cmdName string) {
	if version == "dev" || skipUpdateCheckCommands[cmdName] {
		// Signal done immediately so PostRun doesn't wait.
		notifier.done <- struct{}{}
		return
	}

	go func() {
		cachePath := updater.CachePath()
		cache, err := updater.ReadCache(cachePath)
		if err == nil && updater.IsCacheFresh(cache, updateCheckTTL) {
			// Use cached result.
			if cache.LatestVersion != "" && model.IsNewer(version, cache.LatestVersion) {
				notifier.setMessage(buildNotificationMessage(version, cache.LatestVersion))
			} else {
				notifier.done <- struct{}{}
			}
			return
		}

		// Cache is stale or absent — query GitHub.
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		svc := updater.New(releaseOwner, releaseRepo, http.DefaultClient)
		info, err := svc.LatestVersion(checkCtx)
		if err != nil {
			// Network failure is non-fatal — silently skip notification.
			notifier.done <- struct{}{}
			return
		}

		// Persist the result regardless of whether an update is available.
		newCache := &model.UpdateCache{
			LatestVersion:  info.Version,
			CurrentVersion: version,
			CheckedAt:      time.Now().UTC(),
		}
		_ = updater.WriteCache(cachePath, newCache) // best-effort

		if model.IsNewer(version, info.Version) {
			notifier.setMessage(buildNotificationMessage(version, info.Version))
		} else {
			notifier.done <- struct{}{}
		}
	}()
}

// printUpdateNotification waits briefly for the background check and prints if an update is available.
func printUpdateNotification(notifier *updateCheckerContext) {
	select {
	case <-notifier.done:
	case <-time.After(2 * time.Second):
	}
	if notifier.message != "" {
		fmt.Fprintln(os.Stderr, notifier.message)
	}
}

func buildNotificationMessage(current, latest string) string {
	return fmt.Sprintf(
		"\nA new version of go-apply is available: %s (current: %s)\nRun \"go-apply update\" to update.",
		latest, current,
	)
}
