package tailor

import (
	"log/slog"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Tailor = (*Service)(nil)

// Service implements port.Tailor for keyword injection and edit application.
type Service struct {
	defaults *config.AppDefaults
	log      *slog.Logger
}

// New constructs a Service with the provided dependencies.
func New(defaults *config.AppDefaults, log *slog.Logger) *Service {
	return &Service{
		defaults: defaults,
		log:      log,
	}
}
