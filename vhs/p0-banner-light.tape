# Phase 0 - the splash screen. prism starts, resolves its theme from the
# terminal background, draws the wordmark, and hands over to the dashboard.
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
Output vhs/out/p0-banner-light.gif

Require prism

Set Shell        bash
Set FontFamily   "JetBrains Mono"
Set FontSize     18
Set Width        1400
Set Height       760
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
Sleep 500ms

Screenshot vhs/out/p0-banner-light.png
Sleep 1s

Type "q"
Sleep 400ms
