package logger

import (
	"context"
	"crypto/rand"
	"fmt"
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

// ShortID returns an 8-character hex string for identifying a run or session.
func ShortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "????????"
	}
	return fmt.Sprintf("%08x", b)
}
