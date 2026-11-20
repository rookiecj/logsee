package port

import "context"

type LineAppender interface {
	Path() string
	AppendLine(context.Context, string) error
	AppendLines(context.Context, []string) error
}

type LineAppendWorker interface {
	Path() string
	AppendLine(context.Context, string) error
	AppendLines(context.Context, []string) error
}

type FileSource interface {
	Path() string
	ReadLine(context.Context, int) (string, error)
	SampleLines(context.Context, int) ([]string, error)
}

type SourceObserver interface {
	SourceLineAvailable(line string)
}

// AppendBytes must complete its durability barrier before returning success.
type ByteAppender interface {
	AppendBytes(context.Context, []byte) error
	Size(context.Context) (int64, error)
}
type SourceProgress struct {
	PublishedBytes int64
	CompleteLines  int
	EOF            bool
}
type ProgressObserver interface{ SourceProgressAvailable(SourceProgress) }

type SelectedLineSource interface {
	WalkLines(context.Context, int, int, func(int, string) error) error
}
