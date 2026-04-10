package tui

import (
	"context"
	"strings"

	"bytemind/internal/llm"
)

type fakeClipboardTextWriter struct {
	last       string
	err        error
	waitForCtx bool
}

func (f *fakeClipboardTextWriter) WriteText(ctx context.Context, text string) error {
	if f.waitForCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	f.last = text
	return nil
}

type compactCommandTestClient struct {
	replies  []llm.Message
	requests []llm.ChatRequest
	index    int
}

func (c *compactCommandTestClient) CreateMessage(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
	c.requests = append(c.requests, req)
	if len(c.replies) == 0 {
		return llm.Message{}, nil
	}
	if c.index >= len(c.replies) {
		return c.replies[len(c.replies)-1], nil
	}
	reply := c.replies[c.index]
	c.index++
	return reply, nil
}

func (c *compactCommandTestClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	reply, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && strings.TrimSpace(reply.Content) != "" {
		onDelta(reply.Content)
	}
	return reply, nil
}
