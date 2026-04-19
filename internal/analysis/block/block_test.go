package block

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/pipeline"
)

func rec(seq int64, tag, msg string, pid int32, level domain.Level) domain.Record {
	return domain.Record{Seq: seq, Tag: tag, Message: msg, PID: pid, Level: level, Format: domain.LineFormatAndroid}
}

// --- NativeCrash ---

func TestNativeCrash_StartsAtTripleStarHeader(t *testing.T) {
	n := NewNativeCrash()
	out := n.OnRecord(rec(1, "DEBUG", "*** *** *** *** *** ***", 4567, domain.LevelFatal))
	if !out.Empty() {
		t.Errorf("first record should not emit, got %+v", out)
	}
	if !n.active {
		t.Error("should be active after header")
	}
}

func TestNativeCrash_EmitsOnTagChange(t *testing.T) {
	n := NewNativeCrash()
	n.OnRecord(rec(1, "DEBUG", "*** *** *** *** *** ***", 4567, domain.LevelFatal))
	n.OnRecord(rec(2, "DEBUG", "Build fingerprint: 'x'", 4567, domain.LevelFatal))
	n.OnRecord(rec(3, "DEBUG", ">>> com.example.crasher <<<", 4567, domain.LevelFatal))
	n.OnRecord(rec(4, "DEBUG", "signal 11 (SIGSEGV), fault addr 0x0", 4567, domain.LevelFatal))
	n.OnRecord(rec(5, "DEBUG", "backtrace:", 4567, domain.LevelFatal))
	n.OnRecord(rec(6, "DEBUG", "  #00 pc 0x123 libc.so", 4567, domain.LevelFatal))
	out := n.OnRecord(rec(7, "ActivityManager", "Process died", 1245, domain.LevelInfo))
	if len(out.Spans) != 1 {
		t.Fatalf("expected 1 span on tag change, got %+v", out)
	}
	s := out.Spans[0]
	if s.Kind != domain.SpanNativeCrash {
		t.Errorf("Kind = %v, want SpanNativeCrash", s.Kind)
	}
	if s.StartSeq != 1 || s.EndSeq != 6 {
		t.Errorf("Span range = [%d,%d], want [1,6]", s.StartSeq, s.EndSeq)
	}
	if s.PID != 4567 {
		t.Errorf("PID = %d, want 4567", s.PID)
	}
	if s.Summary == "" {
		t.Error("Summary should be non-empty")
	}
	if n.active {
		t.Error("should be inactive after emit")
	}
}

func TestNativeCrash_FlushEmitsActiveBlock(t *testing.T) {
	n := NewNativeCrash()
	n.OnRecord(rec(1, "DEBUG", "*** *** *** *** *** ***", 99, domain.LevelFatal))
	n.OnRecord(rec(2, "DEBUG", "some backtrace", 99, domain.LevelFatal))
	out := n.Flush()
	if len(out.Spans) != 1 {
		t.Fatalf("Flush should emit the open block, got %+v", out)
	}
	if out.Spans[0].StartSeq != 1 || out.Spans[0].EndSeq != 2 {
		t.Errorf("unexpected range: %+v", out.Spans[0])
	}
}

func TestNativeCrash_IgnoresUnrelatedLinesBeforeHeader(t *testing.T) {
	n := NewNativeCrash()
	for i := int64(1); i <= 5; i++ {
		out := n.OnRecord(rec(i, "ActivityManager", "normal traffic", 1245, domain.LevelInfo))
		if !out.Empty() {
			t.Errorf("pre-header line %d produced output: %+v", i, out)
		}
	}
}

// --- JavaFatal ---

