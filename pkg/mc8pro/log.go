package mc8pro

import (
	"io"
	"log/slog"
)

// discardLogger returns a *slog.Logger that drops all output. Used as
// the no-op default when callers don't supply a Logger in OpenOptions.
// Returning a non-nil logger means downstream code never has to
// nil-check before calling .Debug / .Info / etc.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
