package tui

import (
	"context"

	"github.com/atotto/clipboard"
)

type clipboardTextWriter interface {
	WriteText(ctx context.Context, text string) error
}

type defaultClipboardTextWriter struct{}

func (defaultClipboardTextWriter) WriteText(ctx context.Context, text string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return clipboard.WriteAll(text)
	}
}
