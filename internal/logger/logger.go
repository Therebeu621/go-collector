// Package logger provides a structured logging factory using zerolog.
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New creates a zerolog.Logger configured according to the given level and format.
//   - format "pretty": human-readable console output (for local dev).
//   - format "json" (default): structured JSON output (for production).
func New(level, format string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(lvl)

	if format == "pretty" {
		return zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Logger()
	}

	return zerolog.New(os.Stderr).With().Timestamp().Logger()
}
