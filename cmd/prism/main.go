// Command prism renders Prometheus metrics as a live terminal dashboard.
//
// main is deliberately the thinnest layer in the program: it resolves the one
// environment variable prism understands, translates operating system signals
// into a cancellable context, and owns every exit code. All wiring lives in
// [app.Build]; all teardown lives in [apphelpers.App.Cleanup].
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pantherhawk/prism/internal/app"
	"github.com/pantherhawk/prism/internal/config"
)

// exitFailure is the status returned for any unrecoverable startup, run, or
// teardown error. main is the only function in prism permitted to call
// [os.Exit], so it is the only place this constant is used.
const exitFailure = 1

// Build metadata, injected by goreleaser through -ldflags -X. These must be
// package-level variables for the linker to reach them.
//
//nolint:gochecknoglobals // the linker cannot write to anything else
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// overrides parses the command line into the settings that outrank the
// configuration file. It lives beside main because reading a flag is an
// entrypoint concern, exactly as reading a config file is.
//
// It only parses. Whether the flags make sense together is [app.Build]'s
// question, so that a contradiction is reported through the same path as any
// other bad configuration rather than through a second one.
func overrides() app.Overrides {
	dark := flag.Bool("dark", false, "force the dark palette, ignoring the terminal background")
	light := flag.Bool("light", false, "force the light palette, ignoring the terminal background")

	flag.Parse()

	return app.Overrides{Dark: *dark, Light: *light}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// A bootstrap logger, used only until the configured logger replaces it
	// inside Build. Without it, configuration errors would be silent.
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	application, err := app.Build(ctx, os.Getenv(config.PathEnv), app.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}, overrides())
	if err != nil {
		log.ErrorContext(ctx, "build failed", slog.Any("error", err))

		// Build returns a usable App even when it fails, so partially
		// constructed resources are still released.
		if cleanupErr := application.Cleanup(context.WithoutCancel(ctx)); cleanupErr != nil {
			log.ErrorContext(ctx, "cleanup failed", slog.Any("error", cleanupErr))
		}

		os.Exit(exitFailure)
	}

	// A cancelled context means the operator pressed ctrl-c. That is a
	// successful exit, not a failure.
	runErr := application.Run(ctx)
	if errors.Is(runErr, context.Canceled) {
		runErr = nil
	}

	if runErr != nil {
		log.ErrorContext(ctx, "run failed", slog.Any("error", runErr))
	}

	// Cleanup runs on a context detached from the signal handler: a second
	// ctrl-c must not abort the teardown the first one triggered.
	if cleanupErr := application.Cleanup(context.WithoutCancel(ctx)); cleanupErr != nil {
		log.ErrorContext(ctx, "cleanup failed", slog.Any("error", cleanupErr))
		os.Exit(exitFailure)
	}

	if runErr != nil {
		os.Exit(exitFailure)
	}
}
