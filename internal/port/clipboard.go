package port

import "context"

type ClipboardWriter interface {
	WriteText(context.Context, string) error
}
