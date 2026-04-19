package block

import (
	"strings"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
)

// SystemdCoredump aggregates contiguous `systemd-coredump[…]:` lines into
// a single Span. The block starts on the first such record and ends on
// the next non-coredump tag — systemd-coredump emits its report (header,
// stack trace, module list, coredump path) as a single burst, so tag
// change is a reliable boundary.
type SystemdCoredump struct {
	active  bool
	start   domain.Seq
	end     domain.Seq
	pid     int32
	summary string
}

func NewSystemdCoredump() *SystemdCoredump { return &SystemdCoredump{} }

func (s *SystemdCoredump) Name() string { return "block.systemd_coredump" }

func (s *SystemdCoredump) OnRecord(r domain.Record) analysis.Output {
	if r.Format != domain.LineFormatJournal {
		if s.active {
			return s.emit()
		}
		return analysis.Output{}
	}
	if !s.active {
		if r.Tag == "systemd-coredump" {
			s.active = true
			s.start = r.Seq
			s.end = r.Seq
			s.pid = r.PID
			s.summary = trimForSummary(strings.TrimSpace(r.Message))
		}
		return analysis.Output{}
	}
	if r.Tag != "systemd-coredump" {
		return s.emit()
	}
	s.end = r.Seq
	return analysis.Output{}
}

func (s *SystemdCoredump) Flush() analysis.Output {
	if s.active {
		return s.emit()
	}
	return analysis.Output{}
}

func (s *SystemdCoredump) emit() analysis.Output {
	span := domain.Span{
		Kind:      domain.SpanSystemdCoredump,
		StartSeq:  s.start,
		EndSeq:    s.end,
		PID:       s.pid,
		Summary:   s.summary,
		SchemaVer: domain.SchemaVersion,
	}
	s.reset()
	return analysis.Output{Spans: []domain.Span{span}}
}

func (s *SystemdCoredump) reset() {
	s.active = false
	s.start = 0
	s.end = 0
	s.pid = 0
	s.summary = ""
}
