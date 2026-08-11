package logging_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pantherhawk/prism/internal/logging"
)

// A terminal is where the dashboard is. Writing a record there puts it through
// a frame, so the record is the thing that gives way.
//
// /dev/null stands in for the terminal: both are character devices, which is
// the property the decision actually turns on, and a test that needed a real
// pty would not get written.
func TestRecordsAreDroppedWhenStderrIsATerminal(t *testing.T) {
	t.Parallel()

	device, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}

	t.Cleanup(func() {
		closeErr := device.Close()
		if closeErr != nil {
			t.Errorf("close: %v", closeErr)
		}
	})

	if got := logging.Destination(device); got != io.Discard {
		t.Errorf("Destination(a character device) = %T, want io.Discard", got)
	}
}

// Redirected, there is no frame to protect and the records are worth keeping.
func TestRecordsAreKeptWhenStderrIsRedirected(t *testing.T) {
	t.Parallel()

	file, err := os.Create(filepath.Join(t.TempDir(), "prism.log"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Cleanup(func() {
		closeErr := file.Close()
		if closeErr != nil {
			t.Errorf("close: %v", closeErr)
		}
	})

	if got := logging.Destination(file); got != io.Writer(file) {
		t.Errorf("Destination(a regular file) = %T, want the file itself", got)
	}
}

func TestNewToWritesRecordsAtTheConfiguredLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	log, err := logging.NewTo(logging.Config{Level: "warn", Format: logging.FormatText}, &buf)
	if err != nil {
		t.Fatalf("NewTo: %v", err)
	}

	log.InfoContext(t.Context(), "beneath the level")
	log.WarnContext(t.Context(), "at the level")

	got := buf.String()
	if strings.Contains(got, "beneath") || !strings.Contains(got, "at the level") {
		t.Errorf("records = %q, want only the warning", got)
	}
}

func TestUnknownFormatIsRejected(t *testing.T) {
	t.Parallel()

	_, err := logging.NewTo(logging.Config{Level: "info", Format: "yaml"}, io.Discard)
	if err == nil {
		t.Error("an unknown format was accepted")
	}
}
