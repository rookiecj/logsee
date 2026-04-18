package ui

import (
	"fmt"
	"os"
	"strings"

	"git.inpt.fr/42dottools/log/internal/buffer"
	"git.inpt.fr/42dottools/log/internal/config"
	"git.inpt.fr/42dottools/log/internal/filter"
	"git.inpt.fr/42dottools/log/internal/storage"
	"git.inpt.fr/42dottools/log/internal/userstate"
)

const (
	// topChromeLines: filter strip (high visibility) + highlight/hints row (same chrome pattern as filter).
	topChromeLines = 2
	// bottomChromeLines: status bar (in/out/follow/wrap/lines/…).
	bottomChromeLines = 1
)

// LineMsg carries one stdin line (without trailing newline).
type LineMsg string

// StdinClosedMsg signals EOF on the log input stream (stdin or file).
type StdinClosedMsg struct{}

// LineBatchMsg carries multiple log lines read within one stdin flush window.
type LineBatchMsg []string

// Model is the bubbletea root model.
//
// Filter × search stacking (PRD §8.0): there is no separate stack type. Order is fixed in code:
// Layer 1 — filteredIndices() uses prog on the ring; empty prog keeps every line (기본 탐색 모드).
// Layer 2 — searchBuf is the committed highlight query; searchDraft is edited only while searchCompose (PRD §6.5).
// Highlight uses searchBuf; next/prev match navigation uses n/p on log list (remapped to Ctrl+n/Ctrl+p) and Ctrl+n/Ctrl+p in tryBrowseKey (PRD §8.4), forward/backward only (no wrap).
// List normal: printable keys do not edit search; first '/' opens searchCompose; second Enter commits; Esc clears list selection (Shift range + Space picks) first, then cancels compose (PRD §6.5).
// Clipboard: list focus, not search compose — `c` copies the union of Shift range (if any) and Space picks (if any), sorted by filtered index; if neither applies, cursor line (PRD §8.6 + range∪pick merge). In search compose, `c` edits the draft.
//
// Focus & Esc (PRD §6.0·§6.0.1): keyFocus() is FocusLogList | FocusFilterEditor | FocusHighlightEditor; see focus.go.
// Enter or ':' (from list) → FocusFilterEditor; Esc calls popFilterInputFocus() in filter editor, else list/highlight Esc (PRD §6.5–§6.6).
type Model struct {
	buf *buffer.Ring

	appliedFilter string
	filterDraft   string
	filterEdit    bool
	filterCursor  int // rune offset into filterDraft while filterEdit (0..len runes)
	filterErr     string
	prog          filter.Program

	searchBuf     string // committed highlight query; drives Highlight + match nav when non-empty
	searchDraft   string // highlight query being edited (only while searchCompose)
	searchCompose bool   // list focus: true after '/' opens highlight entry; Enter commits, Esc reverts and exits
	searchCaret   int    // rune index into searchDraft for compose editing (0..len(runes))

	ignoreCase    bool
	noLineNumbers bool
	width, height int
	// scrollTop is the first visible index into fidx when len(fidx) > viewportH; ignored when short list (bottom-padded).
	// Derived from viewTopSeq/cursorSeq at render time (see Phase 1 of docs/plans/seq-coord-pull-window-plan.md).
	scrollTop int
	// cursorIdx is the focused line index into fidx (always kept on-screen; viewport prefers placing it on the last row).
	// Derived from cursorSeq at render time.
	cursorIdx int
	// cursorSeq / viewTopSeq: primary view anchors in file absolute coord (1-based Seq).
	// Survive Ring.ReplaceRecords so cursor screen row is stable across async window loads (file partial mode).
	// 0 means unset (stdin mode uses idx-based flow; seq values mirror ring content anyway).
	cursorSeq  int64
	viewTopSeq int64
	colRuneOff int
	// lineWrap: long lines split into multiple terminal rows (see PRD §5·§6). When false, horizontal scroll uses colRuneOff.
	lineWrap bool
	// scrollSegTop is the first visible wrap segment index when lineWrap && visual rows exceed the viewport; ignored when !lineWrap.
	scrollSegTop int
	follow       bool
	stdinClosed  bool
	store        *storage.LineAppender
	outPath      string
	inputSource  string // "stdin" or path shown in status (log input)
	version      string // build label for help dialog (from internal/version in cmd)

	// filePartial: input-file mode loads a sliding window; Seq = 1-based file line number.
	filePartial    bool
	filePath       string
	fileOffsets    []int64
	fileTotalLines int
	fileSizeBytes  int64
	fileWinFirst   int64
	// windowProvider: Phase 2 abstraction over random-access disk reads. Set by applyFileIndexReady
	// in production; tests that seed fileOffsets directly fall back via windowProviderOrFallback().
	windowProvider WindowProvider
	// searchScanConfirmOpen: modal gate when lazy n/p scan exceeds 100 MiB.
	searchScanConfirmOpen bool
	searchScanResumeSeq   int64
	searchScanDir         int
	searchScanScanned     int64
	// filterTopupActive: file-partial mode keeps scanning forward after filter apply until viewport can be filled or EOF.
	filterTopupActive bool
	// filterTopupDir: +1 forward(from window end), -1 backward(from window start)
	filterTopupDir int
	// filterTopupNavAdvance: +1/-1 marks a j/k boundary-triggered filter scan whose intent is to
	// advance the cursor to the next / previous filter match (vs a viewport-fill top-up). Cleared
	// once the scan delivers a match or reaches EOF.
	filterTopupNavAdvance int

	// helpOpen: F1 help dialog; Esc or F1 closes (PRD §6.1).
	helpOpen bool
	// helpFilterSyntax: when helpOpen, show filter-syntax dialog instead of full keymap (set when F1 from filter input, PRD §5·§6.4).
	helpFilterSyntax bool

	// Line selection: indices into filtered list. selAnchor < 0 means no active Shift range (선택영역 설정).
	selAnchor int
	// picked: Space toggled lines (filtered index -> struct{}). Independent of selAnchor; cleared with Esc / `/` / filter apply, etc. (PRD §8.6).
	picked    map[int]struct{}
	copyFlash string

	// filterHistory / highlightHistory: MRU strings (PRD §6.4.1). stateFile: full path to state.json; empty disables persistence.
	filterHistory    []string
	highlightHistory []string
	stateFile        string

	histKind  histOverlayKind
	histSel   int
	histItems []string

	// bookmarkSlot: PRD §6.7 — fixed slots 1..9 map to record Seq; 0 means empty (Seq starts at 1 in ring).
	bookmarkSlot [9]int64
	// bookmarkRotateNext: when all 9 slots are full, next `m` replaces this slot (1..9), then advances 1→2→…→9→1.
	bookmarkRotateNext int

	// Log type (level extraction shape): --log-type / --log-type-probe-lines (PRD §7.1).
	logTypeKind       LogTypeKind
	logProbeLines     int
	effectiveLogFmt   filter.LogFormat
	logFormatResolved bool // false for LogTypeAuto until probe or stdin EOF

	// highlightNames: merged built-in + config highlight_color_names; drives ParseHighlightNeedles.
	highlightNames  map[string]string
	hlNeedlesKey    string
	hlNeedlesCached []HighlightNeedle
}

