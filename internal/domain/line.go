package domain

// Line is one logical unit of log input with a monotonic sequence id.
// It is the L0 raw-line type; parsed per-field data (level, pid, tag, …)
// lives on the L1 Record type introduced by the analysis pipeline.
type Line struct {
	Seq  int64
	Text string
}
