package logger

import (
	"context"
	"log/slog"
)

const bannerLine = "***************************"

// Banner logs a three-line section header at Info level to mark the start of a
// named pipeline stage. A non-empty title is appended after a colon.
//
//	***************************
//	Score: Original
//	***************************
func Banner(ctx context.Context, log *slog.Logger, stage, title string) {
	label := stage
	if title != "" {
		label = stage + ": " + title
	}
	log.InfoContext(ctx, bannerLine)
	log.InfoContext(ctx, label)
	log.InfoContext(ctx, bannerLine)
}
