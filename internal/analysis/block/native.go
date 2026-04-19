package block

import (
	"regexp"
	"strings"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
)

var (
	nativeHeaderRE  = regexp.MustCompile(`^\*\*\*\s+\*\*\*\s+\*\*\*`)
	nativeProcessRE = regexp.MustCompile(`>>>\s+(\S+)\s+<<<`)
	nativeSignalRE  = regexp.MustCompile(`signal\s+\d+\s+\(\w+\)`)
)

// NativeCrash aggregates lines of a tombstone dump (DEBUG tag, triple-star
// header through the final backtrace entry) into a single Span. The block
// terminates the moment a non-DEBUG record arrives.
type NativeCrash struct {
	active  bool
	start   domain.Seq
	end     domain.Seq
	pid     int32
	process string
	signal  string
}

// New returns a fresh native-crash block analyzer.
func NewNativeCrash() *NativeCrash { return &NativeCrash{} }

func (n *NativeCrash) Name() string { return "block.native" }

func (n *NativeCrash) OnRecord(r domain.Record) analysis.Output {
	if !n.active {
		if r.Tag == "DEBUG" && nativeHeaderRE.MatchString(r.Message) {
			n.active = true
			n.start = r.Seq
			n.end = r.Seq
			n.pid = r.PID
		}
		return analysis.Output{}
	}

	if r.Tag == "DEBUG" {
		n.end = r.Seq
		if n.process == "" {
			if m := nativeProcessRE.FindStringSubmatch(r.Message); len(m) == 2 {
				n.process = m[1]
			}
		}
		if n.signal == "" {
			if m := nativeSignalRE.FindString(r.Message); m != "" {
				n.signal = m
			}
		}
		return analysis.Output{}
	}
	return n.emit()
}

func (n *NativeCrash) Flush() analysis.Output {
	if n.active {
		return n.emit()
	}
	return analysis.Output{}
}

func (n *NativeCrash) emit() analysis.Output {
	span := domain.Span{
		Kind:      domain.SpanNativeCrash,
		StartSeq:  n.start,
		EndSeq:    n.end,
		PID:       n.pid,
		Summary:   nativeSummary(n.process, n.signal),
		SchemaVer: domain.SchemaVersion,
	}
	n.reset()
	return analysis.Output{Spans: []domain.Span{span}}
}

func (n *NativeCrash) reset() {
	n.active = false
	n.start = 0
	n.end = 0
	n.pid = 0
	n.process = ""
	n.signal = ""
}

func nativeSummary(process, signal string) string {
	parts := []string{"native crash"}
	if process != "" {
		parts = append(parts, "in "+process)
	}
	if signal != "" {
		parts = append(parts, signal)
	}
	return strings.Join(parts, ", ")
}
