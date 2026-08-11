# prism — rollout plan

**prism** — split your metrics into their spectrum. A terminal UI for Prometheus
metrics, built on Bubble Tea v2 + Ultraviolet, targeting Envoy's
`/stats/prometheus` endpoint.

The name carries the design metaphor: a prism takes one beam and separates it by
wavelength. That is exactly what "pivot by attribute" does to a metric family,
and it gives the theme its spectral palette.

---

## 0. Verified facts this plan is built on

I read the upstream sources rather than trusting memory. Several things differ
from what you'd expect:

| Thing | Reality |
|---|---|
| Bubble Tea v2 import path | `charm.land/bubbletea/v2` — **not** `github.com/charmbracelet/...`. Same for `charm.land/lipgloss/v2`, `charm.land/bubbles/v2` |
| Versions | bubbletea `v2.0.8`, lipgloss `v2.0.5`, bubbles v2 — all stable, not beta |
| Go version | **1.25.0 minimum** (Ultraviolet and all v2 modules declare `go 1.25.0`) |
| `Model` interface | `Init() Cmd`, `Update(Msg) (Model, Cmd)`, **`View() View`** — View returns a struct, not a string |
| Alt screen | No `tea.WithAltScreen` option any more. Set `v.AltScreen = true` on the returned `tea.View` |
| `tea.Msg` | Type alias for `uv.Event` — Ultraviolet events *are* Bubble Tea messages |
| Ultraviolet | `uv.DefaultTerminal()`, `Screen`/`Buffer`/`Window`, `screen.Context` for drawing, `layout` package with a Cassowary constraint solver |
| Dark mode | `lipgloss.LightDark(isDark)` returns a `LightDarkFunc(light, dark)` picker; `tea.RequestBackgroundColor()` → `BackgroundColorMsg.IsDark()` for auto-detection |
| Physics | `github.com/charmbracelet/harmonica` — `NewSpring(FPS(60), freq, damping)`, `Update(pos, vel, target)`. Already a bubbles v2 dependency, so it's free |
| golangci-lint | v2 schema: `version: "2"`, `linters.default: all`, separate top-level `formatters:` block |

---

## 1. Architecture

```
cmd/prism/main.go            signals, env lookup, os.Exit — nothing else
internal/
  app/build.go               Build(): the only place apphelpers is wired
  apphelpers/app.go          Run / Cleanup / AddStartupFuncs / AddCleanupFuncs
  config/config.go           Read() — composed of sub-package configs
  logging/  telemetry/       logging.Config, telemetry.Config (OTEL SDK)
  scrape/                    streaming expfmt decode, scrape scheduler
  series/                    ring buffer, labelset interning, cardinality
  tempstore/                 spill-to-tempfile for cold buckets
  stream/                    SSE fan-out server + client
  theme/                     spectral palettes, LightDark wiring
  chart/                     braille canvas, axis, spring-eased scales
  tui/                       Bubble Tea v2 model, panels, keymap
features/                    godog acceptance criteria
vhs/                         one .tape per phase
```

**Data flow.** `scrape` streams the endpoint through `expfmt.TextParser` without
buffering the whole body, converts to `series.Sample`, and publishes on a
channel. `series.Store` owns a fixed-capacity ring of time buckets; buckets older
than the in-memory window spill to a temp file via `tempstore` and are read back
on demand when you pan left. `stream` re-broadcasts deltas over SSE so the TUI is
a *client* of the collector, not coupled to it. `tui` renders from a read-only
snapshot — never from the live store — so a slow scrape can never stall a frame.

---

## 2. Phases

Scenarios for phases that have not landed are tagged `@wip` and excluded by the
runner, so a feature file can be written with the plan rather than after the
code. Removing the tag is part of finishing the phase.

Each phase ends with a VHS tape that appends to the walkthrough, so the PNG
series reads as a visual changelog. Every phase is gated on `make bdd` green and
`make tape` regenerating.

This paragraph used to claim `make lint` clean as a third gate. It is not one and
was not one before the artifact pass — see §7, *Still open*. Recording a gate
that is not being enforced is the same failure as recording a mockup and calling
it a frame, so the claim is removed rather than aspirationally restated.

### P0 — Skeleton and gates
**Shipped.** Module, `apphelpers`, `config.Read`, `main.go`, `.golangci.yml` with `default:
all`, `.goreleaser.yaml`, CI. TUI is a stub that shows the banner and quits.

