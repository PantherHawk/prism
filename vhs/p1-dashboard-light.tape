# Phase 1 - the dashboard in the light palette.
#
# The companion to p1-dashboard.tape. Separate because `Set Theme` is a
# tape-level setting and prism's own background request does not survive the
# recording; see p6-theme-dark.tape for the whole argument.
Output vhs/out/p1-dashboard-light.gif

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
Sleep 6s

# See p1-dashboard.tape for why a filter is applied in phase 1's tape.
Type "/"
Sleep 800ms
Type "upstream_rq_total"
Sleep 1200ms
Enter
Sleep 1500ms

Sleep 55s
Type "j"
Sleep 900ms
Screenshot vhs/out/p1-dashboard-light.png
Sleep 2s

Type "q"
Sleep 400ms
