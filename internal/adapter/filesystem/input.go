package filesystem

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

type AppendFile struct {
	path string
	file *os.File
}

func NewAppendFile(path string) (*AppendFile, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open SOT append file %q: %w", path, err)
	}
	return &AppendFile{path: path, file: file}, nil
}

func (f *AppendFile) Path() string {
	return f.path
}

func (f *AppendFile) AppendLine(ctx context.Context, line string) error {
	return f.AppendLines(ctx, []string{line})
}

func (f *AppendFile) AppendLines(_ context.Context, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return f.AppendBytes(context.Background(), []byte(builder.String()))
}
func (f *AppendFile) AppendBytes(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := f.file.Write(data)
	if err != nil {
		return fmt.Errorf("write SOT: %w", err)
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("sync SOT: %w", err)
	}
	return nil
}
func (f *AppendFile) Size(context.Context) (int64, error) {
	info, err := f.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (f *AppendFile) Close() error {
	if f.file == nil {
		return nil
	}
	return f.file.Close()
}

type FileSource struct {
	path         string
	limit        func() int64
	completeOnly bool
}

func NewFileSource(path string) FileSource {
	return FileSource{path: path}
}

func (s FileSource) WithLimit(limit func() int64) FileSource { s.limit = limit; return s }
func (s FileSource) reader(file *os.File) io.Reader {
	if s.limit != nil {
		return io.LimitReader(file, s.limit())
	}
	return file
}
func (s FileSource) Path() string {
	return s.path
}

func (s FileSource) ReadLine(_ context.Context, lineNumber int) (string, error) {
	if lineNumber < 1 {
		return "", fmt.Errorf("line number must be 1 or greater")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return "", fmt.Errorf("open SOT source %q: %w", s.path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(s.reader(file))
	current := 0
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) == 0 {
			if readErr != nil && readErr != io.EOF {
				return "", readErr
			}
			break
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		current++
		if current == lineNumber {
			return line, nil
		}
	}

	return "", fmt.Errorf("line %d not found in SOT source %q", lineNumber, s.path)
}

func (s FileSource) SampleLines(ctx context.Context, maxNonEmpty int) ([]string, error) {
	if maxNonEmpty < 1 {
		return nil, fmt.Errorf("max non-empty sample lines must be 1 or greater")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open SOT source %q: %w", s.path, err)
	}
	defer file.Close()

	reader := bufio.NewReader(s.reader(file))
	lines := []string{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) == 0 {
			if readErr != nil && readErr != io.EOF {
				return nil, readErr
			}
			break
		}
		if s.completeOnly && !strings.HasSuffix(line, "\n") {
			break
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) == maxNonEmpty {
			break
		}
	}

	return lines, nil
}

func (s FileSource) Size(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, fmt.Errorf("stat SOT source %q: %w", s.path, err)
	}
	if s.limit != nil {
		return min(info.Size(), s.limit()), nil
	}
	return info.Size(), nil
}

func (s FileSource) ReadAt(ctx context.Context, offset int64, size int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be 0 or greater")
	}
	if size < 0 {
		return nil, fmt.Errorf("size must be 0 or greater")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open SOT source %q: %w", s.path, err)
	}
	defer file.Close()

	if s.limit != nil {
		size = int(min(int64(size), max(0, s.limit()-offset)))
	}
	buffer := make([]byte, size)
	n, err := file.ReadAt(buffer, offset)
	if canceled := ctx.Err(); canceled != nil {
		return nil, canceled
	}
	if err != nil && n == 0 {
		return nil, fmt.Errorf("read SOT source %q at offset %d: %w", s.path, offset, err)
	}
	return buffer[:n], nil
}

func (s FileSource) SampleCompleteLines(ctx context.Context, max int) ([]string, error) {
	s.completeOnly = true
	return s.SampleLines(ctx, max)
}
