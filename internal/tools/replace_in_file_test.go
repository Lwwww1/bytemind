package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceInFileToolReplacesFirstMatch(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	mustWriteFile(t, path, "alpha beta alpha")

	tool := ReplaceInFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path": "sample.txt",
		"old":  "alpha",
		"new":  "gamma",
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "gamma beta alpha" {
		t.Fatalf("unexpected replacement result %q", string(data))
	}

	var parsed struct {
		Replaced int `json:"replaced"`
		OldCount int `json:"old_count"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Replaced != 1 || parsed.OldCount != 2 {
		t.Fatalf("unexpected replacement counts %#v", parsed)
	}
}

func TestReplaceInFileToolReplacesAllMatches(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	mustWriteFile(t, path, "alpha beta alpha")

	tool := ReplaceInFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path":        "sample.txt",
		"old":         "alpha",
		"new":         "gamma",
		"replace_all": true,
	})
	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "gamma beta gamma" {
		t.Fatalf("unexpected replacement result %q", string(data))
	}
}

func TestReplaceInFileToolRejectsMissingTargetText(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "sample.txt"), "alpha beta")

	tool := ReplaceInFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path": "sample.txt",
		"old":  "missing",
		"new":  "gamma",
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err == nil {
		t.Fatal("expected missing target error")
	}
}

func TestReplaceInFileToolRejectsEscapedPath(t *testing.T) {
	workspace := t.TempDir()
	tool := ReplaceInFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path": filepath.Join("..", "outside.txt"),
		"old":  "a",
		"new":  "b",
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

