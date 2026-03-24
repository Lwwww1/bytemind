package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileReturnsEmptyWhenStartLineExceedsFile(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ReadFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path":       "sample.txt",
		"start_line": 10,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Content   string `json:"content"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Content != "" {
		t.Fatalf("expected empty content, got %q", parsed.Content)
	}
	if parsed.StartLine != 3 || parsed.EndLine != 2 {
		t.Fatalf("unexpected bounds: start=%d end=%d", parsed.StartLine, parsed.EndLine)
	}
}
