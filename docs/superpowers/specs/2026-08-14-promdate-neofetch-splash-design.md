# PROMDATE neofetch splash

Replace prism's block-glyph PRISM splash with a neofetch-style layout: a
hand-drawn PROMDATE logo on the left, a host-info column on the right.

Reference: [charmbracelet/vhs `examples/neofetch`](https://github.com/charmbracelet/vhs/tree/main/examples/neofetch).

## Scope

This changes the splash art and layout only. It is **not** a project rename:

- the binary stays `cmd/prism`
- the module stays `github.com/pantherhawk/prism`
- `tea.View.WindowTitle` stays `"prism"` (`internal/tui/view.go:50`)

PROMDATE is the wordmark drawn on the splash. Nothing else changes name.

## What we borrow from the VHS example

`vhs.ascii` is VHS's own logo, not a reusable template: 46 columns by 13 rows
for the three letters V-H-S, roughly 15 columns per letter. The art is not
reusable, but three things around it are.

**The banded colour ramp**, from `colorize-ascii.go`. It does not interpolate
between colours; it divides the lines into equal bands:

```go
step := len(lines) / len(colors)
n    := clamp(0, len(colors)-1, i/step)
```

`theme.Palette.Spectrum` is already a `[]color.Color` of five violet-to-red
bands, so it drops into that loop unchanged. This is why the design needs no
colour-interpolation helper in `theme`.

**The embedded asset layout.** The art lives in a plain `.ascii` file beside
the Go source and is pulled in with `//go:embed`, so it stays hand-editable
rather than becoming an escaped string literal.

**The info block shape**, from `vhs.conf`: a title line, a blank, a run of
label/value pairs, then a colour swatch row (`info cols`). Its `gap` setting is
the gutter between logo and info.

## Components

### `internal/banner/promdate.ascii` (new)

The logo, rendered in figlet's **isometric2** font — an isometric outline face
drawn from `_ / \ : | ~` rather than VHS's solid `G` / `#` fills.

Generated as the single line `PROM DATE`, then split into two stacked words.
Columns 54–63 of that render are blank across all eleven rows, which is the
word gutter and so the split point.

- rows 1–11: `PROM` (53 columns)
- row 12: blank separator
- rows 13–23: `DATE` (55 columns)

Total block: **55 x 23**.

Using a generated font rather than hand-drawn art is a deliberate change from
the VHS reference. It costs the hand-made character of `vhs.ascii` and buys
machine-consistent letterforms across eight glyphs, which is what keeps the
block to a shape the layout can reason about.

### `internal/banner/logo.go` (new)

Embeds `promdate.ascii` and colours it. Splits the art on newlines and applies
the banded ramp above, with `palette.Spectrum` as the colour list — five bands
across 23 rows, violet at the top through red at the bottom.

Twenty-three rows over five bands does not divide evenly, and this is where the
implementation departs from the VHS reference. VHS divides the row index by a
fixed step of `rows/bands`, which leaves the whole remainder on the last
colour: here that would be four rows each of violet through green under **seven
rows of red**, so `PROM` would carry three wavelengths and `DATE` barely two.
Scaling the index instead — `i*bands/rows` — spreads the remainder across the
ramp, giving stripes of 5, 5, 4, 5, 4. The clamp stays as a bounds guard, but
it is no longer load-bearing: the scaled index cannot overrun.

### `internal/hostinfo` (new)

Collects the host facts once, at startup. This is the only impure part of the
feature and it is quarantined in this package so that `banner` stays testable.

| Field   | Source                                            |
|---------|---------------------------------------------------|
| User    | `os/user.Current()`                               |
| Host    | `os.Hostname()`                                   |
| OS      | `runtime.GOOS` / `runtime.GOARCH`                 |
| Kernel  | `uname -r` equivalent via `golang.org/x/sys/unix` |
| Shell   | `$SHELL`, base name only                          |
| Term    | `$TERM_PROGRAM`, falling back to `$TERM`          |
| Go      | `runtime.Version()`                               |
| Uptime  | `kern.boottime` (darwin), `/proc/uptime` (linux)  |

Every field degrades to `""` when its source errors or is unset. `Collect`
returns no error: a splash screen is not worth failing a startup over. Empty
fields are omitted from the rendered block rather than drawn blank.

Uptime is formatted to a display string at collect time, not at render time, so
that a golden test never observes a moving value.

### `internal/banner` (changed)

The package doc's promise — a pure function of its inputs, given the same
`Info`, palette and width it produces the same string — is preserved. `Info`
grows plain string fields carrying the collected facts. `internal/app/build.go`
fills them from `hostinfo.Collect()` where it already builds `banner.Info`
(`build.go:99`).

The five-row block font (`glyphs`) and the `wordmark` constant are removed.

## Layout

`Render` picks one of three tiers from the terminal dimensions.

**Full (width >= 92, height >= 26).** Logo left, info right, joined with
`lipgloss.JoinHorizontal`, separated by the gutter. The whole block is centred
horizontally as it is today.

The constants gate the tier but cannot be the whole test, because the info
column is as wide as the facts it was handed and `sourceName` can return an
arbitrarily long upstream URL. `Render` therefore composes the full block and
measures it, falling back to compact if it would not fit the width after all.

**Compact (width >= `minWidth`, height >= `minHeight`).** The logo is omitted
and the info block is drawn alone beneath a styled `PROMDATE` title line. This
mirrors real neofetch's `--off` mode, so it reads as an intended mode rather
than a degraded one, and it means one art asset rather than two.

**Too small (below `minWidth` or `minHeight`).** Unchanged — `internal/tui/view.go`
already returns the "terminal too small" message before `banner.Render` is
reached.

A 55 x 23 logo, a 4-column gutter and a ~32-column info block total 91 columns,
and the composition is 25 rows tall with the keyhint.

This does **not** fit a stock 80 x 24 terminal, which is a deliberate trade
made when the wordmark moved from swampland to isometric2: the new face is half
again as large, and nothing short of a second art asset would keep a logo on an
80-column screen. A stock terminal therefore draws the compact tier, and the
full tier is the common case only on a window someone has sized up — which is
most of them, and all of the recorded ones.

The info block follows `vhs.conf`'s order: `user@host` title, blank, the
label/value pairs, then a spectrum swatch row built from `palette.Spectrum`.

### What happens to today's splash furniture

The current splash is logo, tagline, meta lines, keyhint. Under the new layout:

- **The tagline** ("split your metrics into their spectrum") is **dropped**.
  Neofetch has no tagline, and the info block now occupies that space.
- **The meta lines** (version, scrape endpoint, buffer) are **kept**, becoming
  three of the info block's label/value pairs rather than a separate stanza.
- **The keyhint** ("press any key to begin / ? for help") is **kept**, on both
  drawn tiers, as the last line beneath the whole composition. It is the only
  line telling the operator how to leave the splash, so it is never dropped for
  space.

## Testing

**Golden tests** for both drawn tiers, in dark and light, fed a fixed `Info` so
output is deterministic. These replace the existing banner goldens.

**`hostinfo`** gets a test asserting graceful degradation: with an empty
environment, `Collect` returns zero-valued fields and no panic.

**`internal/theme/theme_test.go:120`** — `TestSpectrumIsTheSameWidthInBothPalettes`
asserts that both palettes carry the same number of bands and that the spectrum
is non-empty. Both assertions stay true and stay useful: the banded ramp divides
by `len(colors)`, so an empty spectrum would panic. Only its comment ("the
banner draws one glyph per spectrum band, so the two lengths are coupled") needs
rewriting, since the ramp no longer maps one band to one glyph.

**`banner`'s ramp** gets a test that every band is reachable and that the clamp
holds when `len(lines)` is not a multiple of `len(colors)` — the case where
`i/step` overruns the final index.

**`vhs/p0-neofetch.tape`** joins the existing artifact series alongside
`p0-banner.tape`, rendering to `design/`. It must set a window large enough to
exercise the full tier.

## Risks

Low. The earlier draft of this design hand-drew the art at VHS density, which
was the main risk; a generated font removes it. The art is a fixed asset, and
the surrounding code is small and independently testable.

The one live risk is the size of the isometric2 block: it puts the full tier
out of reach of a stock 80 x 24 terminal, so anyone running the splash in a
default-sized window sees the compact tier and never the wordmark. That is an
accepted trade rather than a defect, but it is the thing to revisit first if
the splash reads as bare in the wild.

The remaining unknown is `hostinfo`'s platform code. Kernel and uptime are the
only fields needing per-OS implementations, they are the two least important
lines on the splash, and both degrade to omitted, so a wrong guess on a given
platform costs a line rather than a startup.