*Acceptance:* Build failure runs Cleanup then exits 1. Cleanup failure logs and
exits 1. `PRISM_CONFIG` unset falls back to the default path. `config` is
unreachable outside `cmd/prism` and `internal/app` (depguard).

**Artifact:** `p0-banner-dark.png` / `p0-banner-light.png` — the spectral banner,
both palettes. ✅ *recorded*

The tape had been photographing the wrong screen. The splash retires itself
after `tui.splashFor`, which is 1200ms, and the tape waited 1800ms before
capturing — so the committed frame was the dashboard, filed under the name of
the banner. Nothing about it looked wrong, because a dashboard is a plausible
thing for a file called `p0-banner` to contain.

### P1 — Scrape, store, first chart ✅ shipped
Streaming expfmt decode (one family at a time, body never buffered whole),
stamped ring buffer, braille chart, series list, cardinality panel, window
selector. Counters converted to per-second rates. `j`/`k` already step the
window; P2 adds the spring easing and panning.

*Acceptance:* Given an endpoint serving a counter, when two scrapes 15s apart
differ by 300, the chart shows 20/s. Malformed exposition increments an error
counter without killing the scraper.

**Artifacts:** `p1-dashboard-dark.png`, `p1-dashboard-light.png`,
`p1-series-selected.png`. ✅ *recorded*

The frames apply a filter, which belongs to P3, and the reason is a finding in
its own right: **prism opens on the alphabetically first family it holds**, and
against Envoy that is `envoy_cluster_assignment_stale` — a gauge that was zero
for the entire recording. The phase's own claim is that a counter is charted as
a per-second rate, and a flat line at the floor cannot show it either way.
Reaching a counter by selection instead is not an option: `↑`/`↓` walk the
series list in target order, and the busy families sort several hundred rows
below the circuit breakers. Opening on something an operator would have chosen
is left as a change to consider, not made here.

### P2 — Time window: zoom and pan ✅ shipped
`j`/`k` zoom, `h`/`l` pan, `L` reattaches to live, `g` jumps to the oldest
bucket held, `?` slides up the key reference. Window and pan live in
`internal/timeline`, deliberately outside the Bubble Tea model so the clamping
edge cases can be driven from a test without a terminal.

*Acceptance:* met — see `features/window.feature` and
`internal/timeline/timeline_test.go`. Zooming out clamps at the buffer horizon
and the status bar says why. Panning off live shows how far behind the window
sits, in the same units as the axis.

**Artifacts:** `p2-window-5m.png`, `p2-zoom-mid.png` (the spring caught in
flight), `p2-zoom-30s.png`, `p2-horizon.png`, `p2-paused.png`, `p2-help.png`.
✅ *recorded*

The backlog flagged a disagreement here: the tape screenshotted
`p2-window-5m.png`, which had never been committed, and this list claimed
`p2-zoom-mid.png`, which no tape step produced. The recording settled it in
favour of keeping both. **The spring is catchable.** `p2-zoom-mid.png` is a
screenshot taken with no sleep after `j`, and it lands with the selector
already reading `[ 1m ]` while the axis still spans 3m49s — the window
mid-flight between 5m and 1m, which is exactly what the phase claims happens
and what no still had ever shown.

`p2-paused.png` needed a correction of its own. The first recording panned with
`h` while the window was at the buffer horizon, where it already spans every
bucket the ring holds and there is nothing to the left to pan into, so `h` is
correctly a no-op — and the frame came out identical to `p2-horizon.png` above
it. The tape now narrows before it pans.

### P3 — Filter and pivot ✅ shipped
`/` opens a filter over label matchers, compiled once on enter and never during
a frame. `p` cycles the pivot: the family separates into one summed line per
distinct label value, coloured across the spectrum — the prism moment. The lower
panel becomes the legend and grows to fit every line, because a legend listing
three of five lines leaves two colours on the chart unexplained.

Syntax: `key=value`, `key!=value`, `key=~regex`, `key!~regex`, `__name__` for
the family, and a bare word as a substring match on the family name. Terms are
combined with AND. Regexes are fully anchored, as Prometheus anchors its own.

*Acceptance:* met — see `features/filter.feature`,
`internal/filter/filter_test.go` and `internal/series/lines_test.go`. A filter
matching nothing names itself on the chart rather than going blank; a filter
that will not compile keeps the field open with the reason; the pivot tail
collapses into `other` with its member count shown.

