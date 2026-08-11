package stream

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pantherhawk/prism/internal/series"
)

// Frame is one scrape as it travels over the wire.
//
// It carries samples rather than rendered state, so a receiving prism rebuilds
// exactly the store it would have built by scraping the target itself. The
// alternative - shipping snapshots - would put the ring buffer's layout in the
// protocol and freeze it there.
type Frame struct {
	At      time.Time       `json:"at"`
	Samples []series.Sample `json:"samples"`
	Stats   series.Stats    `json:"stats"`
}

// encode renders a frame as a single-line JSON payload.
//
// One line matters: server-sent events are newline delimited, and a payload
// containing a newline would be read as two events. Go's JSON encoder never
// emits a bare newline inside a value, so compact marshalling is enough.
func encode(frame Frame) ([]byte, error) {
	payload, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("encode frame: %w", err)
	}

	return payload, nil
}

// decode parses a payload back into a frame.
func decode(payload []byte) (Frame, error) {
	var frame Frame

	if err := json.Unmarshal(payload, &frame); err != nil {
		return Frame{}, fmt.Errorf("decode frame: %w", err)
	}

	return frame, nil
}
