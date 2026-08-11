//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/filter"
	"github.com/pantherhawk/prism/internal/series"
)

// filterWorld is the state of one filtering scenario.
type filterWorld struct {
	snapshot *series.Snapshot
	compiled *filter.Filter
	source   string
	parseErr error
	pivotKey string
	limit    int
}

type filterKey struct{}

func filterFrom(ctx context.Context) *filterWorld {
	w, ok := ctx.Value(filterKey{}).(*filterWorld)
	if !ok {
		panic("filter world missing from context")
	}

	return w
}

// selection builds the selection the scenario has described so far.
func (w *filterWorld) selection() series.Selection {
	sel := series.Selection{Family: "rq", PivotKey: w.pivotKey, Limit: w.limit}
	if w.compiled != nil {
		sel.Match = w.compiled.Match
	}

	return sel
}

// ---- Given -----------------------------------------------------------------

func (w *filterWorld) aFamilyWithSeries(ctx context.Context, table *godog.Table) (context.Context, error) {
	store, err := series.NewStore(time.Minute, 15*time.Second, 0,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return ctx, fmt.Errorf("new store: %w", err)
	}

	samples := make([]series.Sample, 0, len(table.Rows)-1)

	for _, row := range table.Rows[1:] {
		value, err := strconv.ParseFloat(strings.TrimSpace(row.Cells[2].Value), 64)
		if err != nil {
			return ctx, fmt.Errorf("parse value: %w", err)
		}

		labels := series.Labels{
			{Name: "cluster", Value: strings.TrimSpace(row.Cells[0].Value)},
			{Name: "status", Value: strings.TrimSpace(row.Cells[1].Value)},
		}
		labels.Sort()

		samples = append(samples, series.Sample{
			Family: "rq",
			Labels: labels,
			Kind:   series.KindGauge,
			Value:  value,
		})
	}

	store.Append(time.Unix(1_700_000_000, 0), samples, series.Stats{})

	w.snapshot = store.Snapshot()
	w.limit = 10

	return ctx, nil
}

// ---- When ------------------------------------------------------------------

func (w *filterWorld) theFilterIs(ctx context.Context, expr string) (context.Context, error) {
	w.source = expr
	w.compiled, w.parseErr = filter.Parse(expr)

	return ctx, nil
}

func (w *filterWorld) pivotingOn(ctx context.Context, key string) (context.Context, error) {
	w.pivotKey = key

	return ctx, nil
}

func (w *filterWorld) pivotingOnWithLimit(ctx context.Context, key string, limit int) (context.Context, error) {
	w.pivotKey, w.limit = key, limit

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *filterWorld) seriesMatch(ctx context.Context, want int) (context.Context, error) {
	if w.parseErr != nil {
		return ctx, fmt.Errorf("filter did not compile: %w", w.parseErr)
	}

	matching := w.snapshot.Matching(series.Selection{Family: "rq", Match: w.compiled.Match})
	if got := len(matching); got != want {
		return ctx, fmt.Errorf("%d series matched, want %d", got, want)
	}

	return ctx, nil
}

func (w *filterWorld) theFilterIsStillReported(ctx context.Context, want string) (context.Context, error) {
	if got := w.compiled.String(); got != want {
		return ctx, fmt.Errorf("filter reported as %q, want %q", got, want)
	}

	return ctx, nil
}

func (w *filterWorld) theFilterIsRejected(ctx context.Context) (context.Context, error) {
	if w.parseErr == nil {
		return ctx, errFilterAccepted
	}

	return ctx, nil
}

func (w *filterWorld) theReasonMentions(ctx context.Context, want string) (context.Context, error) {
	if w.parseErr == nil {
		return ctx, errFilterAccepted
	}

	if !strings.Contains(w.parseErr.Error(), want) {
		return ctx, fmt.Errorf("reason %q does not mention %q", w.parseErr, want)
	}

	return ctx, nil
}