func TestJavaFatal_StartAndEmit(t *testing.T) {
	j := NewJavaFatal()
	j.OnRecord(rec(10, "AndroidRuntime", "*** FATAL EXCEPTION IN SYSTEM PROCESS: Binder:1245_A", 1245, domain.LevelError))
	j.OnRecord(rec(11, "AndroidRuntime", "java.lang.NullPointerException: Attempt to invoke virtual method", 1245, domain.LevelError))
	j.OnRecord(rec(12, "AndroidRuntime", "\tat com.android.server.am.ActivityManagerService.doStuff(AMS.java:42)", 1245, domain.LevelError))
	j.OnRecord(rec(13, "AndroidRuntime", "Caused by: java.lang.IllegalStateException: oops", 1245, domain.LevelError))
	out := j.OnRecord(rec(14, "Process", "Sending signal. PID: 1245 SIG: 9", 1245, domain.LevelInfo))
	if len(out.Spans) != 1 {
		t.Fatalf("expected 1 span, got %+v", out)
	}
	s := out.Spans[0]
	if s.Kind != domain.SpanJavaFatal {
		t.Errorf("Kind = %v, want SpanJavaFatal", s.Kind)
	}
	if s.StartSeq != 10 || s.EndSeq != 13 {
		t.Errorf("range = [%d,%d], want [10,13]", s.StartSeq, s.EndSeq)
	}
	if s.Summary == "" {
		t.Error("Summary must be non-empty")
	}
}

func TestJavaFatal_AcceptsPlainHeaderForm(t *testing.T) {
	j := NewJavaFatal()
	j.OnRecord(rec(1, "AndroidRuntime", "FATAL EXCEPTION: main", 100, domain.LevelError))
	if !j.active {
		t.Error("plain FATAL EXCEPTION should start the block")
	}
}

func TestJavaFatal_Flush(t *testing.T) {
	j := NewJavaFatal()
	j.OnRecord(rec(1, "AndroidRuntime", "FATAL EXCEPTION: main", 100, domain.LevelError))
	j.OnRecord(rec(2, "AndroidRuntime", "java.lang.RuntimeException: boom", 100, domain.LevelError))
	out := j.Flush()
	if len(out.Spans) != 1 {
		t.Fatalf("Flush should emit open block, got %+v", out)
	}
}

func TestJavaFatal_IgnoresNonAndroidRuntime(t *testing.T) {
	j := NewJavaFatal()
	out := j.OnRecord(rec(1, "System", "FATAL EXCEPTION: nope", 100, domain.LevelError))
	if !out.Empty() || j.active {
		t.Error("different tag must not start the block")
	}
}

// --- ANR ---

func TestANR_StartsAtHeader(t *testing.T) {
	a := NewANR()
	out := a.OnRecord(rec(1, "ActivityManager", "ANR in com.example.app (com.example.app/.MainActivity)", 1245, domain.LevelError))
	if !out.Empty() {
		t.Errorf("first record should not emit: %+v", out)
	}
	if !a.active {
		t.Error("should be active after ANR header")
	}
}

func TestANR_EndsOnTotalLineInclusive(t *testing.T) {
	a := NewANR()
	a.OnRecord(rec(1, "ActivityManager", "ANR in com.example.app", 1245, domain.LevelError))
	a.OnRecord(rec(2, "ActivityManager", "PID: 12345", 1245, domain.LevelError))
	a.OnRecord(rec(3, "ActivityManager", "Reason: Input dispatching timed out", 1245, domain.LevelError))
	a.OnRecord(rec(4, "ActivityManager", "45% 12345/com.example.app: 40% user + 5% kernel", 1245, domain.LevelError))
	out := a.OnRecord(rec(5, "ActivityManager", "62% TOTAL: 45% user + 12% kernel + 2% iowait + 3% irq", 1245, domain.LevelError))
	if len(out.Spans) != 1 {
		t.Fatalf("TOTAL line should close block, got %+v", out)
	}
	if out.Spans[0].StartSeq != 1 || out.Spans[0].EndSeq != 5 {
		t.Errorf("range = [%d,%d], want [1,5]", out.Spans[0].StartSeq, out.Spans[0].EndSeq)
	}
	if a.active {
		t.Error("should be inactive after emit")
	}
}

