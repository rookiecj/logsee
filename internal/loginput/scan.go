package loginput

import (
	"io"
	"strings"
)

// ScanLines reads r and calls emit once per logical line.
//
// Line breaks:
//   - LF (\n): emit accumulated text (may be empty).
//   - CRLF (\r\n): emit text before \r, then start a new line (same as LF for content).
//   - CR alone (\r not followed by \n): discard accumulated text (carriage return to column 0)
//     and continue building from following bytes until LF/CRLF/EOF.
//   - CR at EOF: emit accumulated text (classic Mac one-line file ending with \r).
func ScanLines(r io.Reader, emit func(string) error) error {
	buf := make([]byte, 8192)
	var line []byte
	pendingCR := false

	flush := func() error {
		s := strings.ToValidUTF8(string(line), "\uFFFD")
		line = line[:0]
		return emit(s)
	}

	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			c := buf[i]
			if pendingCR {
				if c == '\n' {
					// CRLF
					pendingCR = false
					if e := flush(); e != nil {
						return e
					}
					continue
				}
				// Lone CR: reset line buffer, then process c
				line = line[:0]
				pendingCR = false
			}
			switch c {
			case '\r':
				pendingCR = true
			case '\n':
				if e := flush(); e != nil {
					return e
				}
			default:
				line = append(line, c)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	if pendingCR {
		// Trailing \r (Mac line terminator or incomplete CRLF): emit remaining as a line
		pendingCR = false
		if err := flush(); err != nil {
			return err
		}
	} else if len(line) > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	return nil
}

// LineStartOffsets returns the byte offset of the first byte of each logical line in r,
// using the same line rules as [ScanLines]. Reads use a 4 KiB buffer.
func LineStartOffsets(r io.Reader) ([]int64, error) {
	buf := make([]byte, 4096)
	var line []byte
	pendingCR := false
	var offsets []int64
	var abs int64 = -1
	curLineStart := int64(0)

	flush := func() {
		offsets = append(offsets, curLineStart)
		line = line[:0]
	}

	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			abs++
			c := buf[i]
			if pendingCR {
				if c == '\n' {
					pendingCR = false
					flush()
					curLineStart = abs + 1
					continue
				}
				line = line[:0]
				pendingCR = false
			}
			switch c {
			case '\r':
				pendingCR = true
			case '\n':
				flush()
				curLineStart = abs + 1
			default:
				if len(line) == 0 {
					curLineStart = abs
				}
				line = append(line, c)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	if pendingCR {
		pendingCR = false
		flush()
	} else if len(line) > 0 {
		flush()
	}
	return offsets, nil
}
