# Phase 0 - the splash screen. prism starts, resolves its theme from the
# terminal background, draws the PROMDATE wordmark beside the host facts, and
# hands over to the dashboard.
#
# The sleep before the screenshot is the whole difficulty. The splash retires
# itself after tui.splashFor, which is 1200ms, and this tape used to wait 1800ms
# before capturing - so it photographed the dashboard and called it the banner.
# Nothing about the frame said so, because a dashboard is a plausible thing for
# a tape named p0-banner to contain.
#
# The window is therefore narrow: after prism has started and drawn, and before
# 1200ms have passed. It is aimed early because VHS captures a beat after the
# command is reached rather than at it.
#
# The window size matters here in a way it does not for the other tapes, and it
# is the second way this tape has photographed the wrong screen. At 1400x760 it
# measured 74 columns, banner.Render fell back to the compact tier, and the
# frame was the one-line title rather than the wordmark - a plausible-looking
# splash that happened to omit the artifact. Two thresholds have to be cleared:
#
#   - banner.fullWidth/fullHeight, 92 x 26; and
#   - the composed block itself, which Render measures. That is the binding one
#     here. The 55-column logo, the 4-column gutter and an info column as wide
#     as its longest fact come to 106 columns against the demo config, whose
#     scrape URL is http://localhost:9901/stats/prometheus.
#
# 2100x900 measures 115 x 35, which clears both. Note that the character cell is
# ~17.2px rather than the ~10.8px JetBrains Mono would give: no machine that has
# recorded these tapes has the font installed, so VHS falls back to a much wider
# face. Every artifact in design/ shows it. Size against what the tapes actually
# render at, not against the font they name.
Output vhs/out/p0-banner.gif

Require prism

Set Shell        bash
Set FontFamily   "JetBrains Mono"
Set FontSize     18
Set Width        2100
Set Height       900
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

Screenshot vhs/out/p0-banner-dark.png
Sleep 1s

Type "q"
Sleep 400ms
