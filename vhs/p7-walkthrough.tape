# Phase 7 - the whole tour, against the Envoy in deploy/.
#
# Every earlier tape demonstrates one mechanism against whatever the machine
# happened to be serving. This one runs the sequence an operator actually
# performs, on the target prism was built for: arrive, find the family among
# three hundred, split it by cluster, then narrow and pan the window.
#
# Bring the stack up first:
#
#   make envoy-up
#   make tape
#
# It uses deploy/prism-demo.yaml rather than config.example.yaml for one
# reason: a 2s scrape interval fills the window inside the length of a
# recording, where the 15s default would buy four buckets and a chart that is
# mostly axis.
#
# The filter and pivot are applied early, before the history exists, so that the
# long accumulation happens with the two cluster lines already on screen. The
# gif is the artifact; the stills are the moments worth linking to on their own,
# each captured after the motion has settled - about 0.7s for the window
# springs, about 1s for the vertical scale.
Output vhs/out/p7-walkthrough.gif

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

# The banner, then the dashboard it hands over to.
Type "prism"
Sleep 400ms
Enter
Sleep 1500ms
Screenshot vhs/out/p7-arrive.png

Sleep 6s
Screenshot vhs/out/p7-dashboard.png

# Envoy exposes three hundred families. The filter is how you get to one: a bare
# word is a substring of the family name.
#
# This one is a histogram, and it is the phase's claim made visible: the
# exposition spreads it over _bucket, _sum and _count lines, and it arrives as a
# single entry charted in observations. Three entries here would mean the
# decoder never put it back together.
Type "/"
Sleep 600ms
Type "upstream_rq_time"
Sleep 800ms
Enter
Sleep 1500ms
Screenshot vhs/out/p7-histogram.png

# Now the counter, which is what the pivot has something to say about.
Type "/"
Sleep 400ms
Ctrl+U
Sleep 300ms
Type "upstream_rq_total"
Sleep 800ms
Enter
Sleep 1500ms
Screenshot vhs/out/p7-filtered.png

# The prism moment. One family separates into one line per cluster, coloured
# across the spectrum, and the legend names every one of them.
Type "p"
Sleep 1200ms

# Let the ring fill with the pivot already on screen, so the zoom and pan below
# have something to move through rather than two points at the right edge.
Sleep 45s
Screenshot vhs/out/p7-pivot.png

# Narrow onto the recent past. The axis glides; the vertical scale follows it a
# beat behind, critically damped, because overshoot on an axis would show a
# value crossing a threshold it never crossed.
Type "j"
Sleep 900ms
Screenshot vhs/out/p7-zoomed.png

# Pan back into history. The window detaches from live and the status bar says
# how far behind it is, in the same units as the axis.
Type "hhh"
Sleep 1500ms
Screenshot vhs/out/p7-panned.png

# L reattaches to live.
Type "L"
Sleep 2s

# The same view in the light palette, because the spectrum has to survive both.
# The still lives in p7-light.tape rather than here: prism asks for a palette
# background but the request does not survive the recording, so pressing d on a
# dark terminal gives light-mode text on black, which is neither palette. That
# frame was committed as p7-light.png for the whole of phase 7. The toggle stays
# in the gif, where it is a record of what the key does rather than of what the
# palette looks like.
Type "d"
Sleep 1500ms

Type "d"
Sleep 800ms

# The key reference slides up from behind the footer to close.
Type "?"
Sleep 1200ms
Screenshot vhs/out/p7-help.png

Type "?"
Sleep 800ms
Type "q"
Sleep 400ms
