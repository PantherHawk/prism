//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/app"
	"github.com/pantherhawk/prism/internal/apphelpers"
)

// settleTimeout bounds how long a leak check waits for goroutines to unwind.
const settleTimeout = 500 * time.Millisecond

// errStartupFailed is the sentinel a deliberately failing startup function
// returns, so that assertions can identify it rather than matching on text.
var errStartupFailed = errors.New("startup failed")

// world is the state of a single scenario. godog runs scenarios
// concurrently, so it is carried on the context rather than in a package
// variable.
type world struct {
	application *apphelpers.App
	buildErr    error
	runErr      error
	cleanupErr  error

	configPath string
	order      []string
	failing    map[string]bool

	goroutinesAtStart int
	cancel            context.CancelFunc
}

type worldKey struct{}

func from(ctx context.Context) *world {
	w, ok := ctx.Value(worldKey{}).(*world)
	if !ok {
		panic("scenario world missing from context")
	}

	return w
}

func (w *world) record(name string) {
	w.order = append(w.order, name)
}

// ---- Given -----------------------------------------------------------------

func (w *world) noConfigurationPath(ctx context.Context) (context.Context, error) {
	w.configPath = ""

	return ctx, nil
}

func (w *world) configurationPathIs(ctx context.Context, path string) (context.Context, error) {
	w.configPath = path

	return ctx, nil
}

func (w *world) aStartupFunctionThatFails(ctx context.Context, delay string) (context.Context, error) {
	after, err := time.ParseDuration(delay)
	if err != nil {
		return ctx, fmt.Errorf("parse delay: %w", err)
	}

	w.application.AddStartupFuncs(func(_ context.Context, errs chan<- error) {
		time.Sleep(after)
		errs <- errStartupFailed
	})

	return ctx, nil
}

func (w *world) aStartupFunctionThatBlocks(ctx context.Context) (context.Context, error) {
	w.application.AddStartupFuncs(func(runCtx context.Context, errs chan<- error) {
		<-runCtx.Done()
		errs <- nil
	})

	return ctx, nil
}

func (w *world) cleanupFunctions(ctx context.Context, a, b, c string) (context.Context, error) {
	for _, name := range []string{a, b, c} {
		w.application.AddCleanupFuncs(func(context.Context) error {
			w.record(name)

			if w.failing[name] {
				return fmt.Errorf("%s failed", name) //nolint:err113 // scenario fixture
			}

			return nil
		})
	}

	return ctx, nil
}

func (w *world) cleanupFunctionFails(ctx context.Context, name string) (context.Context, error) {
	w.failing[name] = true

	return ctx, nil
}

// ---- When ------------------------------------------------------------------

func (w *world) theApplicationIsBuilt(ctx context.Context) (context.Context, error) {
	w.application, w.buildErr = app.Build(ctx, w.configPath, app.BuildInfo{Version: "test"}, app.Overrides{})

	return ctx, nil
}

func (w *world) theApplicationRuns(ctx context.Context) (context.Context, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.runErr = w.application.Run(runCtx)

	return ctx, nil
}

func (w *world) theContextIsCancelledWhileRunning(ctx context.Context) (context.Context, error) {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	done := make(chan struct{})

	go func() {
		defer close(done)

		w.runErr = w.application.Run(runCtx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(settleTimeout):
		return ctx, errors.New("run did not return after cancellation") //nolint:err113 // assertion
	}

	return ctx, nil
}

func (w *world) theApplicationIsCleanedUp(ctx context.Context) (context.Context, error) {
	w.cleanupErr = w.application.Cleanup(ctx)

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *world) theBuildSucceeds(ctx context.Context) (context.Context, error) {
	if w.buildErr != nil {
		return ctx, fmt.Errorf("expected build to succeed: %w", w.buildErr)
	}

	return ctx, nil
}

func (w *world) theBuildFailsWith(ctx context.Context, want string) (context.Context, error) {
	if w.buildErr == nil {
		return ctx, errors.New("expected build to fail") //nolint:err113 // assertion
	}

	if !strings.Contains(w.buildErr.Error(), want) {
		return ctx, fmt.Errorf("build error %q does not mention %q", w.buildErr, want)
	}

	return ctx, nil
}

func (w *world) theApplicationCanStillBeCleanedUp(ctx context.Context) (context.Context, error) {
	if w.application == nil {
		return ctx, errors.New("Build returned a nil application") //nolint:err113 // assertion
	}

	if err := w.application.Cleanup(ctx); err != nil {
		return ctx, fmt.Errorf("cleanup after failed build: %w", err)
	}

	return ctx, nil
}

func (w *world) runReturnsThatError(ctx context.Context) (context.Context, error) {
	if !errors.Is(w.runErr, errStartupFailed) {
		return ctx, fmt.Errorf("expected the startup error, got %v", w.runErr)
	}

	return ctx, nil
}

func (w *world) runReturnsACancellationError(ctx context.Context) (context.Context, error) {
	if !errors.Is(w.runErr, context.Canceled) {
		return ctx, fmt.Errorf("expected a cancellation error, got %v", w.runErr)
	}

	return ctx, nil
}

// noGoroutineIsLeftBlocked is the regression test for the unbuffered error
// channel: with it, the blocking startup function could never complete its
// send once Run had returned.
func (w *world) noGoroutineIsLeftBlocked(ctx context.Context) (context.Context, error) {
	deadline := time.Now().Add(settleTimeout)

	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= w.goroutinesAtStart {
			return ctx, nil
		}

		time.Sleep(10 * time.Millisecond)
	}

	return ctx, fmt.Errorf(
		"goroutines grew from %d to %d",
		w.goroutinesAtStart, runtime.NumGoroutine(),
	)
}

