// Package config reads the single YAML document that configures prism.
//
// It is deliberately dumb: it resolves a path, decodes a file, and returns a
// struct. It performs no I/O beyond that, contacts no network, and constructs
// no live objects. A depguard rule keeps it importable only from the program
// entrypoint and from app.Build, so configuration cannot leak into the domain
// packages as an ambient dependency.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/pantherhawk/prism/internal/logging"
	"github.com/pantherhawk/prism/internal/scrape"
	"github.com/pantherhawk/prism/internal/stream"
	"github.com/pantherhawk/prism/internal/telemetry"
	"github.com/pantherhawk/prism/internal/theme"
)

const (
	// PathEnv is the only environment variable prism reads. Everything else
	// arrives through the YAML file it points at, or through the OTEL_*
	// variables consumed directly by the OpenTelemetry SDK.
	PathEnv = "PRISM_CONFIG"

	// DefaultPath is used when PathEnv is unset or empty.
	DefaultPath = "/etc/prism/config.yaml"
)

// ErrMissingConfig is returned when a path was named explicitly but no file
// exists there. A missing file at [DefaultPath] is not an error: prism runs on
// defaults.
var ErrMissingConfig = errors.New("config file not found")

// Config is composed entirely of the configuration types owned by the packages
// they configure, so that each package defines its own defaults and validation
// and this one never has to know what a scrape interval means.
type Config struct {
	Logging   logging.Config   `yaml:"logging"`
	Telemetry telemetry.Config `yaml:"telemetry"`
	Scrape    scrape.Config    `yaml:"scrape"`
	Stream    stream.Config    `yaml:"stream"`
	Theme     theme.Config     `yaml:"theme"`
}

// Default returns the configuration prism uses when no file is present.
func Default() Config {
	return Config{
		Logging:   logging.Default(),
		Telemetry: telemetry.Default(),
		Scrape:    scrape.Default(),
		Stream:    stream.Default(),
		Theme:     theme.Default(),
	}
}

// Read resolves path, falling back to [DefaultPath] when it is empty, and
// decodes the YAML document there over the defaults. A missing file at the
// default path yields the defaults; a missing file at an explicit path is an
// error, because silently ignoring the operator's chosen file is worse than
// failing to start.
func Read(path string) (Config, error) {
	resolved, explicit := path, true
	if resolved == "" {
		resolved, explicit = DefaultPath, false
	}

	contents, err := os.ReadFile(resolved)

	switch {
	case errors.Is(err, fs.ErrNotExist) && !explicit:
		return Default(), nil
	case errors.Is(err, fs.ErrNotExist):
		return Default(), fmt.Errorf("%w: %s", ErrMissingConfig, resolved)
	case err != nil:
		return Default(), fmt.Errorf("read %s: %w", resolved, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(contents, &cfg); err != nil {
		return Default(), fmt.Errorf("decode %s: %w", resolved, err)
	}

	return cfg, nil
}
