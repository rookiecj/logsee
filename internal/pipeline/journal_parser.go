package pipeline

import (
	"regexp"
	"time"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// journalShortISORE matches `journalctl -o short-iso` and
// `short-iso-precise` output:
//
//	2024-04-19T14:24:10[.123456](+0900|Z) hostname tag[pid]: message
//
// The hostname is captured but currently discarded (not represented in
// domain.Record); left as a group so later enrichment can use it without
// touching the regex.
//
// Groups: 1=timestamp 2=hostname 3=tag 4=pid (optional) 5=message
var journalShortISORE = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d{4}|Z))\s+(\S+)\s+([^\s\[]+)(?:\[(\d+)\])?:\s?(.*)$`,
)

var journalTimeLayouts = []string{
	"2006-01-02T15:04:05.000000-0700",
	"2006-01-02T15:04:05.000-0700",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05.000000Z07:00",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000000Z",
}

// buildJournalRecord parses one journal short-iso line into a Record. Lines
// that do not match the pattern return a minimal Record (Seq + Format +
// SchemaVer, LevelUnknown) so later analyzers can ignore them.
//
// Level is left as LevelUnknown — `short-iso` carries no PRIORITY field;
// the JSON format (out of scope for v1) would populate it.
func buildJournalRecord(line domain.Line) domain.Record {
	r := domain.Record{
		Seq:       line.Seq,
		Format:    domain.LineFormatJournal,
		SchemaVer: domain.SchemaVersion,
	}
	m := journalShortISORE.FindStringSubmatch(line.Text)
	if m == nil {
		return r
	}
	r.Time = parseJournalTime(m[1])
	r.Tag = m[3]
	if m[4] != "" {
		r.PID = parseInt32(m[4])
	}
	r.Message = m[5]
	return r
}

// parseJournalTime tries the layout set and returns the zero time on
// failure — the returned Record carries a zero time the caller can detect
// via `.Time.IsZero()` if strict parsing matters.
func parseJournalTime(s string) time.Time {
	for _, layout := range journalTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	// Attempt an ISO variant with colon in the timezone (`+09:00`), which
	// Go recognizes via the "Z07:00" specifier.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
