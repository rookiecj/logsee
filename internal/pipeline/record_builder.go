package pipeline

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// adbThreadtimeRE captures `adb logcat -v threadtime` output. The canonical
// shape is "MM-DD HH:MM:SS.sss  PID  TID L TAG: message". TAG is matched
// non-greedily so the first ": " cleanly separates tag from message; the
// trailing whitespace after the colon is optional because some emitters
// pack directly against it.
//
// Groups: 1=date 2=time 3=pid 4=tid 5=level 6=tag 7=message
var adbThreadtimeRE = regexp.MustCompile(
	`^(\d{2}-\d{2})\s+(\d{1,2}:\d{2}:\d{2}\.\d{3})\s+(\d+)\s+(\d+)\s+([VDIWEFST])\s+([^:]+?)\s*:\s?(.*)$`,
)

// RecordBuilder parses raw lines into L1 Records. It is stateless — all
// configuration lives on the struct, not the call — so the same builder
// can be shared across goroutines.
type RecordBuilder struct {
	// RefYear fills in the year for formats that omit it (adb threadtime
	// only prints MM-DD). Zero means time.Now().Year() at call time,
	// which is fine for production but makes tests nondeterministic — set
	// explicitly in tests.
	RefYear int

	// Location applies to parsed timestamps. Nil defaults to time.Local.
	Location *time.Location
}

// DefaultRecordBuilder returns a builder with sane runtime defaults.
func DefaultRecordBuilder() RecordBuilder {
	return RecordBuilder{RefYear: time.Now().Year(), Location: time.Local}
}

// Build parses one Line into a Record. Lines that do not match the format's
// canonical shape yield a minimal Record (Seq + Format + SchemaVer +
// LevelUnknown) so analyzers downstream can decide whether to skip them.
func (b RecordBuilder) Build(line domain.Line, format domain.LineFormat) domain.Record {
	r := domain.Record{
		Seq:       line.Seq,
		Format:    format,
		SchemaVer: domain.SchemaVersion,
	}
	if format != domain.LineFormatAndroid {
		return r
	}
	m := adbThreadtimeRE.FindStringSubmatch(line.Text)
	if m == nil {
		return r
	}
	r.Time = b.parseTime(m[1], m[2])
	r.PID = parseInt32(m[3])
	r.TID = parseInt32(m[4])
	r.Level = domain.LevelFromRaw(m[5])
	r.Tag = strings.TrimSpace(m[6])
	r.Message = m[7]
	return r
}

// BuildRecord is a convenience wrapper around DefaultRecordBuilder().Build.
func BuildRecord(line domain.Line, format domain.LineFormat) domain.Record {
	return DefaultRecordBuilder().Build(line, format)
}

func (b RecordBuilder) parseTime(date, tod string) time.Time {
	loc := b.Location
	if loc == nil {
		loc = time.Local
	}
	year := b.RefYear
	if year == 0 {
		year = time.Now().Year()
	}
	t, err := time.ParseInLocation("2006 01-02 15:04:05.000",
		strconv.Itoa(year)+" "+date+" "+tod, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseInt32(s string) int32 {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}
