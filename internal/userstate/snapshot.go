package userstate

// Snapshot is persisted filter/highlight MRU lists and last-applied strings (PRD: 히스토리·세션 복원).
type Snapshot struct {
	Version          int      `json:"version"`
	LastFilter       string   `json:"last_filter"`
	LastHighlight    string   `json:"last_highlight"`
	FilterHistory    []string `json:"filter_history"`
	HighlightHistory []string `json:"highlight_history"`
}

const (
	// SnapshotVersion is bumped when the JSON shape changes.
	SnapshotVersion = 1
	// MaxHistoryEntries caps each MRU list length.
	MaxHistoryEntries = 50
)

// EmptySnapshot returns a zero snapshot with a valid version field.
func EmptySnapshot() Snapshot {
	return Snapshot{Version: SnapshotVersion}
}
