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
// No-op when the effective log format has no Tier A rules (Plain/Bracket/
// Unknown) — firing them on free-form text only adds noise.
func (m *Model) classifyIncoming(seq int64, text string) {
	if m.classifier == nil || m.findings == nil {
		return
	}
	var fmt domain.LineFormat
	switch m.effectiveLogFmt {
	case filter.FormatAndroid:
		fmt = domain.LineFormatAndroid
	case filter.FormatSystemdJournal:
		fmt = domain.LineFormatJournal
	default:
		return
	}
	rec := pipeline.BuildRecord(domain.Line{Seq: seq, Text: text}, fmt)
	out := m.classifier.OnRecord(rec)
	if len(out.Findings) == 0 {
		return
	}
	m.findings[seq] = out.Findings[0].Kind
}
