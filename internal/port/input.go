package port

import "context"

type LineAppender interface {
	Path() string
	AppendLine(context.Context, string) error
}

type LineAppendWorker interface {
	Path() string
	AppendLine(context.Context, string) error
}

type FileSource interface {
	Path() string
	ReadLine(context.Context, int) (string, error)
	SampleLines(context.Context, int) ([]string, error)
}

type SourceObserver interface {
	SourceLineAvailable(line string)
}