// HistoryOpts configures persisted filter/highlight MRU (PRD §3·§6.4.1). StateFile is full path to state.json; empty disables disk load/save.
type HistoryOpts struct {
	StateFile string
	Initial   userstate.Snapshot
}

func capSnapshotHistory(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	n := len(in)
	if n > userstate.MaxHistoryEntries {
		n = userstate.MaxHistoryEntries
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, in[i])
	}
	return out
}

// NewModel constructs UI state. outPath is the output file path for status. inputSource is "stdin" or the input file path for status. version is a one-line build label for the F1 help dialog. hist may be nil (no persistence, empty MRU). logType may be nil (DefaultLogTypeOpts: auto, 32 probe lines). highlightColorNames overrides built-in ANSI name→index entries (may be nil).
func NewModel(buf *buffer.Ring, store *storage.LineAppender, ignoreCase, noLineNumbers bool, outPath, inputSource, version string, hist *HistoryOpts, logType *LogTypeOpts, highlightColorNames map[string]string) *Model {
	if inputSource == "" {
		inputSource = "stdin"
	}
	if version == "" {
		version = "unknown"
	}
	applied := ""
	hl := ""
	var fh, hh []string
	stateFile := ""
	if hist != nil {
		stateFile = hist.StateFile
		snap := hist.Initial
		applied = strings.TrimSpace(snap.LastFilter)
		hl = strings.TrimSpace(snap.LastHighlight)
		fh = capSnapshotHistory(snap.FilterHistory)
		hh = capSnapshotHistory(snap.HighlightHistory)
	}
	prog, err := filter.Parse(applied)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logsee: restored filter invalid, ignoring: %v\n", err)
		applied = ""
		prog = filter.Program{}
	}
	lt := DefaultLogTypeOpts()
	if logType != nil {
		lt = *logType
		if lt.ProbeLines < 1 {
			lt.ProbeLines = 32
		}
	}
	effFmt, effResolved := initialLogFormat(lt.Kind)
	return &Model{
		buf:               buf,
		follow:            true,
		store:             store,
		ignoreCase:        ignoreCase,
		noLineNumbers:     noLineNumbers,
		outPath:           outPath,
		inputSource:       inputSource,
		version:           version,
		selAnchor:         -1,
		picked:            make(map[int]struct{}),
		appliedFilter:     applied,
		filterDraft:       applied,
		prog:              prog,
		searchBuf:         hl,
		searchDraft:       hl,
		filterHistory:     fh,
		highlightHistory:  hh,
		stateFile:         stateFile,
		logTypeKind:       lt.Kind,
		logProbeLines:     lt.ProbeLines,
		effectiveLogFmt:   effFmt,
		logFormatResolved: effResolved,
		highlightNames:    config.MergeHighlightColorNames(highlightColorNames),
	}
}

// SetWindowProvider installs the random-access provider for the current input source. Called
// once at startup for the stdin path (with a [RingStreamProvider]); the file path installs its
// own [FileSliceProvider] via [Model.applyFileIndexReady] after offsets are indexed.
func (m *Model) SetWindowProvider(p WindowProvider) {
	m.windowProvider = p
}

func (m *Model) persistState() {
	if m.stateFile == "" {
		return
	}
	if len(m.filterHistory) > userstate.MaxHistoryEntries {
		m.filterHistory = m.filterHistory[:userstate.MaxHistoryEntries]
	}
	if len(m.highlightHistory) > userstate.MaxHistoryEntries {
		m.highlightHistory = m.highlightHistory[:userstate.MaxHistoryEntries]
	}
	s := userstate.Snapshot{
		Version:          userstate.SnapshotVersion,
		LastFilter:       strings.TrimSpace(m.appliedFilter),
		LastHighlight:    strings.TrimSpace(m.searchBuf),
		FilterHistory:    append([]string(nil), m.filterHistory...),
		HighlightHistory: append([]string(nil), m.highlightHistory...),
	}
	if err := userstate.Save(m.stateFile, s); err != nil {
		fmt.Fprintf(os.Stderr, "logsee: save state %q: %v\n", m.stateFile, err)
	}
}
