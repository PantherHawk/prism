# PROMDATE Neofetch Splash Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace prism's block-glyph PRISM splash with a neofetch-style layout — a stacked PROM/DATE logo in figlet's swampland font on the left, a host-info column on the right.

**Architecture:** A new `internal/hostinfo` package collects host facts once at startup and hands them to `banner.Info` as plain strings, which keeps `internal/banner` a pure function of its inputs. `banner` embeds the art as a `.ascii` asset and colours it with a banded ramp borrowed from charmbracelet/vhs `examples/neofetch`, using `theme.Palette.Spectrum` as the band list. `Render` picks a full or compact layout from the terminal size.

**Tech Stack:** Go 1.25.4, `charm.land/lipgloss/v2`, `//go:embed`, `golang.org/x/sys/unix` (promoted from indirect), `github.com/charmbracelet/x/ansi` (promoted from indirect, test-only).

**Spec:** `docs/superpowers/specs/2026-08-14-promdate-neofetch-splash-design.md`

## Global Constraints

- **This is not a project rename.** The binary stays `cmd/prism`, the module stays `github.com/pantherhawk/prism`, and `tea.View.WindowTitle` stays `"prism"` (`internal/tui/view.go:50`). PROMDATE is only the wordmark drawn on the splash.
- **`internal/banner` must stay a pure function of its inputs.** Given the same `Info`, palette, styles and dimensions it must produce the same string. No clocks, no environment reads, no syscalls inside the package. This is what its package doc promises and what makes it testable.
- **Only `internal/theme` may name a colour.** Every colour used by the logo comes from `theme.Palette.Spectrum`; no hex literals anywhere in `internal/banner`.
- **`hostinfo.Collect` never fails.** It returns no error and every field degrades to `""`. A splash screen must not be able to fail a startup.
- **Empty fields are omitted, never drawn blank.** This already holds for `Info.Version` / `Endpoint` / `Buffer` in `banner.meta` and must hold for every new field.
- **Lint:** the repo runs `golangci-lint` with `gochecknoglobals` enabled. Package-level variables need `//nolint:gochecknoglobals` with a reason comment, as `banner.glyphs` does today.
- **Commands:** `task test` runs `go test -race -shuffle=on -cover ./...`. `task lint` runs `golangci-lint run ./...`. `task build` builds into `./bin`.

---

## File Structure

**Create:**
- `internal/hostinfo/hostinfo.go` — `Facts` struct and `Collect()`; the portable fields.
- `internal/hostinfo/kernel_darwin.go` — `kernel()` and `uptime()` via `unix.Sysctl`.
- `internal/hostinfo/kernel_linux.go` — `kernel()` via `unix.Uname`, `uptime()` via `/proc/uptime`.
- `internal/hostinfo/kernel_other.go` — build-tagged stubs returning zero values.
- `internal/hostinfo/hostinfo_test.go` — degradation and formatting tests.
- `internal/banner/promdate.ascii` — the logo asset.
- `internal/banner/logo.go` — embeds and colours the art.
- `internal/banner/logo_test.go` — art shape and band coverage.
- `internal/banner/banner_test.go` — layout tests for both tiers.
- `vhs/p0-neofetch.tape` — artifact-series recording.

**Modify:**
- `internal/banner/banner.go` — `Info` grows; `glyphs`/`wordmark`/`mark`/`meta` removed; `Render` gains a height parameter and tier selection.
- `internal/tui/view.go:61` — pass `m.height` to `banner.Render`.
- `internal/app/build.go:99` — fill the new `Info` fields from `hostinfo.Collect()`.
- `internal/theme/theme_test.go:119` — rewrite the stale comment above `TestSpectrumIsTheSameWidthInBothPalettes`.

---

## Task 1: `internal/hostinfo`

**Files:**
- Create: `internal/hostinfo/hostinfo.go`
- Create: `internal/hostinfo/kernel_darwin.go`
- Create: `internal/hostinfo/kernel_linux.go`
- Create: `internal/hostinfo/kernel_other.go`
- Test: `internal/hostinfo/hostinfo_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `hostinfo.Facts` with string fields `User, Host, OS, Kernel, Shell, Term, Go, Uptime`; `hostinfo.Collect() Facts`. Task 3 renders these; Task 5 calls `Collect`.

- [ ] **Step 1: Write the failing test**

`internal/hostinfo/hostinfo_test.go`:

```go
package hostinfo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pantherhawk/prism/internal/hostinfo"
)

