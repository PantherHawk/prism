//go:build bdd

package features_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"

	"github.com/pantherhawk/prism/internal/theme"
)

// hexColour matches a literal colour written into source. The theme package is
// the only place one is allowed to appear.
var hexColour = regexp.MustCompile(`#[0-9a-fA-F]{6}`)

// themeRoots are the directories the colour rule covers, relative to this one.
var themeRoots = []string{"../internal", "../cmd"} //nolint:gochecknoglobals // immutable fixture

// themeExempt is the one directory colours belong to.
const themeExempt = "../internal/theme"

// themeWorld is the state of one theme scenario.
type themeWorld struct {
	cfg        theme.Config
	appearance *theme.Appearance
	invalid    error
	offenders  []string
}

type themeKey struct{}

func themeFrom(ctx context.Context) *themeWorld {
	w, ok := ctx.Value(themeKey{}).(*themeWorld)
	if !ok {
		panic("theme world missing from context")
	}

	return w
}

// ---- Given -----------------------------------------------------------------

// theThemeModeIs sets the configured mode. An invalid mode is recorded rather
// than failing the step, so that a scenario can assert on the rejection.
func (w *themeWorld) theThemeModeIs(ctx context.Context, mode string) (context.Context, error) {
	w.cfg = theme.Config{Mode: theme.Mode(mode)}
	w.invalid = w.cfg.Validate()

	if w.invalid == nil {
		w.appearance = theme.NewAppearance(w.cfg)
	}

	return ctx, nil
}

// ---- When ------------------------------------------------------------------

func (w *themeWorld) theTerminalReports(ctx context.Context, dark bool) (context.Context, error) {
	if w.appearance == nil {
		return ctx, errNoAppearance
	}

	w.appearance.TerminalIsDark(dark)

	return ctx, nil
}

func (w *themeWorld) iPressTheThemeKey(ctx context.Context) (context.Context, error) {
	if w.appearance == nil {
		return ctx, errNoAppearance
	}

	w.appearance.Toggle()

	return ctx, nil
}

// ---- Then ------------------------------------------------------------------

func (w *themeWorld) thePaletteInForceIs(ctx context.Context, wantDark bool) (context.Context, error) {
	if w.appearance == nil {
		return ctx, errNoAppearance
	}

	if got := w.appearance.Palette().Dark; got != wantDark {
		return ctx, fmt.Errorf("palette is dark=%v, want dark=%v", got, wantDark)
	}

	return ctx, nil
}

func (w *themeWorld) theThemeIsRejectedNaming(ctx context.Context, want string) (context.Context, error) {
	if w.invalid == nil {
		return ctx, errModeAccepted
	}

	if !errors.Is(w.invalid, theme.ErrUnknownMode) {
		return ctx, fmt.Errorf("rejected with %w, want ErrUnknownMode", w.invalid)
	}

	// The operator has to be told which value was rejected, or they go back to
	// the config file to guess which line it was.
	if !strings.Contains(w.invalid.Error(), want) {
		return ctx, fmt.Errorf("rejection %q does not name %q", w.invalid, want)
	}

	return ctx, nil
}

// everyColourRoleIsSet is the half of the palette contract a grep cannot see. A
// nil colour renders as the terminal default, which silently undoes the
// light-mode work in exactly the places nobody looks at.
func (w *themeWorld) everyColourRoleIsSet(ctx context.Context) (context.Context, error) {
	for _, mode := range []theme.Mode{theme.ModeDark, theme.ModeLight} {
		palette := theme.Resolve(theme.Config{Mode: mode}, false)

		roles := map[string]bool{
			"Background": palette.Background == nil,
			"Foreground": palette.Foreground == nil,
			"Muted":      palette.Muted == nil,
			"Chrome":     palette.Chrome == nil,
			"Accent":     palette.Accent == nil,
			"Secondary":  palette.Secondary == nil,
			"Warn":       palette.Warn == nil,
			"OK":         palette.OK == nil,
			"Selection":  palette.Selection == nil,
		}

		for role, missing := range roles {
			if missing {
				return ctx, fmt.Errorf("the %s palette leaves %s unset", mode, role)
			}
		}

		for i, band := range palette.Spectrum {
			if band == nil {
				return ctx, fmt.Errorf("the %s palette leaves spectrum band %d unset", mode, i)
			}
		}
	}

	return ctx, nil
}

