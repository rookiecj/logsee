package block

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/pipeline"
)

func journalRec(seq int64, tag, msg string, pid int32) domain.Record {
	return domain.Record{Seq: seq, Format: domain.LineFormatJournal, Tag: tag, Message: msg, PID: pid}
}

func TestKernelPanic_StartAndEmit(t *testing.T) {
	k := NewKernelPanic()
	if !k.OnRecord(journalRec(1, "kernel", "BUG: kernel NULL pointer dereference", 0)).Empty() {
		t.Error("start record should not emit")
	}
	if !k.active {
		t.Error("should be active after BUG: start")
	}
	k.OnRecord(journalRec(2, "kernel", "PGD 0 P4D 0", 0))
	k.OnRecord(journalRec(3, "kernel", "Oops: 0000 [#1] SMP NOPTI", 0))
	k.OnRecord(journalRec(4, "kernel", "Kernel panic - not syncing: Fatal exception in interrupt", 0))
	out := k.OnRecord(journalRec(5, "systemd", "Startup finished", 1))
	if len(out.Spans) != 1 {
		t.Fatalf("expected 1 span on tag change, got %+v", out)
	}
	s := out.Spans[0]
	if s.Kind != domain.SpanKernelPanic {
		t.Errorf("Kind = %v, want SpanKernelPanic", s.Kind)
	}
	if s.StartSeq != 1 || s.EndSeq != 4 {
		t.Errorf("range = [%d,%d], want [1,4]", s.StartSeq, s.EndSeq)
	}
	if s.Summary == "" || s.Summary[:12] != "Kernel panic" {
		t.Errorf("Summary should prefer the Kernel panic line, got %q", s.Summary)
	}
}

func TestKernelPanic_OopsAlsoStarts(t *testing.T) {
	k := NewKernelPanic()
	k.OnRecord(journalRec(1, "kernel", "Oops: 0000 [#1] SMP", 0))
	if !k.active {
		t.Error("Oops: should start the block")
	}
}

func TestKernelPanic_IgnoresWrongFormat(t *testing.T) {
	k := NewKernelPanic()
	// Android record with kernel-like message should not start the block.
	r := domain.Record{Seq: 1, Format: domain.LineFormatAndroid, Tag: "kernel", Message: "BUG: not on linux"}
	if !k.OnRecord(r).Empty() {
		t.Error("non-journal format should not start or emit")
	}
	if k.active {
		t.Error("wrong format should leave analyzer inactive")
	}
}

func TestKernelPanic_Flush(t *testing.T) {
	k := NewKernelPanic()
	k.OnRecord(journalRec(1, "kernel", "BUG: oops", 0))
	k.OnRecord(journalRec(2, "kernel", "more trace", 0))
	out := k.Flush()
	if len(out.Spans) != 1 {
		t.Errorf("Flush should emit the open block, got %+v", out)
	}
}

func TestSystemdCoredump_StartAndEmit(t *testing.T) {
	s := NewSystemdCoredump()
	s.OnRecord(journalRec(10, "systemd-coredump", "Process 1789 (api-server) of user 1002 dumped core.", 2345))
	if !s.active {
		t.Error("coredump should start on systemd-coredump tag")
	}
	s.OnRecord(journalRec(11, "systemd-coredump", "Stack trace of thread 1789:", 2345))
	s.OnRecord(journalRec(12, "systemd-coredump", "#0  runtime.sigpanic", 2345))
	out := s.OnRecord(journalRec(13, "systemd", "api-server.service: Failed with result 'core-dump'.", 1))
	if len(out.Spans) != 1 {
		t.Fatalf("expected 1 span on tag change, got %+v", out)
	}
	if out.Spans[0].Kind != domain.SpanSystemdCoredump {
		t.Errorf("Kind = %v, want SpanSystemdCoredump", out.Spans[0].Kind)
	}
	if out.Spans[0].StartSeq != 10 || out.Spans[0].EndSeq != 12 {
		t.Errorf("range = [%d,%d], want [10,12]", out.Spans[0].StartSeq, out.Spans[0].EndSeq)
	}
	if out.Spans[0].PID != 2345 {
		t.Errorf("PID = %d, want 2345", out.Spans[0].PID)
	}
}

// Integration: each journal sample must produce exactly one span of the
// expected kind.
func TestJournalBlocks_DetectOneSpanPerSample(t *testing.T) {
	cases := []struct {
		sample string
		kind   domain.SpanKind
	}{
		{"kernel_panic.log", domain.SpanKernelPanic},
		{"coredump.log", domain.SpanSystemdCoredump},
	}
	for _, tc := range cases {
		t.Run(tc.sample, func(t *testing.T) {
			spans := runJournalSample(t, tc.sample)
			count := 0
			for _, s := range spans {
				if s.Kind == tc.kind {
					count++
				}
			}
			if count != 1 {
				t.Errorf("sample %s: want 1 %v span, got %d", tc.sample, tc.kind, count)
			}
		})
	}
}

// Cross-run guard: journal block analyzers should not wake up on Android
// samples, and the inverse (Android blocks vs journal samples) is
// asserted here as well.
func TestJournalBlocks_NoCrossFormatSpans(t *testing.T) {
	for _, sample := range []string{
		"systemd_unit_failed.log",
		"oom_killer.log",
		"auth_failures.log",
	} {
		spans := runJournalSample(t, sample)
		for _, sp := range spans {
			if sp.Kind == domain.SpanKernelPanic || sp.Kind == domain.SpanSystemdCoredump {
				t.Errorf("sample %s produced unexpected %v span at [%d,%d]", sample, sp.Kind, sp.StartSeq, sp.EndSeq)
			}
		}
	}
}

func runJournalSample(t *testing.T, name string) []domain.Span {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "journalctl", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	anas := []analysis.Analyzer{NewKernelPanic(), NewSystemdCoredump()}
	var spans []domain.Span
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var seq int64
	for scanner.Scan() {
		seq++
		r := pipeline.BuildRecord(domain.Line{Seq: seq, Text: scanner.Text()}, domain.LineFormatJournal)
		for _, a := range anas {
			spans = append(spans, a.OnRecord(r).Spans...)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, a := range anas {
		spans = append(spans, a.Flush().Spans...)
	}
	return spans
}
