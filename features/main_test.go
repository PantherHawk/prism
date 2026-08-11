//go:build bdd

// Package features_test executes prism's acceptance criteria.
//
// The feature files in this directory are the criteria from PLAN.md written so
// that they run. They are behind the `bdd` build tag so that `go test ./...`
// stays a fast unit run and `task bdd` is the deliberate, slower gate: several
// scenarios stand up HTTP servers and wait on real timers.
package features_test

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/spf13/pflag"
)

// wipTag excludes scenarios for phases that have not landed.
//
// PLAN.md writes a phase's criteria with the plan rather than after the code,
// so a feature file describes work that is not finished yet. Removing the tag
// is part of finishing the phase, which makes the tag a to-do list that cannot
// drift from the suite.
const wipTag = "~@wip"

// godogPrefix namespaces godog's flags away from the -test.* ones.
const godogPrefix = "godog."

// opts is bound to the go test command line, so that a subset of the suite can
// be run with `go test -tags bdd ./features/ -godog.tags=@envoy` without
// editing this file.
//
//nolint:gochecknoglobals // godog binds flags at package scope
var opts = godog.Options{
	Format:      "pretty",
	Paths:       []string{"."},
	Strict:      true,
	Tags:        wipTag,
	Concurrency: 1,
}

// init registers godog's flags where `go test` can reach them.
//
// [godog.BindCommandLineFlags] binds against pflag.CommandLine, not the
// standard flag set, so a plain flag.Parse rejects every -godog.* flag as
// undefined. Parsing with pflag instead is worse: pflag reads a single leading
// dash as a cluster of shorthands, so `-test.run=X` is taken as the -t
// shorthand — godog's own tag flag — carrying the value `est.run=X`, and the
// test filter silently never reaches the testing package.
//
// So the flags are mirrored into the standard set instead, and one flag.Parse
// serves both. A pflag.Value already satisfies flag.Value, and it is registered
// rather than copied, so a flag set here writes through to the same [opts].
//
//nolint:gochecknoinits // flag binding has to happen before TestMain runs
func init() {
	godog.BindCommandLineFlags(godogPrefix, &opts)

	pflag.CommandLine.VisitAll(func(bound *pflag.Flag) {
		if !strings.HasPrefix(bound.Name, godogPrefix) {
			return
		}

		if flag.CommandLine.Lookup(bound.Name) != nil {
			return
		}

		flag.CommandLine.Var(bound.Value, bound.Name, bound.Usage)
	})
}

func TestMain(m *testing.M) {
	flag.Parse()

	opts.Output = colors.Colored(os.Stdout)

	os.Exit(m.Run())
}

// TestFeatures runs every scenario in this directory.
//
// Strict mode is on: a step with no definition fails the suite rather than
// being reported and skipped. A criterion that silently does not run is worse
// than one that fails, because it still reads as green.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name:                 "prism",
		ScenarioInitializer:  initializeScenario,
		TestSuiteInitializer: nil,
		Options:              &opts,
	}

	if status := suite.Run(); status != 0 {
		t.Fatalf("acceptance criteria failed with status %d", status)
	}
}

// initializeScenario registers every phase's steps.
//
// One registry rather than one suite per feature: the steps are written in
// prose, and prose from one phase turns up in another's scenarios. Registering
// them together is what lets `Given a buffer holding 15m at 15s resolution`
// mean the same thing wherever it appears.
func initializeScenario(sc *godog.ScenarioContext) {
	initializeLifecycle(sc)
	initializeScrape(sc)
	initializeWindow(sc)
	initializeFilter(sc)
	initializeCardinality(sc)
	initializeStream(sc)
	initializeTheme(sc)
	initializeEnvoy(sc)
}
