# Phase 5 - HTTP streaming, and the link going away.
#
# The only tape that needs two prisms. One scrapes Envoy and republishes every
# scrape on /stream; the other never touches Envoy at all and rebuilds its
# store from what arrives. The one on screen is the follower, and the header
# names 127.0.0.1:9099 rather than the admin port, which is the phase's claim
# made visible: ten watchers, one scrape.
#
# Two mechanics are worth knowing before changing anything here.
#
# The publisher needs a terminal. prism is a Bubble Tea program and exits with
# "error opening TTY" if it is started with its output redirected, so it cannot
# simply be backgrounded - it is run under script(1), which allocates a pty and
# throws the rendered frames away. tmux would do as well where it is installed;
# script is in the base system on macOS and Linux both.
#
# The kill has to be scheduled rather than typed. Once the follower is running
# it owns the terminal, and there is no shell left to type into - so the
# subshell that takes the upstream away is launched during setup, before the
# recording starts, and fires on a timer. It kills by port rather than by name
# because both processes are the same binary and `pkill prism` would take the
# recording down with the thing it is trying to record.
#
# The port alone is not enough either: the follower holds a connection *to*
# 9099, so a bare `lsof -ti tcp:9099` names both ends and the first attempt at
# this killed the process being recorded. -sTCP:LISTEN picks the server.
#
# The timings below are load-bearing and interlock: the upstream dies at t+80s,
# which has to fall after the p5-remote screenshot and far enough before the
# p5-reconnecting one for the backoff to be showing a countdown.
Output vhs/out/p5-stream.gif

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
Type "cd $PWD" Enter

# The publisher: scrapes Envoy, serves /stream, renders into nowhere.
Type "script -q /dev/null env PRISM_CONFIG=$PWD/deploy/prism-publisher.yaml ./bin/prism </dev/null >/dev/null 2>&1 &" Enter
Sleep 2s

# Take the upstream away at t+80s, whatever the follower is doing by then.
Type "( sleep 80; lsof -ti tcp:9099 -sTCP:LISTEN | xargs kill ) >/dev/null 2>&1 &" Enter

# Let the publisher fill a little history before anyone follows it, so the
# follower's first frames are not the only thing either of them has seen.
Sleep 8s

Type "export PRISM_CONFIG=$PWD/deploy/prism-follower.yaml" Enter
Type "clear" Enter
Show

Type "prism"
Sleep 400ms
Enter
Sleep 1500ms
Sleep 6s

# The frames carry samples rather than rendered state, so the follower builds
# the same store it would have built by scraping Envoy itself - the same
# families, the same cardinality, the same rates.
Type "/"
Sleep 800ms
Type "upstream_rq_total"
Sleep 1200ms
Enter
Sleep 1500ms
Type "p"
Sleep 1500ms

Sleep 45s
Type "j"
Sleep 900ms
Screenshot vhs/out/p5-remote.png
Sleep 2s

# Around here the publisher is killed. The link drops, the status turns to a
# countdown, and the history already in the ring is left alone: what was
# observed happened, and losing the link does not unhappen it. The gap renders
# as a gap.
Sleep 25s
Screenshot vhs/out/p5-reconnecting.png
Sleep 2s

Type "q"
Sleep 400ms

# Leave nothing running for the next tape.
Hide
Type "lsof -ti tcp:9099 -sTCP:LISTEN | xargs kill" Enter
Sleep 1s
