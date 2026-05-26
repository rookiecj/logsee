package filesystem

import (
	"bufio"
	"context"
	"fmt"
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
	if _, err := f.file.WriteString(builder.String()); err != nil {
		return fmt.Errorf("write lines: %w", err)
	}
	return nil
}

func (f *AppendFile) Close() error {
	if f.file == nil {
		return nil
	}
	return f.file.Close()
}

type FileSource struct {
	path string
}

func NewFileSource(path string) FileSource {
	return FileSource{path: path}
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

	scanner := bufio.NewScanner(file)
	current := 0
	for scanner.Scan() {
		current++
		if current == lineNumber {
			return scanner.Text(), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read SOT source %q: %w", s.path, err)
	}
	return "", fmt.Errorf("line %d not found in SOT source %q", lineNumber, s.path)
}

func (s FileSource) SampleLines(_ context.Context, maxNonEmpty int) ([]string, error) {
	if maxNonEmpty < 1 {
		return nil, fmt.Errorf("max non-empty sample lines must be 1 or greater")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open SOT source %q: %w", s.path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) == maxNonEmpty {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SOT source %q: %w", s.path, err)
	}
	return lines, nil
}

func (s FileSource) Size(_ context.Context) (int64, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0, fmt.Errorf("stat SOT source %q: %w", s.path, err)
	}
	return info.Size(), nil
}

func (s FileSource) ReadAt(_ context.Context, offset int64, size int) ([]byte, error) {
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

	buffer := make([]byte, size)
	n, err := file.ReadAt(buffer, offset)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("read SOT source %q at offset %d: %w", s.path, offset, err)
	}
	return buffer[:n], nil
}
