package block

import (
	"regexp"
	"strings"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
)

var javaExceptionRE = regexp.MustCompile(
	`^(java\.[\w.$]+(?:Exception|Error)|[a-zA-Z][\w.$]+(?:Exception|Error))(?::\s*(.*))?$`,
)

// JavaFatal aggregates a FATAL EXCEPTION block on the AndroidRuntime tag
// (header, stack trace, Caused-by chain) into a single Span. Ends when a
// non-AndroidRuntime record arrives — in practice the next line is usually
// "Process: Sending signal PID: X SIG: 9" on the Process tag.
type JavaFatal struct {
	active       bool
	start        domain.Seq
	end          domain.Seq
	pid          int32
	headerMsg    string
	exceptionMsg string
}

func NewJavaFatal() *JavaFatal { return &JavaFatal{} }

func (j *JavaFatal) Name() string { return "block.java" }

func (j *JavaFatal) OnRecord(r domain.Record) analysis.Output {
	msg := strings.TrimSpace(r.Message)
	if !j.active {
		if r.Tag == "AndroidRuntime" && isJavaFatalHeader(msg) {
			j.active = true
			j.start = r.Seq
			j.end = r.Seq
			j.pid = r.PID
			j.headerMsg = trimForSummary(msg)
		}
		return analysis.Output{}
	}

	if r.Tag == "AndroidRuntime" {
		j.end = r.Seq
		if j.exceptionMsg == "" {
			if m := javaExceptionRE.FindStringSubmatch(msg); len(m) >= 2 {
				j.exceptionMsg = m[1]
				if len(m) >= 3 && m[2] != "" {
					j.exceptionMsg = j.exceptionMsg + ": " + m[2]
				}
				j.exceptionMsg = trimForSummary(j.exceptionMsg)
			}
		}
		return analysis.Output{}
	}
	return j.emit()
}

func (j *JavaFatal) Flush() analysis.Output {
	if j.active {
		return j.emit()
	}
	return analysis.Output{}
}

func (j *JavaFatal) emit() analysis.Output {
	summary := j.exceptionMsg
	if summary == "" {
		summary = j.headerMsg
	}
	span := domain.Span{
		Kind:      domain.SpanJavaFatal,
		StartSeq:  j.start,
		EndSeq:    j.end,
		PID:       j.pid,
		Summary:   summary,
		SchemaVer: domain.SchemaVersion,
	}
	j.reset()
	return analysis.Output{Spans: []domain.Span{span}}
}

func (j *JavaFatal) reset() {
	j.active = false
	j.start = 0
	j.end = 0
	j.pid = 0
	j.headerMsg = ""
	j.exceptionMsg = ""
}

func isJavaFatalHeader(msg string) bool {
	return strings.HasPrefix(msg, "FATAL EXCEPTION") ||
		strings.HasPrefix(msg, "*** FATAL EXCEPTION")
}

func trimForSummary(s string) string {
	const max = 180
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
