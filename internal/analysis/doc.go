// Package analysis hosts the heterogeneous abnormality-detection layer.
//
// Sub-packages plug into the pipeline through the Analyzer / BlockAnalyzer /
// StatefulAnalyzer ports. Detectors must be pure functions of their Record
// input — I/O and TUI state belong elsewhere. See
// docs/architecture/anomaly-detection.md.
package analysis
