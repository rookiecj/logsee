package domain

// SpanKind classifies a multi-line log event that block analyzers aggregate
// into a single logical unit (e.g. a native tombstone, a Java FATAL with
// its Caused-by chain, an ANR with its CPU-usage tail).
type SpanKind uint16

const (
	SpanUnknown SpanKind = iota
	SpanNativeCrash
	SpanJavaFatal
	SpanANR
	SpanWatchdog
	SpanGCStorm
)

var spanKindNames = [...]string{
	SpanUnknown:     "unknown",
	SpanNativeCrash: "native_crash",
	SpanJavaFatal:   "java_fatal",
	SpanANR:         "anr",
	SpanWatchdog:    "watchdog",
	SpanGCStorm:     "gc_storm",
}

func (k SpanKind) String() string {
	if int(k) >= len(spanKindNames) {
		return "unknown"
	}
	return spanKindNames[k]
}

func (k SpanKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

func (k *SpanKind) UnmarshalText(b []byte) error {
	switch string(b) {
	case "native_crash":
		*k = SpanNativeCrash
	case "java_fatal":
		*k = SpanJavaFatal
	case "anr":
		*k = SpanANR
	case "watchdog":
		*k = SpanWatchdog
	case "gc_storm":
		*k = SpanGCStorm
	default:
		*k = SpanUnknown
	}
	return nil
}

// Span is a half-open range [StartSeq, EndSeq] over L0 lines that belong
// to the same logical event. EndSeq is inclusive because emitter typically
// knows the last line of the block at emit time (e.g. the final backtrace
// frame). PID is 0 when the span is process-agnostic.
type Span struct {
	ID        int64    `json:"id"`
	Kind      SpanKind `json:"kind"`
	StartSeq  Seq      `json:"start_seq"`
	EndSeq    Seq      `json:"end_seq"`
	PID       int32    `json:"pid,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	SchemaVer uint16   `json:"schema_version,omitempty"`
}
