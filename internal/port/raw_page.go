package port

import "context"

type RawPageSource interface {
	Path() string
	Size(context.Context) (int64, error)
	ReadAt(context.Context, int64, int) ([]byte, error)
}
