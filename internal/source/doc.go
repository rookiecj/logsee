// Package source defines the LogSource inbound port and its adapters.
//
// Adapters (stdin, file, adb, journalctl, …) expose a uniform channel of
// domain.Line values so the analysis pipeline is agnostic to where logs
// originate. See docs/architecture/anomaly-detection.md.
package source
