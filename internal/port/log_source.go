package port

// LogSnapshot identifies a source incarnation and its readable, published prefix.
type LogSnapshot struct {
	Generation uint64
	ByteLimit  int64
}
