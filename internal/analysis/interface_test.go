package analysis

import (
	"testing"

	"git.inpt.fr/42dottools/log/internal/domain"
)

func TestOutput_Empty(t *testing.T) {
	var o Output
	if !o.Empty() {
		t.Error("zero Output should be Empty")
	}
	o.Findings = []domain.Finding{{Kind: domain.FindingANR}}
	if o.Empty() {
		t.Error("Output with findings should not be Empty")
	}
	o = Output{Spans: []domain.Span{{Kind: domain.SpanANR}}}
	if o.Empty() {
		t.Error("Output with spans should not be Empty")
	}
}
