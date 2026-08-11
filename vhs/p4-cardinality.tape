# Phase 4 - cardinality and sampling.
#
# Run against deploy/prism-cardinality.yaml rather than the demo config; that
# file explains why at length. The short version is that the phase is about
# what happens when a family outgrows its budget, and against a five-hundred
# series budget nothing here ever does.
#
# What the frame has to show is the cardinality panel doing three jobs at once
# on the same row: the count, the `!` marking it past the configured threshold,
# and the `1:2` saying the chart above is drawn from half the family. Those are
# three different claims and a bare number would wear all three.
#
# The long accumulation is not padding. Each family's growth sparkline is drawn
# from its history, so a frame taken early would show the badge and the ratio
# against a flat line and prove nothing about growth being tracked at all.
Output vhs/out/p4-cardinality.gif

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
Type "export PRISM_CONFIG=$PWD/deploy/prism-cardinality.yaml" Enter
Type "clear" Enter
Show

Type "prism"
Sleep 400ms
Enter
Sleep 1500ms
Sleep 6s

# The panel ranks every family by cardinality whatever is charted above it, so
# the filter costs nothing here and buys the frame a chart with traffic in it.
# Without one prism opens on whichever family sorts first, which on this target
# is a flat zero, and the phase's own claim - that the chart is drawn from a
# sampled subset - would have been made over a line at the floor.
#
# This family is one of the ten-series ones the budget bites on, so the `1:2`
# in the panel and the five lines on the chart are the same fact stated twice.
Type "/"
Sleep 800ms
Type "worker_downstream_cx_total"
Sleep 1200ms
Enter
Sleep 1500ms

Sleep 60s
Type "j"
Sleep 900ms
Screenshot vhs/out/p4-cardinality.png
Sleep 2s

Type "q"
Sleep 400ms