**Artifacts:** `p3-filter.png`, `p3-pivot.png`, `p3-pivot-light.png`,
`p3-empty.png`. ✅ *recorded*

The pivot is `envoy_listener_worker_downstream_cx_total` by `envoy_worker_id`,
which is the only thing this Envoy exposes with enough distinct values to prove
the claim about the tail: ten workers against a plot limit of five, so `other`
appears with six members rather than being asserted in prose. A pivot by cluster
would have shown two lines and collapsed nothing.

### P4 — Cardinality and sampling ✅ shipped
Per-family cardinality with a growth sparkline, a `!` badge past the configured
threshold, a `~` when the count is an estimate, and per-family sampling that
engages when a family outgrows its budget. The ratio appears both against the
family and in the status bar.

Two pieces of machinery, both new packages:

- **`internal/sketch`** counts distinct series in bounded memory. Exact while a
  family is small — most are, and for those an estimate is a worse answer than
  the truth — then K-Minimum-Values past the limit. KMV rather than HyperLogLog
  because it is a heap of integers instead of a register array with bias
  correction tables, and at these cardinalities the accuracy is the same:
  measured 0.03% to 3.7% error from 10k to 500k against the exact hash sequence
  the tests use.
- **`internal/sample`** decides which series are stored. *Not* reservoir
  sampling, despite what this plan originally said. A reservoir re-rolls its
  membership as items arrive, so a series would drop out of the chart and
  reappear between scrapes, and a flickering line is worse than an absent one.
  Admission is instead a pure function of series identity — `mix(id) mod 2^k` —
  so a series admitted at a ratio stays admitted, and tightening only ever
  removes. Ratios halve, giving the 1:2, 1:4, 1:8 reported on screen.

The crude `samples[:MaxSeries]` truncation from P1 is gone. It dropped whole
families based on nothing but the order the target happened to write them in.

*Acceptance:* met — see `features/cardinality.feature`. Storing 100,000 series
into a budget of 100 keeps 100, while the reported cardinality tracks the true
figure to within 3.5%.

**Artifact:** `p4-cardinality.png`. ✅ *recorded*

Recorded against `deploy/prism-cardinality.yaml` rather than the demo config.
The phase is about what happens when a family outgrows its budget and the Envoy
in `deploy/` never does — its widest family holds ten series against a budget of
five hundred — so the thresholds come down to meet the target rather than the
target being inflated to meet them. At a budget and warning level of eight, the
per-worker families cross both at once and the frame carries `10 1:2 !` on one
row with `sampled 1:2` in the status bar.

**One claim in this phase cannot be photographed against this target.** The `~`
estimate marker appears only once a family passes `sketch.DefaultExactLimit`,
which is 4096 and is not configurable; nothing Envoy exposes here comes within
two orders of magnitude. It stays covered by `internal/sketch`'s tests, and this
is the one place the artifact series is knowingly incomplete.

### P5 — HTTP streaming ✅ shipped
A broker publishes each scrape as a server-sent event on `/stream`; a client
follows another prism and feeds the frames into its own store. Ten people can
watch one endpoint without scraping it ten times.

The collector no longer knows what it is feeding: it writes to a
`series.Sink`, and `series.Fanout` sends each scrape to the local store first
and the broker second, so a wedged network consumer can never delay the data on
screen.

**Backpressure is a slot, not a queue.** Each subscriber holds exactly one
pending frame; a frame arriving before the last one was sent replaces it, and
the count of skipped frames rides along on the event. A queue here would turn a
slow consumer into unbounded memory on the producer, which is the precise
failure prism exists to help diagnose. Coalescing is safe because the store
derives rates from the timestamps carried in each frame rather than from
adjacent samples — dropping an intermediate frame costs resolution, not
correctness.

Frames carry samples, not rendered state, so a follower rebuilds exactly the
store it would have built by scraping the target itself. Shipping snapshots
would have put the ring buffer's layout into the protocol and frozen it there.

*Acceptance:* met — see `features/stream.feature`. An upstream that goes away is
reported as `⟳ reconnecting 4s` with exponential backoff and jitter, and the
history already in the ring is left alone: what was observed happened, and
losing the link does not unhappen it. The gap renders as a gap.

**Artifacts:** `p5-remote.png`, `p5-reconnecting.png`. ✅ *recorded*