func (w *filterWorld) thereAreLines(ctx context.Context, want int) (context.Context, error) {
	if got := len(w.snapshot.Lines(w.selection())); got != want {
		return ctx, fmt.Errorf("%d lines, want %d", got, want)
	}

	return ctx, nil
}

func (w *filterWorld) theLineTotals(
	ctx context.Context, label string, total float64, members int,
) (context.Context, error) {
	for _, line := range w.snapshot.Lines(w.selection()) {
		if line.Label != label {
			continue
		}

		if line.Last != total || line.Members != members {
			return ctx, fmt.Errorf(
				"line %q totalled %v across %d series, want %v across %d",
				label, line.Last, line.Members, total, members)
		}

		return ctx, nil
	}

	return ctx, fmt.Errorf("no line labelled %q", label)
}

func (w *filterWorld) theLastLineIs(
	ctx context.Context, label string, total float64, members int,
) (context.Context, error) {
	lines := w.snapshot.Lines(w.selection())
	if len(lines) == 0 {
		return ctx, errNoLines
	}

	last := lines[len(lines)-1]
	if last.Label != label || last.Last != total || last.Members != members {
		return ctx, fmt.Errorf(
			"last line = %q/%v/%d, want %q/%v/%d",
			last.Label, last.Last, last.Members, label, total, members)
	}

	return ctx, nil
}

// ---- wiring ----------------------------------------------------------------

var (
	errFilterAccepted = errors.New("filter compiled when it should have been rejected")
	errNoLines        = errors.New("no lines were produced")
)

// initializeFilter registers the filtering steps.
func initializeFilter(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, filterKey{}, &filterWorld{limit: 10}), nil
	})

	sc.Step(`^a family "rq" with series$`, func(ctx context.Context, table *godog.Table) (context.Context, error) {
		return filterFrom(ctx).aFamilyWithSeries(ctx, table)
	})

	sc.Step(`^the filter is "([^"]*)"$`, func(ctx context.Context, expr string) (context.Context, error) {
		return filterFrom(ctx).theFilterIs(ctx, expr)
	})
	sc.Step(`^pivoting on "([^"]*)"$`, func(ctx context.Context, key string) (context.Context, error) {
		return filterFrom(ctx).pivotingOn(ctx, key)
	})
	sc.Step(`^pivoting on "([^"]*)" with a limit of (\d+)$`,
		func(ctx context.Context, key string, limit int) (context.Context, error) {
			return filterFrom(ctx).pivotingOnWithLimit(ctx, key, limit)
		})

	sc.Step(`^(\d+) series match$`, func(ctx context.Context, want int) (context.Context, error) {
		return filterFrom(ctx).seriesMatch(ctx, want)
	})
	sc.Step(`^the filter is still reported as "([^"]*)"$`,
		func(ctx context.Context, want string) (context.Context, error) {
			return filterFrom(ctx).theFilterIsStillReported(ctx, want)
		})
	sc.Step(`^the filter is rejected$`, func(ctx context.Context) (context.Context, error) {
		return filterFrom(ctx).theFilterIsRejected(ctx)
	})
	sc.Step(`^the reason mentions "([^"]*)"$`, func(ctx context.Context, want string) (context.Context, error) {
		return filterFrom(ctx).theReasonMentions(ctx, want)
	})
	sc.Step(`^there are (\d+) lines$`, func(ctx context.Context, want int) (context.Context, error) {
		return filterFrom(ctx).thereAreLines(ctx, want)
	})
	sc.Step(`^the line "([^"]*)" totals ([0-9.]+) across (\d+) series$`,
		func(ctx context.Context, label string, total float64, members int) (context.Context, error) {
			return filterFrom(ctx).theLineTotals(ctx, label, total, members)
		})
	sc.Step(`^the last line is "([^"]*)" totalling ([0-9.]+) across (\d+) series$`,
		func(ctx context.Context, label string, total float64, members int) (context.Context, error) {
			return filterFrom(ctx).theLastLineIs(ctx, label, total, members)
		})
}
