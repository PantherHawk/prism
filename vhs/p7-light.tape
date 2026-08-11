# Phase 7 - the walkthrough's view in the light palette.
#
# Split out of p7-walkthrough.tape when the artifact backlog was worked
# through. It reaches the same place the tour reaches - the counter filtered
# down and split by cluster - on a light terminal, so that the frame is a
# light dashboard rather than a light-mode palette drawn on black.
#
# See p6-theme-dark.tape for why that distinction needs a whole second tape.
Output vhs/out/p7-light.gif

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
Sleep 600ms
Type "upstream_rq_total"
Sleep 800ms
Enter
Sleep 1500ms
Type "p"
Sleep 1200ms

Sleep 55s
Type "j"
Sleep 900ms
Screenshot vhs/out/p7-light.png
Sleep 2s

Type "q"
Sleep 400ms
