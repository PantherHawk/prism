GO    ?= go
BIN   := ./bin/prism
TAPES := $(wildcard vhs/*.tape)

COMPOSE      := docker compose -f deploy/docker-compose.yaml
ENVOY_ADMIN  ?= http://localhost:9901
ENVOY_STATS  := $(ENVOY_ADMIN)/stats/prometheus
ENVOY_FIXTURE := features/testdata/envoy-stats.txt

.PHONY: all deps build lint fmt test bdd bdd-envoy tape artifacts clean \
        envoy-up envoy-down envoy-fixture

# The frames that make up the visual changelog in design/, in phase order.
#
# Listed rather than globbed. vhs/out/ also holds a working gif per tape, and
# the stills a tape happens to take are not all of them artifacts; naming them
# here is what makes `make artifacts` a decision about the changelog rather
# than a copy of whatever the last run left behind.
ARTIFACTS := \
	p0-banner-dark.png p0-banner-light.png \
	p1-dashboard-dark.png p1-dashboard-light.png p1-series-selected.png \
	p2-window-5m.png p2-zoom-mid.png p2-zoom-30s.png \
	p2-horizon.png p2-paused.png p2-help.png \
	p3-filter.png p3-pivot.png p3-pivot-light.png p3-empty.png \
	p4-cardinality.png \
	p5-remote.png p5-reconnecting.png \
	p6-dark.png p6-light.png \
	p7-arrive.png p7-dashboard.png p7-histogram.png p7-filtered.png \
	p7-pivot.png p7-zoomed.png p7-panned.png p7-light.png p7-help.png \
	p7-walkthrough.gif

all: lint test build

# go.mod carries only the module and language version so that nothing is
# pinned to a version this repository has not actually resolved. Run this once.
deps:
	$(GO) get charm.land/bubbletea/v2@latest
	$(GO) get charm.land/lipgloss/v2@latest
	$(GO) get charm.land/bubbles/v2@latest
	$(GO) get github.com/charmbracelet/harmonica@latest
	$(GO) get github.com/prometheus/common/expfmt@latest
	$(GO) get github.com/cucumber/godog@latest
	$(GO) get go.opentelemetry.io/otel@latest
	$(GO) get go.opentelemetry.io/otel/sdk@latest
	$(GO) get go.opentelemetry.io/otel/sdk/metric@latest
	$(GO) get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest
	$(GO) get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc@latest
	$(GO) get gopkg.in/yaml.v3@latest
	$(GO) mod tidy

build:
	$(GO) build -trimpath -o $(BIN) ./cmd/prism

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

test:
	$(GO) test -race -shuffle=on -cover ./...

# Acceptance criteria, executed. Every scenario maps to a phase in PLAN.md.
#
# The Envoy scenarios run here too, replaying the recorded exposition in
# features/testdata. That keeps the gate runnable without a container runtime
# while still grading prism against bytes Envoy actually wrote.
bdd:
	$(GO) test -race -tags bdd ./features/...

# The same scenarios against the live admin port. Requires `make envoy-up`.
# Recording drift is the failure this catches: the fixture is a moment in one
# Envoy version's life, and this proves the moment still resembles the present.
bdd-envoy:
	PRISM_ENVOY_ENDPOINT=$(ENVOY_STATS) \
		$(GO) test -count=1 -race -tags bdd ./features/ -godog.tags=@envoy

# ---- the target prism is built against ---------------------------------------

# Brings the stack up and does not return until Envoy is serving stats that
# describe traffic. Returning at "container started" would hand the suite an
# endpoint whose histograms are all declared and empty.
envoy-up:
	$(COMPOSE) up -d
	@printf 'waiting for envoy admin at %s ' "$(ENVOY_ADMIN)"
	@for i in $$(seq 1 60); do \
		if curl -fsS -o /dev/null "$(ENVOY_ADMIN)/ready"; then break; fi; \
		printf '.'; sleep 1; \
	done; printf '\n'
	@curl -fsS -o /dev/null "$(ENVOY_ADMIN)/ready" \
		|| { echo "envoy admin never became ready"; $(COMPOSE) logs envoy; exit 1; }
	@printf 'waiting for upstream traffic '
	@for i in $$(seq 1 60); do \
		if curl -fsS "$(ENVOY_STATS)" \
			| grep -cE '^envoy_cluster_upstream_rq_completed\{[^}]*\} [1-9]' >/dev/null; \
		then break; fi; \
		printf '.'; sleep 1; \
	done; printf '\n'
	@echo "envoy is serving $(ENVOY_STATS)"

envoy-down:
	$(COMPOSE) down -v --remove-orphans

# Re-records the exposition the acceptance suite replays. Deliberately manual:
# a fixture that regenerates as a build step is not a fixture, it is a mirror,
# and a change in what Envoy emits would then never fail anything.
envoy-fixture:
	@mkdir -p $(dir $(ENVOY_FIXTURE))
	curl -fsS "$(ENVOY_STATS)" > $(ENVOY_FIXTURE)
	@printf '%s: ' "$(ENVOY_FIXTURE)"; wc -l < $(ENVOY_FIXTURE)

# Regenerates every walkthrough frame. Requires vhs and ttyd on PATH, and the
# stack from deploy/ up, since the tapes record prism against a real target.
#
# The freshly built binary goes on PATH ahead of anything installed, so a tape
# records the working tree rather than whatever version happens to be in
# /usr/local/bin.
tape: build
	@mkdir -p vhs/out
	@for t in $(TAPES); do \
		echo "vhs $$t"; \
		PATH="$(CURDIR)/bin:$$PATH" vhs $$t || exit 1; \
	done

# Promotes the frames in vhs/out/ into design/, which is the visual changelog
# the README and PLAN.md link to.
#
# Deliberately not folded into `tape`. A live chart never renders twice the
# same, so wiring the copy into the recording would put a diff on all thirty
# artifacts every time anybody re-recorded any one phase, and a changelog that
# restates itself on every run is not a changelog. Keeping it a second command
# means design/ changes only when somebody meant it to.
artifacts:
	@for f in $(ARTIFACTS); do \
		if [ ! -f "vhs/out/$$f" ]; then \
			echo "missing vhs/out/$$f - run make tape"; exit 1; \
		fi; \
	done
	@for f in $(ARTIFACTS); do cp "vhs/out/$$f" design/; done
	@echo "promoted $(words $(ARTIFACTS)) artifacts into design/"

clean:
	rm -rf bin vhs/out