The tape runs two prisms: `deploy/prism-publisher.yaml` scrapes Envoy and serves
`/stream`, `deploy/prism-follower.yaml` follows it and never touches Envoy at
all. The follower is the one on screen, and its header names `127.0.0.1:9099`
rather than the admin port, which is the phase's claim made visible.

Recording it needed two mechanics worth keeping. The publisher is a Bubble Tea
program and exits with *error opening TTY* if it is simply backgrounded, so it
runs under `script(1)`, which allocates a pty and throws the frames away. And
the kill that takes the upstream away has to be scheduled before the recording
starts, because once the follower is running there is no shell left to type
into. Killing by port needs `-sTCP:LISTEN`: the follower holds a connection *to*
9099, so a bare `lsof -ti tcp:9099` names both ends, and the first attempt at
this killed the process being recorded.

**The frame this phase exists for was unobtainable until a bug was fixed.** See
"What re-recording the series found" below.

### P6 — Theme ✅ shipped
`--dark` / `--light` / auto via `BackgroundColorMsg.IsDark()`, plus a `d` toggle.
Palettes are data, resolved once per theme change. The flags had never been
exercised by a tape until this pass; every earlier recording took whatever auto
detection returned.

*Acceptance:* met. Every color goes through `theme.Palette`, and the grep for
raw hex outside `internal/theme` is a real CI step
(`.github/workflows/ci.yml`) — verified, not assumed.

**Artifact:** `p6-light.png` beside `p6-dark.png`. ✅ *recorded* — this was the
one entry in §2 asserting an artifact that had never existed.

It is a pair of tapes rather than one, and why is worth recording. prism does
ask for a background: `View.BackgroundColor` is set from the palette on every
render, and the palette carries one for each side. What the recordings show is
that the request does not survive the trip through `ttyd` — pressing `d` changes
the foreground colours and leaves the terminal's background where it was. So a
light frame reached by toggling on a dark terminal is light-mode text on black,
which is neither palette and is close to illegible. The old `p7-light.png` was
exactly that, committed and linked for the whole of P7.

Each half now records on the terminal theme it is meant for, Mocha and Latte, so
the only deliberate difference between the two frames is prism's palette.
Whether `ttyd` is declining the request or something above it is not settled
here; it does not need to be for the recording to be honest, and it is the one
loose thread this pass leaves.

### P7 — Envoy end to end ✅ shipped
`deploy/docker-compose.yaml` stands up Envoy behind three fortio generators at
different rates, over two clusters (`api`, `web`) and a route that answers 404
directly, so the response-code families hold more than one value. `make
envoy-up` does not return until Envoy is serving stats that describe traffic —
returning at "container started" would hand the suite an endpoint whose
histograms are declared and empty.

The suite runs twice against the same bytes. `make bdd` replays
`features/testdata/envoy-stats.txt`, recorded from that stack, so the gate needs
no container runtime; `make bdd-envoy` runs the identical scenarios against the
live admin port, which is how we find out the recording has aged. A fixture is a
moment in one Envoy version's life, and nothing else in the suite would notice
the moment passing.

*Acceptance:* met — see `features/envoy.feature`. Against Envoy 1.31 the
endpoint declares **306 families and prism holds all 306**, as counted by a
second, deliberately naive parser in the step definitions rather than by the
decoder under test. Twelve histogram families arrive as twelve entries, not
thirty-six.

**Histogram grouping needed no code.** The exposition splits a histogram across
`_bucket`/`_sum`/`_count`, but it also declares the family in a TYPE header, and
`expfmt` uses that header to reassemble the parts before yielding anything — so
the three suffixes never reach prism as names. The `scrape.Family` helper this
plan implied is gone: nothing called it, and the scenario grading it only ever
checked the helper against itself. Folding suffixes by spelling would have been
worse than nothing, because Envoy ships two genuine counters whose names end in
`_count`, and a suffix table would have silently merged them into families that
do not exist.

The strongest scenario here is *Envoy is not special-cased*: the same exposition
is scraped twice, once with every `envoy_` rewritten to `wavelength_`, and the
two runs must produce identical families, kinds and cardinalities. Any branch on
the vendor's name — a suffix table, a family list, a prefix strip — makes the
renamed run diverge, and no amount of reading the code proves its absence the
way that does.

