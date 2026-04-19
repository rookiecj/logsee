// Package pipeline wires LogSource → RecordBuilder → fanout(Analyzers) → Store.
//
// The pipeline is the only place that owns goroutines and channels; Analyzers
// and Adapters stay pure. Backpressure is bounded per stage with drop-oldest
// and lag metrics. See docs/architecture/anomaly-detection.md.
package pipeline
