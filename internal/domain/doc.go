// Package domain holds pure data types shared across the analysis pipeline.
//
// Types here must not import any other internal/* package. Seq is the single
// cross-layer key that ties L0 lines, L1 records, L2 spans, and L3 findings
// together. See docs/architecture/anomaly-detection.md for the layered model.
package domain