**What running it against the real thing cost.** One bug, and it was the kind
only a real target produces. The axis gutter is six columns and its labels are
right aligned, so a wider label does not spill into the plot where it would be
noticed — `rput` walks off the left edge and drops the leading characters.
Envoy's counters put 903890 on an axis, `%.2fk` made that `903.89k`, and seven
characters in six columns drew **`03.89k`**: a plausible number, three orders of
magnitude wrong, with nothing about it to suggest anything had gone amiss. The
mocked-up frames in `design/` could never have shown this, because the mock
picked its own numbers.

`chart.Format` is now bounded by `chart.MaxFormatWidth`, giving up decimal
places as the leading digits need the room rather than fixing them, and
`internal/chart/scale_test.go` pins the bound. The same test found a second
case unaided: the `G` branch had no ceiling, so `math.MaxFloat64` formatted to a
301-character label.

**Artifact:** `p7-walkthrough.gif` — the whole tour, plus the stills bracketing
it (`p7-arrive`, `p7-dashboard`, `p7-histogram`, `p7-filtered`, `p7-pivot`,
`p7-zoomed`, `p7-panned`, `p7-light`, `p7-help`). ✅ *recorded*

`p7-light.png` has moved into its own tape. It was the one frame in this phase
that was real output of the wrong thing: the tour reached it by pressing `d` on
a dark terminal, which produces light-mode text on a dark background. See P6.

---

### What re-recording the series found

The backlog argued that re-recording was the cheapest way to find out what else
the mockups had been agreeing with, on the strength of one bug. It found six,
and four of them were the same bug the axis truncation was: **a plausible string
or number that no code meant to produce, sitting where nothing would question
it.**

1. **A counter's first sample was charted as zero.** `state.observe` returned 0
   when there was no previous reading, and `record` stored that 0 as an
   observation. Every counter's line therefore rose out of the floor at the left
   edge of history — a vertical drop to zero, at the rates Envoy reports, that
   asserted an outage that never happened. It is in the committed
   `p7-pivot.png`. One reading is not a rate, and the honest answer is silence;
   the bucket is now left unstamped and renders as the gap it is. A counter
   *reset* still reports zero, which is a different case and a deliberate one.

2. **The header and the footer overwrote themselves.** The metric name runs left
   to right and the window strip is laid out from the right edge back, and
   neither knew about the other. On a narrow enough terminal they shared cells
   and the later writer won, character by character, producing
   `⌁ windowrk30si[ 1m ]` — fragments of `envoy_worker_id` with the marker and
   the `30s` step written through them. The footer did the same to the status
   line: `p pivot envoy_wor10/553 series`. This document had already noticed the
   symptom and filed it under §7 as *long names truncate into the window
   selector, cosmetic* — it was not truncation, because nothing truncated
   anything, and it was not cosmetic. Both rows now measure the right-hand side
   before drawing the left.

3. **Bounding the header raised a second question: which part gives way.** Drawn
   in order, the family name takes every column it wants and the pivot key — four
   times shorter, and the only thing on the row saying how the chart is split —
   is what disappears. The short parts now reserve their room first, so a cut
   name sits beside a legible `by envoy_worker_id (6)` rather than the reverse.
   This is the column-allocation decision §7 said was needed before any frame
   showing it could be called final.

4. **prism's own logs were printed across its dashboard.** `internal/logging`
   said in a comment that logs go to stderr to keep them clear of the UI. That
   was never true: stdout and stderr are the same terminal unless somebody
   redirects one, and a log line written into a full-screen render corrupts it
   from either. The P5 frame meant to show an upstream going away had the warning
   about the upstream going away printed over it — prism's account of the failure
   obscuring the failure. Logs now go to stderr only when stderr is not a
   terminal, so `prism 2>prism.log` keeps them and an interactive run does not
   lose a frame to them. Nothing is lost by the default: a scrape that is failing
   already shows as `● stalled`, a dropped upstream as `⟳ reconnecting`.

5. **The legend reported a rate of zero during a gap.** `SeriesView.Last` was 0
   when the newest bucket was empty, so the reconnecting frame read `api 0` and
   `web 0` beside a chart that correctly drew nothing there — the one place the
   reading is given as a number, and it was the only place asserting the traffic
   had stopped. `Last` is now this package's not-a-number for absent, which
   `chart.Format` already drew as `-`, and a pivoted total sums the members that
   have a value rather than being poisoned by the ones that do not.