func (w *themeWorld) theSpectrumMatchesInBothPalettes(ctx context.Context) (context.Context, error) {
	dark := theme.Resolve(theme.Config{Mode: theme.ModeDark}, false)
	light := theme.Resolve(theme.Config{Mode: theme.ModeLight}, false)

	if len(dark.Spectrum) == 0 {
		return ctx, errEmptySpectrum
	}

	if len(dark.Spectrum) != len(light.Spectrum) {
		return ctx, fmt.Errorf("spectrum is %d bands dark and %d light",
			len(dark.Spectrum), len(light.Spectrum))
	}

	return ctx, nil
}

// noHexOutsideTheThemePackage is P6's acceptance criterion, executed.
//
// CI greps for the same thing, but a rule that only lives in a workflow file is
// a rule that a developer discovers after pushing. Running it here means the
// answer arrives from `make bdd` instead.
func (w *themeWorld) noHexOutsideTheThemePackage(ctx context.Context) (context.Context, error) {
	exempt, err := filepath.Abs(themeExempt)
	if err != nil {
		return ctx, fmt.Errorf("resolve the theme package: %w", err)
	}

	for _, root := range themeRoots {
		if err := w.scan(root, exempt); err != nil {
			return ctx, err
		}
	}

	if len(w.offenders) > 0 {
		return ctx, fmt.Errorf("%w: %s", errColourOutsideTheme, strings.Join(w.offenders, ", "))
	}

	return ctx, nil
}

// scan walks a directory collecting Go files that name a colour.
func (w *themeWorld) scan(root, exempt string) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			absolute, absErr := filepath.Abs(path)
			if absErr != nil {
				return absErr
			}

			if absolute == exempt {
				return filepath.SkipDir
			}

			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		return w.inspect(path)
	})
	if err != nil {
		return fmt.Errorf("walk %s: %w", root, err)
	}

	return nil
}

// inspect records a file if it contains a hex colour.
func (w *themeWorld) inspect(path string) error {
	body, err := os.ReadFile(path) //nolint:gosec // the path comes from walking this repository
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if match := hexColour.Find(body); match != nil {
		w.offenders = append(w.offenders, fmt.Sprintf("%s (%s)", path, match))
	}

	return nil
}

// ---- wiring ----------------------------------------------------------------

var (
	errNoAppearance       = errors.New("no appearance was built; the mode was rejected")
	errModeAccepted       = errors.New("an unknown theme mode was accepted")
	errEmptySpectrum      = errors.New("the spectrum is empty")
	errColourOutsideTheme = errors.New("hex colours must live in internal/theme")
)

// initializeTheme registers the appearance steps.
func initializeTheme(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, themeKey{}, &themeWorld{}), nil
	})

	sc.Step(`^the theme mode is "([^"]*)"$`, func(ctx context.Context, mode string) (context.Context, error) {
		return themeFrom(ctx).theThemeModeIs(ctx, mode)
	})

	sc.Step(`^the terminal reports a dark background$`, func(ctx context.Context) (context.Context, error) {
		return themeFrom(ctx).theTerminalReports(ctx, true)
	})
	sc.Step(`^the terminal reports a light background$`, func(ctx context.Context) (context.Context, error) {
		return themeFrom(ctx).theTerminalReports(ctx, false)
	})
	sc.Step(`^I press the theme key$`, func(ctx context.Context) (context.Context, error) {
		return themeFrom(ctx).iPressTheThemeKey(ctx)
	})

	sc.Step(`^the dark palette is in force$`, func(ctx context.Context) (context.Context, error) {
		return themeFrom(ctx).thePaletteInForceIs(ctx, true)
	})
	sc.Step(`^the light palette is in force$`, func(ctx context.Context) (context.Context, error) {
		return themeFrom(ctx).thePaletteInForceIs(ctx, false)
	})
	sc.Step(`^the theme is rejected naming "([^"]*)"$`,
		func(ctx context.Context, want string) (context.Context, error) {
			return themeFrom(ctx).theThemeIsRejectedNaming(ctx, want)
		})
	sc.Step(`^every colour role is set$`, func(ctx context.Context) (context.Context, error) {
		return themeFrom(ctx).everyColourRoleIsSet(ctx)
	})
	sc.Step(`^the spectrum has the same number of bands in both palettes$`,
		func(ctx context.Context) (context.Context, error) {
			return themeFrom(ctx).theSpectrumMatchesInBothPalettes(ctx)
		})
	sc.Step(`^no Go file outside internal/theme contains a hex colour$`,
		func(ctx context.Context) (context.Context, error) {
			return themeFrom(ctx).noHexOutsideTheThemePackage(ctx)
		})
}