// Collect must never panic and never fail, whatever the environment looks
// like. A splash screen is not worth failing a startup over.
func TestCollectDegradesRatherThanFailing(t *testing.T) {
	t.Setenv("SHELL", "")
	t.Setenv("TERM", "")
	t.Setenv("TERM_PROGRAM", "")

	facts := hostinfo.Collect()

	if facts.Shell != "" {
		t.Errorf("shell is %q with no $SHELL set, want empty", facts.Shell)
	}

	if facts.Term != "" {
		t.Errorf("term is %q with no $TERM set, want empty", facts.Term)
	}

	// Go version is compiled in, so it is the one field that is always known.
	if !strings.HasPrefix(facts.Go, "go") {
		t.Errorf("go version is %q, want a go-prefixed string", facts.Go)
	}
}

func TestShellIsReducedToItsBaseName(t *testing.T) {
	t.Setenv("SHELL", "/opt/homebrew/bin/zsh")

	if got := hostinfo.Collect().Shell; got != "zsh" {
		t.Errorf("shell is %q, want %q", got, "zsh")
	}
}

// TERM_PROGRAM names the emulator, which is what neofetch reports; TERM only
// names the terminfo entry, so it is the fallback rather than the first choice.
func TestTermPrefersTheProgramOverTheTerminfoEntry(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "ghostty")

	if got := hostinfo.Collect().Term; got != "ghostty" {
		t.Errorf("term is %q, want %q", got, "ghostty")
	}

	t.Setenv("TERM_PROGRAM", "")

	if got := hostinfo.Collect().Term; got != "xterm-256color" {
		t.Errorf("term fell back to %q, want %q", got, "xterm-256color")
	}
}

