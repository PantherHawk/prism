// Package apphelpers manages the startup and cleanup phases of an application
// lifecycle. It exists so that main.go carries nothing but signal handling and
// exit codes, which keeps the interesting parts of the program testable.
package apphelpers

import (
	"context"
	"errors"
)

type (
	// App holds the functions to run at the start and end of the application
	// lifecycle. The zero value is not usable; call [New].
	App struct {
		startupFuncs []StartupFunc
		cleanupFuncs []CleanupFunc
	}

	// StartupFunc is a long-lived unit of work executed as a goroutine. It
	// must write exactly one value, nil or otherwise, to the error channel
	// before returning, so that [App.Run] can account for it.
	StartupFunc func(context.Context, chan<- error)

	// CleanupFunc releases a resource acquired during startup, or performs a
	// short generic task at shutdown. Cleanup functions are expected to be
	// short lived and must respect the context deadline.
	CleanupFunc func(context.Context) error
)

// New returns an App with no registered functions.
func New() *App {
	return &App{
		startupFuncs: make([]StartupFunc, 0),
		cleanupFuncs: make([]CleanupFunc, 0),
	}
}

// AddStartupFuncs registers functions to be run concurrently by [App.Run].
func (app *App) AddStartupFuncs(fns ...StartupFunc) {
	app.startupFuncs = append(app.startupFuncs, fns...)
}

// AddCleanupFuncs registers functions to be run by [App.Cleanup]. They run in
// reverse registration order, so register each one alongside the resource it
// releases.
func (app *App) AddCleanupFuncs(fns ...CleanupFunc) {
	app.cleanupFuncs = append(app.cleanupFuncs, fns...)
}

// Run starts each registered startup function in its own goroutine and blocks
// until they have all reported completion, one of them reports a non-nil error,
// or ctx is done. An error from any startup function is treated as terminal.
func (app *App) Run(ctx context.Context) error {
	// The channel is buffered to the number of startup functions. Run returns
	// as soon as it sees the first error or a cancelled context, and an
	// unbuffered channel would then block every remaining goroutine forever on
	// its send, leaking it along with any deferred release it still owes.
	errs := make(chan error, len(app.startupFuncs))

	for _, fn := range app.startupFuncs {
		go fn(ctx, errs)
	}

	for range app.startupFuncs {
		select {
		case err := <-errs:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// Cleanup runs each registered cleanup function sequentially in reverse
// registration order, so resources are released in the opposite order to their
// acquisition. Every function runs even if an earlier one fails; the resulting
// errors are joined and returned together.
func (app *App) Cleanup(ctx context.Context) error {
	errs := make([]error, 0, len(app.cleanupFuncs))

	for i := len(app.cleanupFuncs) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)

			break
		}

		errs = append(errs, app.cleanupFuncs[i](ctx))
	}

	return errors.Join(errs...)
}
