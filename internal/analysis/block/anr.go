package block

import (
	"regexp"
	"strings"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
)

var anrTotalRE = regexp.MustCompile(`^\s*\d+%\s+TOTAL`)

// ANR aggregates an "ANR in …" report on the ActivityManager tag (header,
// PID/Reason/Parent/Load metadata, per-process CPU usage, and TOTAL line)
// into a single Span. The block ends at either the TOTAL line (inclusive)
// or a tag change away from ActivityManager, whichever comes first —
// real-world ANR reports always close with TOTAL.
type ANR struct {
	active  bool
	start   domain.Seq
	end     domain.Seq
	pid     int32
	summary string
}

func NewANR() *ANR { return &ANR{} }

func (a *ANR) Name() string { return "block.anr" }

func (a *ANR) OnRecord(r domain.Record) analysis.Output {
	msg := strings.TrimSpace(r.Message)
	if !a.active {
		if r.Tag == "ActivityManager" && strings.HasPrefix(msg, "ANR in") {
			a.active = true
			a.start = r.Seq
			a.end = r.Seq
			a.pid = r.PID
			a.summary = trimForSummary(msg)
		}
		return analysis.Output{}
	}

	if r.Tag != "ActivityManager" {
		return a.emit()
	}
	a.end = r.Seq
	if anrTotalRE.MatchString(r.Message) {
		return a.emit()
	}
	return analysis.Output{}
}

func (a *ANR) Flush() analysis.Output {
	if a.active {
		return a.emit()
	}
	return analysis.Output{}
}

func (a *ANR) emit() analysis.Output {
	span := domain.Span{
		Kind:      domain.SpanANR,
		StartSeq:  a.start,
		EndSeq:    a.end,
		PID:       a.pid,
		Summary:   a.summary,
		SchemaVer: domain.SchemaVersion,
	}
	a.reset()
	return analysis.Output{Spans: []domain.Span{span}}
}

func (a *ANR) reset() {
	a.active = false
	a.start = 0
	a.end = 0
	a.pid = 0
	a.summary = ""
}