func TestUptimeIsFormattedInDaysHoursAndMinutes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{45 * time.Second, "less than a minute"},
		{9 * time.Minute, "9 mins"},
		{time.Hour + time.Minute, "1 hour, 1 min"},
		{50 * time.Hour, "2 days, 2 hours"},
	} {
		if got := hostinfo.FormatUptime(c.in); got != c.want {
			t.Errorf("FormatUptime(%s) is %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/hostinfo/...`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the portable half**

`internal/hostinfo/hostinfo.go`:

```go
// Package hostinfo collects the host facts prism's splash screen reports.
//
// It is the impure half of the splash, quarantined here so that
// [github.com/pantherhawk/prism/internal/banner] can stay a pure function of
// its inputs. Everything is gathered once at startup and handed on as plain
// strings; nothing in this package is called per frame.
//
// No function here returns an error. Every fact degrades to the empty string
// when its source is missing or fails, and the banner omits empty fields, so a
// host prism cannot introspect costs a line rather than a startup.
package hostinfo

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"
)

// Facts is everything the splash knows about the machine it is running on.
type Facts struct {
	User   string
	Host   string
	OS     string
	Kernel string
	Shell  string
	Term   string
	Go     string
	Uptime string
}

// Collect gathers the host facts. It never fails.
func Collect() Facts {
	return Facts{
		User:   username(),
		Host:   hostname(),
		OS:     runtime.GOOS + "/" + runtime.GOARCH,
		Kernel: kernel(),
		Shell:  shell(),
		Term:   terminal(),
		Go:     runtime.Version(),
		Uptime: FormatUptime(uptime()),
	}
}

func username() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}

	return current.Username
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}

	return name
}

// shell reports the login shell by base name: "/opt/homebrew/bin/zsh" is
// reported as "zsh", because the path is noise on a splash screen.
func shell() string {
	path := os.Getenv("SHELL")
	if path == "" {
		return ""
	}

	return filepath.Base(path)
}

// terminal prefers $TERM_PROGRAM, which names the emulator, over $TERM, which
// only names the terminfo entry it claims to implement.
func terminal() string {
	if program := os.Getenv("TERM_PROGRAM"); program != "" {
		return program
	}

	return os.Getenv("TERM")
}

// FormatUptime renders a duration the way neofetch does. A zero duration means
// the uptime could not be read, and formats as empty so the line is omitted.
//
// Exported because the formatting is the only part worth testing directly: the
// duration itself comes from a syscall that a test cannot pin.
func FormatUptime(d time.Duration) string {
	if d <= 0 {
		return ""
	}

	if d < time.Minute {
		return "less than a minute"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%s, %s", plural(days, "day"), plural(hours, "hour"))
	case hours > 0:
		return fmt.Sprintf("%s, %s", plural(hours, "hour"), plural(mins, "min"))
	default:
		return plural(mins, "min")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}

	return fmt.Sprintf("%d %ss", n, unit)
}
```

- [ ] **Step 4: Write the platform halves**

`internal/hostinfo/kernel_darwin.go`:

```go
//go:build darwin

package hostinfo

import (
	"time"

	"golang.org/x/sys/unix"
)

// kernel reports the Darwin release, which is what `uname -r` prints.
func kernel() string {
	release, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return ""
	}

	return "Darwin " + release
}

// uptime derives the boot time from the kernel and subtracts. Returns zero
// when it cannot be read, which FormatUptime renders as empty.
func uptime() time.Duration {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0
	}

	return time.Since(time.Unix(tv.Sec, int64(tv.Usec)*1000))
}
```

`internal/hostinfo/kernel_linux.go`:

```go
//go:build linux

package hostinfo

import (
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func kernel() string {
	var name unix.Utsname
	if err := unix.Uname(&name); err != nil {
		return ""
	}

	return strings.TrimRight(string(name.Sysname[:]), "\x00") +
		" " + strings.TrimRight(string(name.Release[:]), "\x00")
}

// uptime reads /proc/uptime, whose first field is seconds since boot.
func uptime() time.Duration {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	first, _, found := strings.Cut(string(raw), " ")
	if !found {
		return 0
	}

	secs, err := strconv.ParseFloat(first, 64)
	if err != nil {
		return 0
	}

	return time.Duration(secs * float64(time.Second))
}
```

`internal/hostinfo/kernel_other.go`:

```go
//go:build !darwin && !linux

package hostinfo

import "time"

// prism targets Unix. On anything else the two platform-specific facts are
// simply unknown, and the splash omits their lines.
func kernel() string { return "" }

func uptime() time.Duration { return 0 }
```

- [ ] **Step 5: Promote `golang.org/x/sys` to a direct dependency**

It is already in `go.sum` as an indirect dependency, so this only moves it in `go.mod`.

```bash
go get golang.org/x/sys@v0.47.0
go mod tidy
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -race ./internal/hostinfo/...`
Expected: PASS

- [ ] **Step 7: Lint**

Run: `golangci-lint run ./internal/hostinfo/...`
Expected: no findings.

- [ ] **Step 8: Commit**

```bash
git add internal/hostinfo go.mod go.sum
git commit -m "Add hostinfo, which collects the splash's host facts"
```

---

## Task 2: The logo asset and its colour ramp

**Files:**
- Create: `internal/banner/promdate.ascii`
- Create: `internal/banner/logo.go`
- Test: `internal/banner/logo_test.go`

**Interfaces:**
- Consumes: `theme.Palette.Spectrum` (`[]color.Color`).
- Produces: `logo(palette theme.Palette) []string` returning 15 styled rows; `logoWidth` and `logoHeight` constants (43 and 15); `band(i, rows, bands int) int`. Task 4 composes `logo` into the full tier.

- [ ] **Step 1: Write the art asset**

`internal/banner/promdate.ascii` — exactly 15 lines, no trailing blank line beyond the final newline. Lines 1-7 are PROM, line 8 is empty, lines 9-15 are DATE:

```
 ______   ______    ______   ___ __ __
/_____/\ /_____/\  /_____/\ /__//_//_/\
\:::_ \ \\:::_ \ \ \:::_ \ \\::\| \| \ \
 \:(_) \ \\:(_) ) )_\:\ \ \ \\:.      \ \
  \: ___\/ \: __ `\ \\:\ \ \ \\:.\-/\  \ \
   \ \ \    \ \ `\ \ \\:\_\ \ \\. \  \  \ \
    \_\/     \_\/ \_\/ \_____\/ \__\/ \__\/

 ______   ________   _________  ______
/_____/\ /_______/\ /________/\/_____/\
\:::_ \ \\::: _  \ \\__.::.__\/\::::_\/_
 \:\ \ \ \\::(_)  \ \  \::\ \   \:\/___/\
  \:\ \ \ \\:: __  \ \  \::\ \   \::___\/_
   \:\/.:| |\:.\ \  \ \  \::\ \   \:\____/\
    \____/_/ \__\/\__\/   \__\/    \_____\/
```

- [ ] **Step 2: Write the failing test**

`internal/banner/logo_test.go`:

```go
package banner

import (
	"strings"
	"testing"

	"github.com/pantherhawk/prism/internal/theme"
)

// The layout maths in Render depends on these, so a hand-edit to the asset
// that changes its shape must fail here rather than silently misalign the
// splash.
func TestTheArtIsTheDeclaredShape(t *testing.T) {
	t.Parallel()

	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")

	if len(lines) != logoHeight {
		t.Fatalf("art is %d rows, want %d", len(lines), logoHeight)
	}

	for i, line := range lines {
		if len(line) > logoWidth {
			t.Errorf("art row %d is %d columns, want at most %d", i, len(line), logoWidth)
		}
	}
}

// Every band must be reachable, or the spectrum is decorative rather than used.
func TestBandCoversEveryColour(t *testing.T) {
	t.Parallel()

	const bands = 5

	seen := make(map[int]bool, bands)
	for i := range logoHeight {
		seen[band(i, logoHeight, bands)] = true
	}

	for b := range bands {
		if !seen[b] {
			t.Errorf("band %d is never selected across %d rows", b, logoHeight)
		}
	}
}

// The reference implementation this is borrowed from divides row index by a
// step, which overruns the final index whenever the row count is not a
// multiple of the band count. The clamp is what makes that safe.
func TestBandClampsWhenRowsDoNotDivideEvenly(t *testing.T) {
	t.Parallel()

	for _, rows := range []int{7, 13, 16, 27} {
		const bands = 5

		for i := range rows {
			if got := band(i, rows, bands); got < 0 || got >= bands {
				t.Errorf("band(%d, %d, %d) is %d, out of range", i, rows, bands, got)
			}
		}
	}
}

func TestBandHandlesFewerRowsThanBands(t *testing.T) {
	t.Parallel()

	if got := band(0, 1, 5); got != 0 {
		t.Errorf("band(0, 1, 5) is %d, want 0", got)
	}
}

func TestLogoReturnsOneStyledRowPerArtRow(t *testing.T) {
	t.Parallel()

	rows := logo(theme.Resolve(theme.Config{Mode: theme.ModeDark}, true))

	if len(rows) != logoHeight {
		t.Errorf("logo returned %d rows, want %d", len(rows), logoHeight)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/banner/...`
Expected: FAIL — `art`, `logoHeight`, `logoWidth`, `band` and `logo` are undefined.

- [ ] **Step 4: Write the implementation**

`internal/banner/logo.go`:

```go
package banner

import (
	_ "embed"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/pantherhawk/prism/internal/theme"
)

const (
	// logoWidth and logoHeight are the art's dimensions. Render's layout maths
	// depends on them, and logo_test.go asserts the asset still matches.
	logoWidth  = 43
	logoHeight = 15
)

// art is the wordmark: PROM stacked over DATE, rendered in figlet's swampland
// font. It is a file rather than a string literal so that it stays editable as
// art — every other character in it is a backslash, and escaping the whole
// thing into Go source would make it unreadable and unfixable.
//
//go:embed promdate.ascii
var art string //nolint:gochecknoglobals // immutable embedded asset

// logo returns the art with one spectrum band applied per horizontal stripe.
//
// The banding is borrowed from charmbracelet/vhs examples/neofetch: rather
// than interpolating a gradient, the rows are cut into as many equal stripes
// as there are colours. Fifteen rows over five bands divides exactly, so each
// wavelength covers three rows and the beam reads as split rather than blurred.
func logo(palette theme.Palette) []string {
	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")
	rows := make([]string, len(lines))

	for i, line := range lines {
		colour := palette.Spectrum[band(i, len(lines), len(palette.Spectrum))]
		rows[i] = lipgloss.NewStyle().Foreground(colour).Render(line)
	}

	return rows
}

// band returns the colour index for row i of rows, given bands colours.
//
// The clamp is load-bearing: rows/bands truncates, so the last rows of an art
// whose height is not a multiple of the band count would index past the end.
func band(i, rows, bands int) int {
	step := rows / bands
	if step == 0 {
		step = 1
	}

	return min(bands-1, max(0, i/step))
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -race ./internal/banner/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/banner/promdate.ascii internal/banner/logo.go internal/banner/logo_test.go
git commit -m "Add the PROMDATE logo asset and its spectrum ramp"
```

---

## Task 3: The info block

**Files:**
- Modify: `internal/banner/banner.go`
- Test: `internal/banner/banner_test.go`

**Interfaces:**
- Consumes: `hostinfo.Facts` field names from Task 1; `theme.Styles`, `theme.Palette`.
- Produces: `Info` with fields `Version, Endpoint, Buffer, User, Host, OS, Kernel, Shell, Term, Go, Uptime`; `infoBlock(info Info, palette theme.Palette, styles theme.Styles) []string`. Task 4 composes `infoBlock` into both tiers.

- [ ] **Step 1: Write the failing test**

`internal/banner/banner_test.go`:

```go
package banner

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/pantherhawk/prism/internal/theme"
)

func testPalette() (theme.Palette, theme.Styles) {
	palette := theme.Resolve(theme.Config{Mode: theme.ModeDark}, true)

	return palette, theme.NewStyles(palette)
}

func fullInfo() Info {
	return Info{
		Version:  "v0.4.1",
		Endpoint: "localhost:9090",
		Buffer:   "15m ring · 1s buckets",
		User:     "alex",
		Host:     "studio",
		OS:       "darwin/arm64",
		Kernel:   "Darwin 25.5.0",
		Shell:    "zsh",
		Term:     "ghostty",
		Go:       "go1.25.4",
		Uptime:   "4 days, 2 hours",
	}
}

func TestInfoBlockLeadsWithUserAtHost(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	first := ansi.Strip(infoBlock(fullInfo(), palette, styles)[0])
	if first != "alex@studio" {
		t.Errorf("info block leads with %q, want %q", first, "alex@studio")
	}
}

func TestInfoBlockRendersEveryPopulatedFact(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	body := ansi.Strip(strings.Join(infoBlock(fullInfo(), palette, styles), "\n"))

	for _, want := range []string{
		"v0.4.1", "localhost:9090", "15m ring · 1s buckets", "darwin/arm64",
		"Darwin 25.5.0", "zsh", "ghostty", "go1.25.4", "4 days, 2 hours",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("info block is missing %q\n%s", want, body)
		}
	}
}

// An unreadable fact must cost its line, not leave a labelled blank.
func TestInfoBlockOmitsEmptyFacts(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	info := fullInfo()
	info.Kernel = ""
	info.Uptime = ""

	body := ansi.Strip(strings.Join(infoBlock(info, palette, styles), "\n"))

	for _, absent := range []string{"kernel", "uptime"} {
		if strings.Contains(body, absent) {
			t.Errorf("info block still labels %q with no value\n%s", absent, body)
		}
	}
}

// With no facts at all the block is still the title and the swatch, never a
// bare list of labels.
func TestInfoBlockSurvivesAnEmptyInfo(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	if got := infoBlock(Info{}, palette, styles); len(got) == 0 {
		t.Error("info block is empty for a zero Info")
	}
}

func TestInfoBlockEndsWithASwatchPerSpectrumBand(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	block := infoBlock(fullInfo(), palette, styles)
	last := ansi.Strip(block[len(block)-1])

	if want := len(palette.Spectrum) * swatchWidth; len([]rune(last)) != want {
		t.Errorf("swatch row is %d cells, want %d", len([]rune(last)), want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/banner/...`
Expected: FAIL — `infoBlock`, `swatchWidth` and the new `Info` fields are undefined.

- [ ] **Step 3: Promote `github.com/charmbracelet/x/ansi` to a direct dependency**

Already in `go.sum` as indirect; the test above imports it directly.

```bash
go get github.com/charmbracelet/x/ansi@v0.11.7
go mod tidy
```

- [ ] **Step 4: Replace `Info` and `meta` in `banner.go`**

Delete the `Info` struct and the `meta` function, and replace them with:

```go
// Info is the runtime and host detail shown beside the wordmark. Every field
// is a plain string, already formatted: the banner is a pure function of its
// inputs, so nothing here may be derived at render time. Empty fields are
// omitted rather than rendered blank.
type Info struct {
	// prism's own runtime.
	Version  string
	Endpoint string
	Buffer   string

	// The host, from internal/hostinfo.
	User   string
	Host   string
	OS     string
	Kernel string
	Shell  string
	Term   string
	Go     string
	Uptime string
}

// swatchWidth is how many cells each spectrum band occupies in the colour row.
const swatchWidth = 3

// infoBlock renders the neofetch column: a user@host title, a rule, the facts
// as an aligned two-column list, then the spectrum as colour blocks.
//
// The shape follows neofetch's own config, which is a title, a run of
// label/value pairs and a trailing colour row.
func infoBlock(info Info, palette theme.Palette, styles theme.Styles) []string {
	lines := make([]string, 0, 14)

	title := heading(info, styles)
	lines = append(lines, title, styles.Chrome.Render(strings.Repeat("─", ansi.StringWidth(title))))

	for _, fact := range []struct{ label, value string }{
		{"version", info.Version},
		{"scrape", info.Endpoint},
		{"buffer", info.Buffer},
		{"os", info.OS},
		{"kernel", info.Kernel},
		{"shell", info.Shell},
		{"term", info.Term},
		{"go", info.Go},
		{"uptime", info.Uptime},
	} {
		if fact.value == "" {
			continue
		}

		lines = append(lines, join(
			styles.Muted.Render(label(fact.label)),
			styles.Base.Render(fact.value),
		))
	}

	return append(lines, "", swatch(palette))
}

// heading renders the user@host line, falling back to the product name when
// neither is known.
func heading(info Info, styles theme.Styles) string {
	switch {
	case info.User != "" && info.Host != "":
		return styles.Title.Render(info.User) +
			styles.Chrome.Render("@") +
			styles.Title.Render(info.Host)
	case info.Host != "":
		return styles.Title.Render(info.Host)
	default:
		return styles.Title.Render("promdate")
	}
}

// swatch renders the spectrum as solid blocks, the way neofetch closes its
// info column with the terminal's palette.
func swatch(palette theme.Palette) string {
	var row strings.Builder

	for _, colour := range palette.Spectrum {
		row.WriteString(lipgloss.NewStyle().
			Foreground(colour).
			Render(strings.Repeat("█", swatchWidth)))
	}

	return row.String()
}
```

Add `"github.com/charmbracelet/x/ansi"` to the import block. `labelWidth` stays at 6, which fits the longest label (`version`, 7) — widen it to 8 so values still form a column:

```go
	// labelWidth pads the runtime labels so their values form a column.
	labelWidth = 8
```

- [ ] **Step 5: Keep the tree compiling**

Deleting `meta` breaks `Render`, which still calls it and `mark`. Rather than
leave a broken intermediate state, retire the block font here and give `Render`
the logo-less composition — Task 4 adds the logo and the tier split on top.

Delete `glyphHeight`, the `wordmark` constant, the `glyphs` map and the `mark`
function, then replace `Render` with:

```go
// Render returns the splash screen, centred within width.
func Render(info Info, palette theme.Palette, styles theme.Styles, width int) string {
	lines := append(
		[]string{styles.Accent.Render("PROMDATE"), ""},
		infoBlock(info, palette, styles)...,
	)
	lines = append(lines, "", hint(styles))

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Left, lines...))
}
```

The tagline is dropped here and never returns: the info block now occupies that
space, and neofetch has no tagline.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -race ./internal/banner/... ./internal/tui/...`
Expected: PASS. The tree compiles and the splash draws — without the logo yet.

- [ ] **Step 7: Commit**

```bash
git add internal/banner go.mod go.sum
git commit -m "Render the splash's host facts as a neofetch info column"
```

---

## Task 4: The two-tier layout

**Files:**
- Modify: `internal/banner/banner.go`
- Modify: `internal/tui/view.go:61`
- Test: `internal/banner/banner_test.go`

**Interfaces:**
- Consumes: `logo` (Task 2), `infoBlock` (Task 3).
- Produces: `Render(info Info, palette theme.Palette, styles theme.Styles, width, height int) string` — note the **new `height` parameter**. Task 5 does not change this further.

- [ ] **Step 1: Write the failing test**

Append to `internal/banner/banner_test.go`:

```go
// The full tier must fit a stock 80x24 terminal — that is the whole reason the
// logo is stacked rather than drawn on one line.
func TestFullTierFitsAStockTerminal(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	out := Render(fullInfo(), palette, styles, 80, 24)

	for i, line := range strings.Split(out, "\n") {
		if got := ansi.StringWidth(line); got > 80 {
			t.Errorf("row %d is %d columns wide, want at most 80", i, got)
		}
	}

	if got := len(strings.Split(out, "\n")); got > 24 {
		t.Errorf("splash is %d rows, want at most 24", got)
	}
}

func TestFullTierDrawsTheLogoBesideTheFacts(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	out := ansi.Strip(Render(fullInfo(), palette, styles, 100, 30))

	// A row carrying both art and a fact is what "side by side" means.
	if !strings.Contains(out, "ghostty") {
		t.Fatal("full tier dropped the facts")
	}

	var joined bool

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, `\_\/`) && strings.Contains(line, "darwin/arm64") {
			joined = true
		}
	}

	if !joined {
		t.Errorf("no row carries both the logo and a fact:\n%s", out)
	}
}

// Below the full tier the logo is dropped rather than wrapped, which is what
// neofetch --off does.
func TestCompactTierDropsTheLogoAndKeepsTheFacts(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	out := ansi.Strip(Render(fullInfo(), palette, styles, 62, 20))

	if strings.Contains(out, `\_\/`) {
		t.Error("compact tier still drew the logo")
	}

	if !strings.Contains(out, "localhost:9090") {
		t.Error("compact tier dropped the facts")
	}

	if !strings.Contains(out, "PROMDATE") {
		t.Error("compact tier dropped the wordmark title")
	}

	for i, line := range strings.Split(out, "\n") {
		if got := ansi.StringWidth(line); got > 62 {
			t.Errorf("row %d is %d columns wide, want at most 62", i, got)
		}
	}
}

// The keyhint is the only line telling the operator how to leave the splash,
// so it survives on both tiers.
func TestBothTiersKeepTheKeyhint(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	for _, c := range []struct{ w, h int }{{100, 30}, {62, 20}} {
		out := ansi.Strip(Render(fullInfo(), palette, styles, c.w, c.h))
		if !strings.Contains(out, "any key") {
			t.Errorf("%dx%d dropped the keyhint:\n%s", c.w, c.h, out)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	t.Parallel()

	palette, styles := testPalette()

	first := Render(fullInfo(), palette, styles, 100, 30)
	if second := Render(fullInfo(), palette, styles, 100, 30); first != second {
		t.Error("Render is not a pure function of its inputs")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/banner/...`
Expected: FAIL — `Render` does not take a height.

- [ ] **Step 3: Add the tiers to `Render`**

The block font is already gone (Task 3). In `internal/banner/banner.go`, replace `Render` and add the two layout functions:

```go
const (
	// fullWidth and fullHeight are the smallest terminal that gets the logo
	// beside the facts. Below either, the logo is dropped rather than wrapped.
	//
	// The width is the art plus the gutter plus a comfortable info column; the
	// height is the art plus the keyhint and its spacing. A stock terminal is
	// 80x24, so the full tier is the common case.
	fullWidth  = 80
	fullHeight = 22

	// gutter is the gap between the logo and the info column.
	gutter = 4
)

// Render returns the splash screen, centred within width.
//
// It is a pure function of its inputs: the same Info, palette, styles and
// dimensions always produce the same string.
func Render(info Info, palette theme.Palette, styles theme.Styles, width, height int) string {
	var block string

	if width >= fullWidth && height >= fullHeight {
		block = full(info, palette, styles)
	} else {
		block = compact(info, palette, styles)
	}

	block = lipgloss.JoinVertical(lipgloss.Left, block, "", hint(styles))

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
}

// full draws the logo beside the facts.
func full(info Info, palette theme.Palette, styles theme.Styles) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, logo(palette)...),
		strings.Repeat(" ", gutter),
		lipgloss.JoinVertical(lipgloss.Left, infoBlock(info, palette, styles)...),
	)
}

// compact drops the logo and titles the facts with the wordmark instead.
//
// This is neofetch's own --off mode rather than a degradation: an art block 43
// columns wide does not belong in a split pane, and wrapping it would be worse
// than omitting it.
func compact(info Info, palette theme.Palette, styles theme.Styles) string {
	lines := append(
		[]string{styles.Accent.Render("PROMDATE"), ""},
		infoBlock(info, palette, styles)...,
	)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
```

- [ ] **Step 4: Update the caller**

`internal/tui/view.go:61` — pass the height:

```go
	case m.splash:
		return m.centred(banner.Render(m.info, m.palette, m.styles, m.width, m.height))
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -race ./internal/banner/... ./internal/tui/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/banner internal/tui/view.go
git commit -m "Lay the splash out as neofetch, with a logo-less compact tier"
```

---

## Task 5: Wire it up and record the artifact

**Files:**
- Modify: `internal/app/build.go:99`
- Modify: `internal/theme/theme_test.go:119`
- Create: `vhs/p0-neofetch.tape`

**Interfaces:**
- Consumes: `hostinfo.Collect()` (Task 1), `banner.Info` (Task 3).
- Produces: nothing further.

- [ ] **Step 1: Fill the new fields in `build.go`**

Replace the `splash := banner.Info{...}` literal at `internal/app/build.go:99`:

```go
	facts := hostinfo.Collect()

	splash := banner.Info{
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
```

Add `"github.com/pantherhawk/prism/internal/hostinfo"` to the imports.

- [ ] **Step 2: Fix the stale comment in `theme_test.go`**

The comment above `TestSpectrumIsTheSameWidthInBothPalettes` at `internal/theme/theme_test.go:119` describes a coupling that no longer exists. The assertions stay; only the reason changes:

```go
// The banner cuts its art into one stripe per spectrum band, so an empty or
// lopsided spectrum would divide by zero or leave a wavelength undrawn.
```

- [ ] **Step 3: Write the tape**

`vhs/p0-neofetch.tape`, modelled on `vhs/p0-banner.tape`. The window must be wide enough to exercise the full tier, and the screenshot must land inside `tui.splashFor` (1200ms) — the comment in `p0-banner.tape` explains why that window is narrow:

```
# Phase 0 - the splash screen. prism starts, resolves its theme from the
# terminal background, and draws the PROMDATE wordmark beside the host facts.
#
# The capture window is the same one p0-banner.tape documents: after prism has
# drawn and before the splash retires itself at tui.splashFor, which is 1200ms.
#
# The width matters here in a way it does not for the other tapes. Below 80
# columns the splash drops the logo and draws its compact tier, so a narrow
# window would photograph the fallback and file it as the banner.
Output vhs/out/p0-neofetch.gif

Require prism

Set Shell        bash
Set FontFamily   "JetBrains Mono"
Set FontSize     18
Set Width        1400
Set Height       760
Set Padding      24
Set Margin       36
Set MarginFill   "#08070c"
Set BorderRadius 12
Set Theme        "Catppuccin Mocha"
Set TypingSpeed  55ms

Hide
Type "export PRISM_CONFIG=$PWD/deploy/prism-demo.yaml" Enter
Type "clear" Enter
Show

Type "prism"
Sleep 400ms
Enter
Sleep 500ms

Screenshot vhs/out/p0-neofetch-dark.png
Sleep 1s

Type "q"
Sleep 400ms
```

- [ ] **Step 4: Run the full suite**

Run: `task test`
Expected: PASS across all packages.

- [ ] **Step 5: Lint the tree**

Run: `task lint`
Expected: no findings.

- [ ] **Step 6: Look at the actual splash**

Run: `task build && ./bin/prism --help` to confirm it builds, then run it against the demo config and **look at the screen**. Confirm the logo reads PROM over DATE, the rainbow runs violet at the top to pink at the bottom, and the facts align in a column. Then resize the terminal below 80 columns and confirm the compact tier appears without the art wrapping.

- [ ] **Step 7: Commit**

```bash
git add internal/app/build.go internal/theme/theme_test.go vhs/p0-neofetch.tape
git commit -m "Feed the splash real host facts and record its artifact"
```

---

## Self-Review Notes

Checked against the spec:

- **Scope (no rename):** covered by Global Constraints; no task touches the module path, binary name or `WindowTitle`.
- **Banner purity:** Task 1 quarantines the impure code; Task 4 asserts determinism.
- **Borrowed ramp:** Task 2, with the clamp tested for the uneven case the reference would break on.
- **Embedded asset:** Task 2.
- **`hostinfo` fields and degradation:** Task 1, with the empty-environment test.
- **Uptime formatted at collect time:** Task 1, `FormatUptime` called inside `Collect`.
- **Two drawn tiers plus the untouched too-small path:** Task 4. `internal/tui/view.go` returns "terminal too small" before `Render` is reached, so no task changes it.
- **Info block order from `vhs.conf`:** Task 3 — title, rule, pairs, swatch.
- **Tagline dropped, meta kept as pairs, keyhint kept on both tiers:** Task 3 (pairs), Task 4 (keyhint test). The tagline simply has no code in any task.
- **Theme comment rewrite:** Task 5.
- **Tape:** Task 5.

Two deviations from the spec, both deliberate:

1. The spec's "golden tests" are implemented as property assertions over
   ANSI-stripped output rather than committed golden files. The repo has no
   golden-file helper to follow, and a committed file full of escape sequences
   would fail on any palette tweak while telling the reader nothing. The
   assertions cover what the spec wanted from goldens: fixed input, deterministic
   output, and a failure when the layout moves.
2. `labelWidth` widens from 6 to 8, which the spec does not mention. The longest
   label is now `version` at 7, and leaving it at 6 would break the value column.
