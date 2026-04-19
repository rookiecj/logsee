package ui

import (
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/filter"
	"git.inpt.fr/42dottools/log/internal/pipeline"
)

// classifyIncoming runs the classifier on one newly-pushed stdin line and
// records the resulting FindingKind (if any) under its Seq. Keeping this
// a thin method on *Model lets later phases swap in block analyzers and
// span accumulation without disturbing applyIncomingLines.
//
// No-op when the effective log format is not Android — Tier A rules are
// adb-specific and firing them on plain text only adds noise.
func (m *Model) classifyIncoming(seq int64, text string) {
	if m.classifier == nil || m.findings == nil {
		return
	}
	if m.effectiveLogFmt != filter.FormatAndroid {
		return
	}
	rec := pipeline.BuildRecord(domain.Line{Seq: seq, Text: text}, domain.LineFormatAndroid)
	out := m.classifier.OnRecord(rec)
	if len(out.Findings) == 0 {
		return
	}
	m.findings[seq] = out.Findings[0].Kind
}
