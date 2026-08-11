# Phase 3 - the pivot in the light palette.
#
# The companion still to p3-pivot.png. It is a separate tape for the reason
# set out at length in p6-theme-dark.tape: `Set Theme` configures the terminal
# before the tape starts, and prism's own background request does not survive
# the recording, so a light frame has to be recorded on a light terminal rather
# than reached by pressing `d` on a dark one.
#
# The spectrum is the thing under test here. Six lines including the collapsed
# tail is the most colour prism ever puts on screen at once, which makes this
# the frame where a palette that does not separate would show it.
Output vhs/out/p3-pivot-light.gif

Require prism

Set Shell        bash
Set FontFamily   "JetBrains Mono"
Set FontSize     18
Set Width        1600
Set Height       900
Set Padding      24
Set Margin       36
Set MarginFill   "#e6e9ef"
Set BorderRadius 12
Set Theme        "Catppuccin Latte"
Set TypingSpeed  55ms

Hide
Type "export PRISM_CONFIG=$PWD/deploy/prism-demo.yaml" Enter
Type "clear" Enter
Show

Type "prism --light"
Sleep 400ms
Enter
Sleep 1500ms
Sleep 6s

Type "/"
Sleep 800ms
Type "worker_downstream_cx_total"
Sleep 1200ms
Enter
Sleep 1500ms

Sleep 60s
Type "j"
Sleep 900ms

Type "p"
Sleep 1200ms
Type "p"
Sleep 1500ms
Screenshot vhs/out/p3-pivot-light.png
Sleep 2s

Type "q"
Sleep 400ms
