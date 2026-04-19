package domain

// Seq is the 1-based monotonic cross-layer key that ties L0 lines, L1
// records, L2 spans, and L3 findings together. It is declared as an alias
// for int64 so existing call-sites continue to compile; a follow-up may
// promote it to a distinct named type once all consumers use the alias.
type Seq = int64
