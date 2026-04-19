package domain

// LineFormat hints the shape of a raw line, copied onto Record so analyzers
// can cheaply gate rules (e.g. Android-only regex) without re-sniffing.
// The enum mirrors filter.LogFormat but is defined here to keep
// internal/domain dependency-free.
type LineFormat uint8

const (
	LineFormatUnknown LineFormat = iota
	LineFormatPlain
	LineFormatBracket
	LineFormatAndroid
)

var lineFormatNames = [...]string{
	LineFormatUnknown: "unknown",
	LineFormatPlain:   "plain",
	LineFormatBracket: "bracket",
	LineFormatAndroid: "android",
}

// String returns the stable snake_case name used on the wire.
func (f LineFormat) String() string {
	if int(f) >= len(lineFormatNames) {
		return "unknown"
	}
	return lineFormatNames[f]
}

// MarshalText writes the snake_case name.
func (f LineFormat) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

// UnmarshalText parses the name produced by MarshalText. Unknown names
// decode to LineFormatUnknown rather than erroring so schema evolution
// does not break readers.
func (f *LineFormat) UnmarshalText(b []byte) error {
	switch string(b) {
	case "plain":
		*f = LineFormatPlain
	case "bracket":
		*f = LineFormatBracket
	case "android":
		*f = LineFormatAndroid
	default:
		*f = LineFormatUnknown
	}
	return nil
}