6. **The header claimed to be waiting for a first scrape after forty-three of
   them.** With a filter matching nothing there is no selected series, and the
   fallback string said `waiting for first scra…` while the body of the same
   frame said `nothing matches envoy_worker_id=~99` and the footer counted the
   scrapes. A frame that contradicts itself is worse than either half of it.

Each has a test, and every one of those tests has been *watched failing without
its fix* — for the ones written after the fix rather than before it, by reverting
the fix, confirming the failure, and restoring. That step is not ceremony. A test
written alongside the code it checks has only ever agreed with it, which makes it
a claim about a bug rather than evidence of one, and this document has just spent
a section on the difference between those two things.

The layout tests sweep terminal widths rather than pinning one, because a single
width would have passed while the frames on disk showed the bug — the recordings
are narrower than the tape's pixel dimensions suggest. Reverting the header's
column allocation, for instance, drops the pivot key out of an 80-column header
entirely, which is the recorded `b…` frame reproduced in a test.

Three of the six were only reachable by *recording*: the log overprint and the
gap-legend need a real upstream to die, and the opening zero needs a real
counter's first scrape. No unit test was going to ask for them, because none of
them is a question you think to ask.

---

## 3. Decisions I made where the spec was ambiguous

Flagging these rather than silently picking:

1. **`Run` vs `Start`.** Your prose says `Run`/`Shutdown`; the pasted code has
   `Start`/`Cleanup`/`AddStartupFuncs`. I went with **`Run` + `Cleanup` +
   `AddStartupFuncs`/`AddCleanupFuncs`** — matching the main.go narrative and the
   plural adders you actually wrote.

2. **Goroutine leak in `Start`.** `errChan` is unbuffered. When `Start` returns
   early — on the first non-nil error or on `ctx.Done()` — every remaining
   startup goroutine blocks forever on its send, and the `defer`s in those
   functions never run. Fixed by buffering the channel to
   `len(startupFuncs)`. This matters here because the scraper and the SSE
   server both hold resources.

3. **`Cleanup`'s doc comment is wrong.** It says "runs each cleanup function in
   a separate goroutine"; the code runs them sequentially in reverse order.
   Sequential reverse is the *correct* behaviour for teardown (you want the
   server drained before the store closes), so I kept the code and rewrote the
   comment.

4. **depguard can't be main.go-only.** You want `config` reachable only from
   `main.go`, but you also want `config.Read` called in `Build` — which lives in
   `internal/app`. Those conflict. The rule allows `internal/config` in exactly
   two files: `cmd/prism/main.go` and `internal/app/build.go`, and denies it
   everywhere else. Say the word if you'd rather main did the read and passed
   the struct down; then the rule tightens to main.go alone.

5. **"streamed over http"** — read as both directions: the scrape response is
   parsed as a stream (never fully buffered), and prism *serves* SSE so a TUI can
   attach remotely. If you only meant ingest, P5 gets much smaller.

6. **"no persistence but temporary file storage"** — in-memory ring is the source
   of truth; cold buckets spill to `os.CreateTemp` files that are unlinked
   immediately after creation, so they vanish on exit even if we're killed.

7. **OTEL** — the telemetry config wires the SDK for prism's *own* traces and
   metrics (self-observability), entirely separate from the metrics it scrapes.

---

## 4. Physics

`harmonica` is already in the dependency tree via bubbles v2, so springs cost
nothing extra. Four places where they earn their keep:

Every constant below was chosen by integrating the spring and reading off the
settling time and the overshoot, not by feel:

| Spring | freq / damping | settles | overshoot |
|---|---|---|---|
| Zoom (log space) | 14 / 0.80 | ~0.72s | 2.3% under |
| Pan | 18 / 1.00 | ~0.7s | none (critical) |
| Y-axis rescale | 12 / 1.00 | ~0.97s | none (critical) |
| Help panel | 18 / 0.85 | ~0.6s | 1.5% |

- **Y-axis rescale.** The biggest ergonomic win. When a spike moves the axis
  maximum 400 → 1200, snapping makes the whole chart appear to teleport.
  Critically damped: overshoot on an axis would show a value crossing a
  threshold it never crossed.
- **Zoom, in log space.** Window widths are roughly a constant factor apart, so
  a linear interpolation from 1h to 30s spends almost the whole transition in
  the wide end and then snaps. Interpolating `log(seconds)` makes every step of
  the zoom feel the same size. `TestZoomIsGeometric` pins this: eight frames
  into a 1h → 15m zoom the drawn window is at 25m, where a linear
  interpolation would still be at 31.5m.