func (w *world) theyRanInTheOrder(ctx context.Context, want string) (context.Context, error) {
	got := strings.Join(w.order, ",")
	if got != want {
		return ctx, fmt.Errorf("cleanup order was %q, want %q", got, want)
	}

	return ctx, nil
}

func (w *world) everyCleanupFunctionStillRan(ctx context.Context) (context.Context, error) {
	const registered = 3

	if len(w.order) != registered {
		return ctx, fmt.Errorf("%d of %d cleanup functions ran", len(w.order), registered)
	}

	return ctx, nil
}

func (w *world) theCleanupErrorMentions(ctx context.Context, want string) (context.Context, error) {
	if w.cleanupErr == nil {
		return ctx, errors.New("expected a cleanup error") //nolint:err113 // assertion
	}

	if !strings.Contains(w.cleanupErr.Error(), want) {
		return ctx, fmt.Errorf("cleanup error %q does not mention %q", w.cleanupErr, want)
	}

	return ctx, nil
}

// ---- wiring ----------------------------------------------------------------

// initializeLifecycle registers the lifecycle steps. The expressions read as
// the feature file does, so a failing step points straight at its line.
func initializeLifecycle(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w := &world{
			application:       apphelpers.New(),
			failing:           make(map[string]bool),
			order:             make([]string, 0, 3),
			goroutinesAtStart: runtime.NumGoroutine(),
		}

		return context.WithValue(ctx, worldKey{}, w), nil
	})

	sc.Step(`^no configuration path is set$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).noConfigurationPath(ctx)
	})
	sc.Step(`^the configuration path is "([^"]*)"$`, func(ctx context.Context, p string) (context.Context, error) {
		return from(ctx).configurationPathIs(ctx, p)
	})
	sc.Step(`^a startup function that fails after (\d+ms)$`, func(ctx context.Context, d string) (context.Context, error) {
		return from(ctx).aStartupFunctionThatFails(ctx, d)
	})
	sc.Step(`^a startup function that blocks until cancelled$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).aStartupFunctionThatBlocks(ctx)
	})
	sc.Step(`^cleanup functions "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(ctx context.Context, a, b, c string) (context.Context, error) {
		return from(ctx).cleanupFunctions(ctx, a, b, c)
	})
	sc.Step(`^cleanup function "([^"]*)" fails$`, func(ctx context.Context, n string) (context.Context, error) {
		return from(ctx).cleanupFunctionFails(ctx, n)
	})

	sc.Step(`^the application is built$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).theApplicationIsBuilt(ctx)
	})
	sc.Step(`^the application runs$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).theApplicationRuns(ctx)
	})
	sc.Step(`^the context is cancelled while running$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).theContextIsCancelledWhileRunning(ctx)
	})
	sc.Step(`^the application is cleaned up$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).theApplicationIsCleanedUp(ctx)
	})

	sc.Step(`^the build succeeds$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).theBuildSucceeds(ctx)
	})
	sc.Step(`^the build fails with "([^"]*)"$`, func(ctx context.Context, w string) (context.Context, error) {
		return from(ctx).theBuildFailsWith(ctx, w)
	})
	sc.Step(`^the returned application can still be cleaned up$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).theApplicationCanStillBeCleanedUp(ctx)
	})
	sc.Step(`^run returns that error$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).runReturnsThatError(ctx)
	})
	sc.Step(`^run returns a cancellation error$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).runReturnsACancellationError(ctx)
	})
	sc.Step(`^no startup goroutine is left blocked$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).noGoroutineIsLeftBlocked(ctx)
	})
	sc.Step(`^they ran in the order "([^"]*)"$`, func(ctx context.Context, w string) (context.Context, error) {
		return from(ctx).theyRanInTheOrder(ctx, w)
	})
	sc.Step(`^every cleanup function still ran$`, func(ctx context.Context) (context.Context, error) {
		return from(ctx).everyCleanupFunctionStillRan(ctx)
	})
	sc.Step(`^the cleanup error mentions "([^"]*)"$`, func(ctx context.Context, w string) (context.Context, error) {
		return from(ctx).theCleanupErrorMentions(ctx, w)
	})
}
