package block

import (
	"regexp"
	"strings"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
)

var kernelPanicStartRE = regexp.MustCompile(`^(BUG:|Oops:|Kernel panic)`)

// KernelPanic collapses a kernel BUG/Oops/panic report on the `kernel`
// tag into a single Span. The block starts at the first matching kernel
// line and extends until a non-kernel record arrives — real-world panics
// usually finish with `---[ end Kernel panic ...]---` followed by the
// system restart stream on other units.
type KernelPanic struct {
	active  bool
	start   domain.Seq
	end     domain.Seq
	summary string
}

func NewKernelPanic() *KernelPanic { return &KernelPanic{} }

func (k *KernelPanic) Name() string { return "block.kernel_panic" }

func (k *KernelPanic) OnRecord(r domain.Record) analysis.Output {
	if r.Format != domain.LineFormatJournal {
		if k.active {
			return k.emit()
		}
		return analysis.Output{}
	}
	msg := strings.TrimSpace(r.Message)
	if !k.active {
		if r.Tag == "kernel" && kernelPanicStartRE.MatchString(msg) {
			k.active = true
			k.start = r.Seq
			k.end = r.Seq
			k.summary = trimForSummary(msg)
		}
		return analysis.Output{}
	}
	if r.Tag != "kernel" {
		return k.emit()
	}
	k.end = r.Seq
	// Prefer the "Kernel panic" message as summary when it appears later
	// in the block.
	if strings.HasPrefix(msg, "Kernel panic") {
		k.summary = trimForSummary(msg)
	}
	return analysis.Output{}
}

func (k *KernelPanic) Flush() analysis.Output {
	if k.active {
		return k.emit()
	}
	return analysis.Output{}
}

func (k *KernelPanic) emit() analysis.Output {
	span := domain.Span{
		Kind:      domain.SpanKernelPanic,
		StartSeq:  k.start,
		EndSeq:    k.end,
		Summary:   k.summary,
		SchemaVer: domain.SchemaVersion,
	}
	k.reset()
	return analysis.Output{Spans: []domain.Span{span}}
}

func (k *KernelPanic) reset() {
	k.active = false
	k.start = 0
	k.end = 0
	k.summary = ""
}