- **Pan.** Critically damped. Overshoot would scroll past the buckets the
  operator asked for and then walk back, which reads as a glitch.
- **Help panel.** Slides up from behind the footer, clipped by its own top edge
  so it appears to emerge rather than fade in.

A pivot is not animated. Splitting one line into five is a change of subject,
not a change of view, and easing between two different sets of numbers would
draw values that were never observed.

**No cursor glide.** The plan called for the `▸` to slide between rows. A cell
grid cannot draw a marker at a fractional row, and the tricks for faking it —
dimming both neighbours, half-block glyphs — read worse than an instant move.
The fourth spring went to the help panel instead.

Rules that keep it fast: springs run on a single 60fps ticker shared by all
animators; if no spring is in motion the ticker stops entirely and the app
becomes fully event-driven at zero CPU. Data is never interpolated — only
*presentation* values (scales, offsets, positions). A lying chart is worse than
an abrupt one.

---

## 5. Performance budget

- Frame render < 8ms at 200 series; Ultraviolet's diffing renderer only emits
  changed cells, so a static chart costs ~0 bytes of output.
- Braille cells are computed per bucket and cached; panning reuses them.
- Filters compile to a matcher struct once on `Enter`, never per frame.
- The TUI reads an immutable snapshot swapped in atomically by the collector.
- Status bar shows live `ms/frame` — the budget is visible, so regressions are
  obvious during the demo.

---

## 6. Note on the artifacts

**Every frame in `design/` is now VHS output**, recorded from prism built out of
this tree against the Envoy in `deploy/`. Everything on them — the 553 series,
the 26/s and 8.0/s cluster rates, the ten listener workers — is what Envoy
reported.

Until this pass, `p0` through `p5` were **rendered mockups**: produced in a
sandbox with no Go toolchain and a blocked module proxy, by a Python renderer
implementing the same braille 2×4 cell algorithm and the same palette as the Go
code. They were the design target, and they picked their own numbers. That is
the whole reason the six bugs above survived as long as they did — a mockup
never has to render a number it did not choose, and never has to render one at
a terminal width it did not choose either.

Regenerating: `make envoy-up && make tape`, on a machine with Go 1.25, VHS and
`ttyd`. `make tape` writes into `vhs/out/` only. Promoting those frames into
`design/` is `make artifacts`, a second and deliberate command.

**Why promotion is not folded into `make tape`.** A live chart never renders
twice the same, so a `tape` target that copied into `design/` would put a diff
on all thirty artifacts every time anybody re-recorded any one phase, and a
visual changelog that restates itself on every unrelated run is not a changelog.
The cost of the split is that the two can drift; the mitigation is that
`make artifacts` names its thirty files explicitly, so a frame that no tape
produces any more fails the target instead of quietly staying behind.

A caution for anyone changing a tape. **VHS does not capture at the instant
`Screenshot` is reached** — the capture lands a beat later, after whatever was
typed next has been processed. Every screenshot in these tapes is followed by a
sleep for that reason. Without it this series produced a `p3-pivot.png` with the
filter bar reopened over it and a `p3-empty.png` of the shell prompt, prism
having already quit. Neither announces itself: they are valid frames of the
wrong moment, which is the same failure mode as the mockups.

---

## 7. Follow-up — the artifact backlog

**Done.** Every phase now has a tape, every committed frame is output of the
program, and the six bugs the exercise turned up are listed under §2's P7 with a
test apiece. What is left open is at the bottom of this section.

### What the series looks like now

| | Tapes | Frames in `design/` |
|---|---|---|
| P0 | `p0-banner.tape`, `p0-banner-light.tape` | `p0-banner-{dark,light}.png` |
| P1 | `p1-dashboard.tape`, `p1-dashboard-light.tape` | `p1-dashboard-{dark,light}.png`, `p1-series-selected.png` |
| P2 | `p2-zoom.tape` | `p2-window-5m`, `p2-zoom-mid`, `p2-zoom-30s`, `p2-horizon`, `p2-paused`, `p2-help` |
| P3 | `p3-filter.tape`, `p3-pivot-light.tape` | `p3-filter`, `p3-pivot`, `p3-pivot-light`, `p3-empty` |
| P4 | `p4-cardinality.tape` | `p4-cardinality.png` |
| P5 | `p5-stream.tape` | `p5-remote.png`, `p5-reconnecting.png` |
| P6 | `p6-theme-dark.tape`, `p6-theme-light.tape` | `p6-dark.png`, `p6-light.png` |
| P7 | `p7-walkthrough.tape`, `p7-light.tape` | gif + 9 stills |

