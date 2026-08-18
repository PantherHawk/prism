// Package app assembles prism's domains into a runnable application. It is the
// only package that knows about every other one, which keeps the dependency
// graph a tree rather than a web.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pantherhawk/prism/internal/apphelpers"
	"github.com/pantherhawk/prism/internal/banner"
	"github.com/pantherhawk/prism/internal/config"
	"github.com/pantherhawk/prism/internal/hostinfo"
	"github.com/pantherhawk/prism/internal/logging"
	"github.com/pantherhawk/prism/internal/scrape"
	"github.com/pantherhawk/prism/internal/series"
	"github.com/pantherhawk/prism/internal/stream"
	"github.com/pantherhawk/prism/internal/telemetry"
	"github.com/pantherhawk/prism/internal/tui"
)

// BuildInfo is the version metadata the linker injects into main. It is
// passed down rather than read from a global so that the splash screen is
// testable.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Build reads configuration, constructs every long-lived component, and
// registers their startup and cleanup functions on an App.
//
// It always returns a non-nil App, even when it returns an error, so that the
// caller can release whatever was constructed before the failure. That
// contract is what lets main.go treat startup failure and shutdown as the same
// code path.
func Build(
	ctx context.Context,
	configPath string,
	info BuildInfo,
	overrides Overrides,
) (*apphelpers.App, error) {
	application := apphelpers.New()

	cfg, err := config.Read(configPath)
	if err != nil {
		return application, fmt.Errorf("read config: %w", err)
	}

	mode, err := overrides.Theme()
	if err != nil {
		return application, fmt.Errorf("resolve flags: %w", err)
	}

	cfg.Theme.Mode = mode.Or(cfg.Theme.Mode)

	log, err := logging.New(cfg.Logging)
	if err != nil {
		return application, fmt.Errorf("build logger: %w", err)
	}

	shutdownTelemetry, err := telemetry.Setup(ctx, cfg.Telemetry)
	if err != nil {
		return application, fmt.Errorf("build telemetry: %w", err)
	}

	application.AddCleanupFuncs(shutdownTelemetry)

	store, err := series.NewStore(
		cfg.Scrape.Retention,
		cfg.Scrape.Resolution,
		cfg.Scrape.FamilyBudget,
		log,
	)
	if err != nil {
		return application, fmt.Errorf("build series store: %w", err)
	}

	// Registered immediately so that a later failure still releases it.
	application.AddCleanupFuncs(store.Close)

	broker, err := stream.New(cfg.Stream, log)
	if err != nil {
		return application, fmt.Errorf("build stream broker: %w", err)
	}

	application.AddCleanupFuncs(broker.Shutdown)

	// The local store is written first so that a slow network consumer can
	// never delay the data on screen.
	sink := series.Fanout(store, broker)

	source, err := buildSource(cfg, sink, log)
	if err != nil {
		return application, err
	}

	ui, err := tui.New(cfg.Theme, splashInfo(cfg, info), cfg.Scrape.CardinalityWarn, store, log)
	if err != nil {
		return application, fmt.Errorf("build tui: %w", err)
	}

	// Order matters only for readability here: Run starts them concurrently.
	application.AddStartupFuncs(source, broker.Run, ui.Run)

	return application, nil
}

// splashInfo assembles what the splash screen draws: prism's own runtime, from
// the config and the linker's build metadata, and the host's, from hostinfo.
//
// The host facts are read here, once, at startup. That keeps banner.Render a
// pure function of what it is handed rather than of the machine it runs on.
func splashInfo(cfg config.Config, info BuildInfo) banner.Info {
	facts := hostinfo.Collect()

	return banner.Info{
		Version:  info.Version,
		Endpoint: sourceName(cfg),
		Buffer:   fmt.Sprintf("%s ring · %s buckets", cfg.Scrape.Retention, cfg.Scrape.Resolution),
		User:     facts.User,
		Host:     facts.Host,
		OS:       facts.OS,
		Kernel:   facts.Kernel,
		Shell:    facts.Shell,
		Term:     facts.Term,
		Go:       facts.Go,
		Uptime:   facts.Uptime,
	}
}

// sourceName describes where samples will come from, for the splash screen.
func sourceName(cfg config.Config) string {
	if cfg.Stream.Upstream != "" {
		return cfg.Stream.Upstream + " (upstream)"
	}

	return cfg.Scrape.Endpoint
}

// buildSource returns the startup function that fills the store.
//
// prism either scrapes a target or follows another prism, never both. Doing
// both would merge two clocks into one ring and quietly interleave samples that
// were never observed together.
func buildSource(
	cfg config.Config,
	sink series.Sink,
	log *slog.Logger,
) (apphelpers.StartupFunc, error) {
	if cfg.Stream.Upstream != "" {
		client, err := stream.NewClient(cfg.Stream, sink, log)
		if err != nil {
			return nil, fmt.Errorf("build stream client: %w", err)
		}

		return client.Run, nil
	}

	collector, err := scrape.New(cfg.Scrape, sink, log)
	if err != nil {
		return nil, fmt.Errorf("build collector: %w", err)
	}

	return collector.Run, nil
}
