# Phase 2 - spring-eased zoom and pan.
#
# The gif is the artifact that matters here; the stills bracket the motion.
#
# Two things changed when the artifact backlog was worked through. It points at
# deploy/prism-demo.yaml rather than config.example.yaml, because the shipped
# 15s default bought about one bucket inside the old ten-second warm-up and
# left nothing to zoom into. And it accumulates two and a half minutes before
# the first frame, because a window selector is only worth photographing when
# widening it reaches something: at 5m with 30s of history every window from 5m
# up is the same picture.
#
# p2-zoom-mid is the exception to the "capture at rest" rule and the reason
# there is no sleep before it - it is meant to be the spring caught in flight.
# Whether VHS can catch it is a question the recording answers rather than the
# tape asserts: Screenshot does not capture at the instant it is reached, and
# the zoom settles in about 0.72s.
Output vhs/out/p2-zoom.gif

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
Type "prism" Enter
Sleep 8s

# Filtered off screen, for the reason given in p1-dashboard.tape: prism opens on
# the alphabetically first family and against Envoy that one is flat at zero.
# A window is easiest to read against data that has a shape, because the whole
# claim of the phase is that the same data is being shown at another resolution.
Type "/"
Sleep 600ms
Type "upstream_rq_total"
Sleep 800ms
Enter
Sleep 1500ms

# Enough history that widening the window has somewhere to go. This waits
# inside Hide deliberately: VHS records no frames while hidden, so two and a
# half minutes of a chart filling costs the gif nothing. Left visible it was
# most of a four minute recording and the single most expensive thing in
# `task tape`, all of it spent on the part with nothing to demonstrate.
Sleep 150s
Show

Screenshot vhs/out/p2-window-5m.png
Sleep 2s

# j narrows a step at a time. The axis glides rather than snapping and the
# vertical scale follows it, critically damped, a beat behind.
Type "j"
Screenshot vhs/out/p2-zoom-mid.png
Sleep 2s

Type "j"
Sleep 2s
Screenshot vhs/out/p2-zoom-30s.png
Sleep 2s

# k widens back out past the opening window until the buffer runs out. The
# ring holds 15m, so 1h is not available and the status bar says why rather
# than drawing an hour of blank.
Type "kkkk"
Sleep 3s
Screenshot vhs/out/p2-horizon.png
Sleep 2s

# Back down to a minute before panning. At the horizon the window already
# spans every bucket the ring holds, so there is nothing to the left to pan
# into and h is correctly a no-op - which made the first recording of this tape
# produce a p2-paused.png identical to the horizon frame above it.
Type "jj"
Sleep 2s

# h pans into history, which detaches the window from live and says how far
# behind it now sits, in the same units as the axis.
Type "hhh"
Sleep 2s
Screenshot vhs/out/p2-paused.png
Sleep 2s

# L reattaches.
Type "L"
Sleep 2s

# The key reference slides up from behind the footer.
Type "?"
Sleep 1500ms
Screenshot vhs/out/p2-help.png
Sleep 2s

Type "?"
Sleep 800ms
Type "q"
Sleep 400ms
