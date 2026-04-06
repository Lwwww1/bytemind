package history

import (
	"os"
	"testing"
	"time"
)

func TestAppendPromptAndLoadRecentPrompts(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	if err := AppendPrompt("/repo", "sess-1", "first prompt", now); err != nil {
		t.Fatalf("append first prompt: %v", err)
	}
	if err := AppendPrompt("/repo", "sess-2", "second prompt", now.Add(time.Second)); err != nil {
		t.Fatalf("append second prompt: %v", err)
	}

	entries, err := LoadRecentPrompts(10)
	if err != nil {
		t.Fatalf("load prompts: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Prompt != "first prompt" || entries[1].Prompt != "second prompt" {
		t.Fatalf("unexpected prompts: %#v", entries)
	}
	if entries[0].SessionID != "sess-1" || entries[1].SessionID != "sess-2" {
		t.Fatalf("unexpected session ids: %#v", entries)
	}
}

func TestLoadRecentPromptsRespectsLimit(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	for i, text := range []string{"a", "b", "c"} {
		if err := AppendPrompt("/repo", "sess", text, time.Unix(int64(i), 0)); err != nil {
			t.Fatalf("append %q: %v", text, err)
		}
	}

	entries, err := LoadRecentPrompts(2)
	if err != nil {
		t.Fatalf("load prompts: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Prompt != "b" || entries[1].Prompt != "c" {
		t.Fatalf("expected most recent prompts [b c], got %#v", entries)
	}
}

func TestLoadRecentPromptsSkipsCorruptedLines(t *testing.T) {
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	path, err := historyFilePath()
	if err != nil {
		t.Fatalf("history path: %v", err)
	}

	content := []byte("{bad json}\n" +
		"{\"prompt\":\"\",\"session_id\":\"s\"}\n" +
		"{\"prompt\":\"ok\",\"session_id\":\"s\",\"timestamp\":\"2026-04-05T10:00:00Z\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	entries, err := LoadRecentPrompts(10)
	if err != nil {
		t.Fatalf("load prompts: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
	if entries[0].Prompt != "ok" {
		t.Fatalf("expected prompt ok, got %#v", entries[0])
	}
}