Light frames are their own tapes throughout, for the reason set out in P6.

Three configuration files were added to make phases recordable that were not:
`deploy/prism-cardinality.yaml` (thresholds Envoy can actually cross),
`deploy/prism-publisher.yaml` and `deploy/prism-follower.yaml` (the two halves of
P5). Each says in its own header why it exists.

### Was it worth doing

The backlog's argument was that a recorded frame is a test with an operator as
the assertion, and that re-recording was the cheapest way to find out what the
mockups had been agreeing with. Six bugs, of which three were unreachable by any
unit test anyone would have thought to write, and one of which made the frame
P5 exists for impossible to capture at all.

The cost was that four of the six had to be fixed before their own artifact
could be recorded honestly, so this pass changed code as well as pictures. That
was the anticipated shape of the work — §7 said anything a recording contradicts
is either a bug or a claim that needs rewriting — but it is worth being explicit
about how it split. Every claim in §2 about what prism *does* held up. The claims
that did not were about the artifacts themselves — which frames existed and what
they showed — and about the gates, one of which is still failing and is listed
below. The pictures were wrong, and underneath four of them so was the program.

### Known snags, so they are not rediscovered

- Tapes need `prism` on `PATH`. `make tape` puts `./bin` first, so a tape
  records the working tree rather than an installed build.
- Use `deploy/prism-demo.yaml`, not `config.example.yaml`. The 15s default fills
  four buckets inside a recording; the demo config's 2s fills the window. This
  was the actual defect in the old P0–P2 tapes.
- **`Screenshot` captures a beat late.** Follow every one with a sleep, or it
  records the state after the next keystrokes. See §6.
- A screenshot aimed at a transient state has to account for that lag in the
  other direction: `p0-banner` fires at 500ms against a splash that retires at
  1200ms, and `p2-zoom-mid` takes no sleep at all against a spring that settles
  in 720ms.
- **Screenshots are written after the gif is encoded, not during the recording.**
  Checking a run's progress by file mtime therefore shows the previous run's
  stills for as long as ffmpeg is busy, which on a long tape is minutes. This
  looked exactly like P5 silently failing to write its two frames, and was not.
- The long accumulation in `p2-zoom.tape` sits inside `Hide`. VHS records no
  frames while hidden, so two and a half minutes of a chart filling costs the
  gif nothing; left visible it was the most expensive thing in `make tape` and
  all of it was the part with nothing to demonstrate.
- `vhs` and `ttyd` are Homebrew installs; neither is in the repo's toolchain and
  CI does not run `make tape`. The frames are therefore not gated by CI and can
  drift from the program without anything failing.

### Still open

1. **`make lint` is not clean, and §2 used to say every phase was gated on it being
   clean.** That claim is false, and was false before this pass:
   `golangci-lint run ./...` reports **124 findings** across the tree. The
   paragraph in §2 asserting the gate has been removed rather than restated.

   The count is what it was before this pass, give or take one either way: the
   work here fixed one finding and introduced one. Some of the findings do sit
   in files this pass edited, but at lines and patterns that predate it —
   `funcorder` on method ordering, `goconst` on strings repeated in tests, the
   `f *frame` parameter name that every draw function in the package uses.

   None of the 124 is a correctness finding. The new tests and the new logging
   code were brought to zero findings of their own, which is the standard the
   rest of the tree is not held to. Either the gate or the linter configuration
   needs to change, and that is a decision rather than a chore: `linters.default:
   all` means every new linter release adds findings to a tree nobody has agreed
   to keep clean. Left alone here because silencing a hundred style findings
   inside an artifact pass would bury the six real ones under them.
2. **prism opens on the alphabetically first family**, which against Envoy is a
   gauge that is flat at zero. See P1. Opening on something with data in it is a
   product decision, not an artifact one.
3. **Whether `ttyd` drops prism's background request, or something above it
   does.** See P6. It is the reason light frames need their own terminal, and
   nobody has yet established where the request is lost.
4. **The `~` cardinality estimate cannot be photographed** against this target,
   because `sketch.DefaultExactLimit` is 4096, is not configurable, and no Envoy
   family here approaches it. See P4.
