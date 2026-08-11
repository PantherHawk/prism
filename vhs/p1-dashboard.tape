# Phase 1 - scrape, store, first chart. Points the TUI at the Envoy in deploy/
# and lets the ring buffer fill before the frame is captured.
#
# This used config.example.yaml until the artifact backlog was worked through,
# and that was the bug in the tape rather than in prism: the shipped default is
# a 15s scrape interval, which is right for a dashboard somebody leaves open
# and wrong for a recording. Six seconds of it bought a single bucket, so the
# committed frame was a mockup of a chart that the tape could not have drawn.
# deploy/prism-demo.yaml scrapes every 2s and fills the window inside a take.
#
# The dark and light frames are separate tapes for the reason set out in
# p6-theme-dark.tape: prism asks for a background but the request does not
# survive the recording, so a light palette has to be recorded on a light
# terminal to be worth looking at.
Output vhs/out/p1-dashboard.gif

Require prism

Set Shell        bash
Set FontFamily   "JetBrains Mono"
Set FontSize     18
Set Width        1600
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
Sleep 6s

# The filter belongs to phase 3 and is used here for one reason: prism opens on
# the alphabetically first family it has, and against Envoy that is
# envoy_cluster_assignment_stale - a gauge that is zero now and was zero for the
# whole recording. The phase's claim is that a counter is charted as a per-second
# rate, and a frame of a flat line at the floor cannot show it either way.
#
# Reaching a counter by selection instead was the alternative and is not one:
# the list is every series in target order, and the busy families sort several
# hundred rows below the circuit breakers.
Type "/"
Sleep 800ms
Type "upstream_rq_total"
Sleep 1200ms
Enter
Sleep 1500ms

# Let the buffer accumulate a window worth charting, then narrow onto the span
# this recording has actually observed.
Sleep 55s
Type "j"
Sleep 900ms
Screenshot vhs/out/p1-dashboard-dark.png
Sleep 2s

# Select a series, which is what the pivot in phase 3 builds on.
Down Down
Sleep 1s
Screenshot vhs/out/p1-series-selected.png
Sleep 2s

Type "q"
Sleep 400ms
