# Phase 0 - the splash screen in the light palette.
#
# The companion to p0-banner.tape. Separate because `Set Theme` is a tape-level
# setting and prism's own background request does not survive the recording; see
# p6-theme-dark.tape for the whole argument.
#
# The capture window and the 2100x900 sizing are both p0-banner.tape's, for the
# reasons it sets out: the splash retires itself at tui.splashFor, and anything
# narrower than the composed full tier photographs the compact one instead.
Output vhs/out/p0-banner-light.gif

Require prism

Set Shell        bash
Set FontFamily   "JetBrains Mono"
Set FontSize     18
Set Width        2100
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
Sleep 500ms

Screenshot vhs/out/p0-banner-light.png
Sleep 1s

Type "q"
Sleep 400ms
