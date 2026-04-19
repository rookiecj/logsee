package source

import (
	"context"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// LogSource is the inbound port: anything that can emit log Lines keyed
// by a strictly monotonic Seq starting at 1. Adapters wrap stdin, a file,
// `adb logcat`, journalctl, etc.
//
// Contract:
//   - Lines returns a channel that is closed when the source is exhausted
//     or ctx is cancelled, so callers can safely `for line := range …`.
//   - Implementations own the producer goroutine; callers must still call
//     Close for resource cleanup.
type LogSource interface {
	Lines(ctx context.Context) (<-chan domain.Line, error)
	Close() error
}
