# Phase 3 - filter and pivot.
#
# The phase's three stills in one pass: a filter narrowing five hundred series
# to ten, the pivot separating those ten across the spectrum, and a filter that
# matches nothing.
#
# The family is envoy_listener_worker_downstream_cx_total, chosen over the
# cluster families the other tapes use because it is the only thing this Envoy
# exposes with enough distinct values to prove the claim about the tail. It
# carries one series per listener worker and there are ten workers, against a
# plot limit of five - so the pivot has to collapse the remainder, and the
# `other` line with five members is on screen rather than being asserted in
# prose. A pivot by cluster would have shown two lines and collapsed nothing.
#
# All ten workers carry real and roughly equal traffic, which is what makes the
# collapse legible: `other` sits at about five times a single worker's rate
# because it is five workers, and that relationship is visible in the frame.
#
# Every Screenshot is followed by a sleep, and they are load-bearing. VHS does
# not capture at the instant the command is reached - the capture lands a beat
# later, after whatever was typed next has already been processed. Without the
# sleep this tape wrote a p3-pivot.png with the filter bar reopened on top of
# it and a p3-empty.png of the shell prompt, prism having already quit. Neither
# failure announces itself: they are valid frames of the wrong moment.
Output vhs/out/p3-filter.gif

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
Sleep 1500ms
Sleep 6s

# A bare word is a substring match on the family name - the shortest path from
# three hundred families to the one you meant.
Type "/"
Sleep 800ms
Type "worker_downstream_cx_total"
Sleep 1200ms
Enter
Sleep 1500ms

# Fill the window before the still, then narrow onto what was observed. The
# filter is compiled once, on enter, so none of this accumulation is paying to
# re-parse it per frame.
Sleep 60s
Type "j"
Sleep 900ms
Screenshot vhs/out/p3-filter.png
Sleep 2s

# The prism moment. p cycles the pivot key: once onto the listener address,
# which every one of these series shares and which therefore separates nothing,
# and again onto the worker id, which separates all ten.
Type "p"
Sleep 1200ms
Type "p"
Sleep 1500ms
Screenshot vhs/out/p3-pivot.png
Sleep 2s

# A filter that compiles and matches nothing. The chart names the filter rather
# than going blank, because an empty chart and a wrong query look identical
# otherwise.
Type "/"
Sleep 600ms
Ctrl+U
Sleep 400ms
Type "envoy_worker_id=~99"
Sleep 1000ms
Enter
Sleep 1500ms
Screenshot vhs/out/p3-empty.png
Sleep 2s

Type "q"
Sleep 400ms
