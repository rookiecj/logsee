// Package block parses multi-line Android anomalies (native tombstone,
// Java FATAL with its Caused-by chain, ANR with its CPU-usage tail) into
// single Spans. Each analyzer is a small state machine keyed off tag and
// message prefix; it emits a Span when the block terminates, either by a
// tag change or by a content-specific close marker.
//
// Block analyzers do not duplicate the Classifier's line-level Findings —
// correlation of a Finding to its enclosing Span happens post-hoc in the
// pipeline by walking ranges.
package block
