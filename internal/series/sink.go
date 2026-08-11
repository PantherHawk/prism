package series

import "time"

// Sink receives one scrape's worth of samples.
//
// It exists so that the collector does not know whether it is feeding a store,
// a network broker, or both. [Store] implements it; so does the stream broker.
type Sink interface {
	Append(at time.Time, samples []Sample, stats Stats)
}

// fanout writes to several sinks in order.
type fanout []Sink

// Fanout returns a sink that forwards to each of the given sinks in turn.
//
// Order matters: the local store is written first so that a slow or wedged
// network consumer can never delay the data the operator is actually looking
// at.
func Fanout(sinks ...Sink) Sink {
	return fanout(sinks)
}

// Append forwards a scrape to every sink.
func (f fanout) Append(at time.Time, samples []Sample, stats Stats) {
	for _, sink := range f {
		sink.Append(at, samples, stats)
	}
}
