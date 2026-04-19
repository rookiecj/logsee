package fileindex

import (
	"errors"
	"io"
	"os"
	"strings"

	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/loginput"
)

var errEnoughLines = errors.New("enough lines")

// ReadFirstNLines reads up to n logical lines from the start of path (same rules as [loginput.ScanLines]).
func ReadFirstNLines(path string, n int) ([]string, error) {
	if n < 1 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	err = loginput.ScanLines(f, func(s string) error {
		out = append(out, s)
		if len(out) >= n {
			return errEnoughLines
		}
		return nil
	})
	if err != nil && err != errEnoughLines {
		return nil, err
	}
	return out, nil
}

// BuildLineStartOffsets returns byte offsets of each logical line (see [loginput.LineStartOffsets]).
func BuildLineStartOffsets(path string) ([]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return loginput.LineStartOffsets(f)
}

// ReadWindowRecords reads logical lines firstLine1..lastLine1 inclusive (1-based line numbers).
// offsets must be from [loginput.LineStartOffsets]; len(offsets) is the total line count.
func ReadWindowRecords(path string, offsets []int64, firstLine1, lastLine1 int) ([]domain.Line, error) {
	if len(offsets) == 0 || firstLine1 < 1 || lastLine1 < firstLine1 {
		return nil, nil
	}
	total := len(offsets)
	if firstLine1 > total {
		return nil, nil
	}
	if lastLine1 > total {
		lastLine1 = total
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	startByte := offsets[firstLine1-1]
	if _, err := f.Seek(startByte, io.SeekStart); err != nil {
		return nil, err
	}
	nWant := lastLine1 - firstLine1 + 1
	var recs []domain.Line
	seq := int64(firstLine1)
	err = loginput.ScanLines(f, func(s string) error {
		recs = append(recs, domain.Line{Seq: seq, Text: strings.ToValidUTF8(s, "\uFFFD")})
		seq++
		if len(recs) >= nWant {
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}
	return recs, nil
}
