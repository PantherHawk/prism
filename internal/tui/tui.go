// Package tui renders prism's terminal interface.
//
// The model is a pure function of a snapshot: Update folds messages into
// state, View turns state into cells, and neither performs I/O. That is what a
// depguard rule in .golangci.yml enforces, and what keeps a frame inside its
// budget.
package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/pantherhawk/prism/internal/banner"
	"github.com/pantherhawk/prism/internal/series"
	"github.com/pantherhawk/prism/internal/theme"
)

// TUI owns the Bubble Tea program and the state it renders from.
type TUI struct {
	cfg    theme.Config
	info   banner.Info
	warnAt int
	store  *series.Store
	log    *slog.Logger
}

// New returns a TUI rendering from store. warnAt is the family cardinality above
// which the UI flags a family.
func New(
	cfg theme.Config,
	info banner.Info,
	warnAt int,
	store *series.Store,
	log *slog.Logger,
) (*TUI, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate theme: %w", err)
	}

	return &TUI{cfg: cfg, info: info, warnAt: warnAt, store: store, log: log}, nil
}

// Run drives the program until the operator quits or ctx is done.
//
// A cancelled context and an interrupt both mean the operator asked to leave,
// so neither is reported as an error: they are the normal way this program
// ends.
func (t *TUI) Run(ctx context.Context, errs chan<- error) {
	program := tea.NewProgram(newModel(t.cfg, t.info, t.warnAt, t.store), tea.WithContext(ctx))

	if _, err := program.Run(); err != nil {
		switch {
		case errors.Is(err, tea.ErrProgramKilled),
			errors.Is(err, tea.ErrInterrupted),
			errors.Is(err, context.Canceled):
			t.log.DebugContext(ctx, "tui interrupted")
		default:
			errs <- fmt.Errorf("run tui: %w", err)

			return
		}
	}

	errs <- nil
}
