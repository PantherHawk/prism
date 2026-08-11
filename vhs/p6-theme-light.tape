# Phase 6 - the theme, light half.
#
# The companion to p6-theme-dark.tape; read the reasoning there. The only
# differences are the terminal theme, the flag prism is started with, and the
# margin fill, which follows the terminal so the frame is not a light panel
# floating in a black surround.
#
# Catppuccin Latte is Mocha's light counterpart, so the two frames differ by
# the palette under test and not by the terminal it happens to be sitting in.
Output vhs/out/p6-theme-light.gif

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
Screenshot vhs/out/p6-light.png
Sleep 2s

Type "d"
Sleep 1500ms
Type "d"
Sleep 1500ms

Type "q"
Sleep 400ms
