# Phase 6 - the theme, dark half.
#
# This phase is a pair, and the pair is the artifact: p6-dark.png and
# p6-light.png are only worth anything side by side. It is two tapes rather
# than one, for a reason worth stating precisely.
#
# prism does ask for a background: View.BackgroundColor is set from
# theme.Palette on every render, and the palette carries one for each side.
# What can be observed from the recordings is that the request does not survive
# the trip through ttyd - toggling with `d` changes the foreground colours and
# leaves the terminal's own background where it was. The old p7-light.png is
# that outcome committed as an artifact: light-mode text on a dark background,
# which is neither palette and is barely legible.
#
# Whether that is ttyd declining the request or something above it is not
# settled here, and it does not need to be for the recording to be honest. Each
# half of the pair runs on the terminal theme it is meant for - Mocha here,
# Latte in p6-theme-light.tape, the same Catppuccin family either side, so the
# only deliberate difference between the two frames is prism's palette.
#
# `Set Theme` is what forces the split into two files either way: it configures
# the terminal before the first keystroke and cannot be changed partway through.
#
# The phase's flags are what starts prism - `--dark` here - because until now
# no tape has exercised them; every earlier recording took whatever auto
# detection returned. The `d` toggle still appears in the gif, pressed after
# the still is captured and pressed again to come back, so the sequence the
# phase describes is on record without the screenshot depending on it.
#
# The view is pivoted rather than plain. A palette that separates a family
# across the spectrum cannot be judged from a chart with one line in it.
Output vhs/out/p6-theme-dark.gif

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

Type "prism --dark"
Sleep 400ms
Enter
Sleep 1500ms

# Past the banner and into the dashboard.
Sleep 6s

# One family, split by cluster, so the spectrum has something to spread across.
Type "/"
Sleep 600ms
Type "upstream_rq_total"
Sleep 800ms
Enter
Sleep 1500ms
Type "p"
Sleep 1200ms

# Let the ring fill, then narrow onto the span the recording has actually
# observed. prism opens on a 5m window and a tape is not five minutes long, so
# the default would spend four fifths of the frame proving there is no data
# yet - which is honest, and useless for judging a palette.
Sleep 55s
Type "j"
Sleep 900ms
Screenshot vhs/out/p6-dark.png
Sleep 2s

# d switches the palette and stops following the terminal. On a dark terminal
# this is the half-lit state described above - it belongs in the gif, which is
# a record of what the key does, and not in the still, which is a record of
# what the palette looks like.
Type "d"
Sleep 1500ms
Type "d"
Sleep 1500ms

Type "q"
Sleep 400ms
