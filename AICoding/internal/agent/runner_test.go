package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"aicoding/internal/config"
	"aicoding/internal/llm"
	"aicoding/internal/session"
	"aicoding/internal/tools"
)

type fakeClient struct {
	replies []llm.Message
	index   int
}

func (f *fakeClient) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	if len(f.replies) == 0 {
		return llm.Message{}, nil
	}
	if f.index >= len(f.replies) {
		return f.replies[len(f.replies)-1], nil
	}
	message := f.replies[f.index]
	f.index++
	return message, nil
}

func (f *fakeClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := f.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && message.Content != "" {
		onDelta(message.Content)
	}
	return message, nil
}

func TestRunPromptReturnsBudgetSummaryInsteadOfError(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 1,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "Paused before a final answer.") {
		t.Fatalf("expected pause summary, got %q", answer)
	}
	if !strings.Contains(answer, "execution budget of 1 turns") {
		t.Fatalf("expected budget detail, got %q", answer)
	}
}

func TestRunPromptStopsOnRepeatedToolPlan(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	repeatedReply := llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			ID:   "call-repeat",
			Type: "function",
			Function: llm.ToolFunctionCall{
				Name:      "list_files",
				Arguments: `{}`,
			},
		}},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 5,
			Stream:        false,
		},
		Client:   &fakeClient{replies: []llm.Message{repeatedReply, repeatedReply, repeatedReply, repeatedReply}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "looping task", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "repeated the same tool plan") {
		t.Fatalf("expected repeat-detection summary, got %q", answer)
	}
}
