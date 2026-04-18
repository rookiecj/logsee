package domain

// Record is one logical line of log input with a monotonic sequence id.
type Record struct {
	Seq  int64
	Text string
}
