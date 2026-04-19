// Package store provides the append-only indexes that hold derived layers
// (records, spans, findings) keyed by domain.Seq ranges.
//
// The v1 driver is an in-memory implementation; a persistent driver (JSONL
// or bbolt) is added in a later phase. See
// docs/architecture/anomaly-detection.md for the layering rules.
package store
