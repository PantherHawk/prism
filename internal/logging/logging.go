// Package logging builds the application logger. It is separate from the
// packages that log so that the shape of a log line is decided in one place.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ErrUnknownFormat is returned when the configured format is not recognised.
var ErrUnknownFormat = errors.New("unknown log format")

// Format selects the encoding of a log record.
type Format string

const (
	// FormatText is the human-readable encoding used on a terminal.
	FormatText Format = "text"
	// FormatJSON is the machine-readable encoding used when shipping logs.
	FormatJSON Format = "json"
)

// Config describes the logger.
//
// Logs go to stderr, but only when stderr is somewhere other than the terminal
// the dashboard is drawing on. Sending them to stderr was once described here
// as keeping them clear of the UI, which was never true: stdout and stderr are
// the same terminal unless somebody redirects one of them, and a log line
// written into a full-screen render corrupts it exactly as much from either.
//
// The P5 walkthrough is what settled it. The frame meant to show an upstream
// going away had the warning about the upstream going away printed across it,
// which is the failure prism exists to help diagnose being obscured by prism's
// own account of it. See [Destination].
type Config struct {
	Level  string `yaml:"level"`
	Format Format `yaml:"format"`
	Source bool   `yaml:"source"`
}

// Default returns the configuration used when none is supplied.
func Default() Config {
	return Config{
		Level:  "info",
		Format: FormatText,
		Source: false,
	}
}

// Destination returns where log records should be written given where stderr
// currently points.
//
// A character device is a terminal, and the terminal is where the dashboard
// is. There is nowhere on it for a log line to go that is not on top of a
// frame, so records are dropped rather than drawn through the UI. Redirect
// stderr and they are kept:
//
//	prism 2>prism.log
//
// Nothing is lost by the default. Everything an operator needs while watching
// the screen is on the screen - a scrape that is failing shows as `● stalled`,
// a dropped upstream as `⟳ reconnecting`, and the error count sits in the
// status bar - because a dashboard that needed its log to be legible would be
// a dashboard with a gap in it.
func Destination(stderr *os.File) io.Writer {
	info, err := stderr.Stat()
	if err != nil {
		// Nothing is known about the destination, so assume it is the terminal
		// and protect the frame. A missing log is recoverable; a corrupted
		// dashboard misreports the thing being diagnosed.
		return io.Discard
	}

	if info.Mode()&os.ModeCharDevice != 0 {
		return io.Discard
	}

	return stderr
}

// New builds a logger from cfg, writing to [Destination].
func New(cfg Config) (*slog.Logger, error) {
	return NewTo(cfg, Destination(os.Stderr))
}

// NewTo builds a logger from cfg that writes to w. It is what makes the format
// and level handling testable without a terminal to point at.
func NewTo(cfg Config, dest io.Writer) (*slog.Logger, error) {
	var level slog.Level

	err := level.UnmarshalText([]byte(strings.ToLower(cfg.Level)))
	if err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", cfg.Level, err)
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.Source,
	}

	switch cfg.Format {
	case FormatText:
		return slog.New(slog.NewTextHandler(dest, opts)), nil
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(dest, opts)), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownFormat, cfg.Format)
	}
}