func TestANR_EndsOnTagChange(t *testing.T) {
	a := NewANR()
	a.OnRecord(rec(1, "ActivityManager", "ANR in com.example.app", 1245, domain.LevelError))
	a.OnRecord(rec(2, "ActivityManager", "PID: 12345", 1245, domain.LevelError))
	out := a.OnRecord(rec(3, "DropBoxManagerService", "add tag=data_app_anr", 1245, domain.LevelInfo))
	if len(out.Spans) != 1 {
		t.Fatalf("tag change should close block, got %+v", out)
	}
	if out.Spans[0].EndSeq != 2 {
		t.Errorf("EndSeq = %d, want 2 (last AM record)", out.Spans[0].EndSeq)
	}
}

func TestANR_Flush(t *testing.T) {
	a := NewANR()
	a.OnRecord(rec(1, "ActivityManager", "ANR in com.example.app", 1245, domain.LevelError))
	out := a.Flush()
	if len(out.Spans) != 1 {
		t.Fatalf("Flush should emit open block, got %+v", out)
	}
}

// --- Integration: run all analyzers over each sample and verify exactly one
// span of the expected kind shows up.

type analyzerFactory func() analysis.Analyzer

func TestBlocks_DetectOneSpanPerSample(t *testing.T) {
	cases := []struct {
		sample    string
		wantKind  domain.SpanKind
		factories []analyzerFactory
	}{
		{
			sample:   "anr_input_dispatch.log",
			wantKind: domain.SpanANR,
			factories: []analyzerFactory{
				func() analysis.Analyzer { return NewANR() },
				func() analysis.Analyzer { return NewJavaFatal() },
				func() analysis.Analyzer { return NewNativeCrash() },
			},
		},
		{
			sample:   "native_tombstone.log",
			wantKind: domain.SpanNativeCrash,
			factories: []analyzerFactory{
				func() analysis.Analyzer { return NewANR() },
				func() analysis.Analyzer { return NewJavaFatal() },
				func() analysis.Analyzer { return NewNativeCrash() },
			},
		},
		{
			sample:   "java_fatal_system_server.log",
			wantKind: domain.SpanJavaFatal,
			factories: []analyzerFactory{
				func() analysis.Analyzer { return NewANR() },
				func() analysis.Analyzer { return NewJavaFatal() },
				func() analysis.Analyzer { return NewNativeCrash() },
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.sample, func(t *testing.T) {
			spans := runSampleThroughAnalyzers(t, tc.sample, tc.factories)
			count := 0
			for _, s := range spans {
				if s.Kind == tc.wantKind {
					count++
				}
			}
			if count != 1 {
				t.Errorf("sample %s: want exactly 1 %v span, got %d (all=%v)", tc.sample, tc.wantKind, count, spans)
			}
		})
	}
}

func TestBlocks_SpanRangeNonEmpty(t *testing.T) {
	spans := runSampleThroughAnalyzers(t, "anr_input_dispatch.log", []analyzerFactory{
		func() analysis.Analyzer { return NewANR() },
	})
	if len(spans) == 0 {
		t.Fatal("expected at least one ANR span")
	}
	s := spans[0]
	if s.EndSeq < s.StartSeq {
		t.Errorf("EndSeq (%d) must be >= StartSeq (%d)", s.EndSeq, s.StartSeq)
	}
	if s.Summary == "" {
		t.Error("Summary must be populated")
	}
}

func runSampleThroughAnalyzers(t *testing.T, sample string, factories []analyzerFactory) []domain.Span {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "android", sample)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	anas := make([]analysis.Analyzer, len(factories))
	for i, mk := range factories {
		anas[i] = mk()
	}

	builder := pipeline.RecordBuilder{RefYear: 2024, Location: time.UTC}
	var spans []domain.Span

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var seq int64
	for scanner.Scan() {
		seq++
		r := builder.Build(domain.Line{Seq: seq, Text: scanner.Text()}, domain.LineFormatAndroid)
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
