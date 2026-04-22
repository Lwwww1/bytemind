package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileToolCreatesDirectoriesAndWritesContent(t *testing.T) {
	workspace := t.TempDir()
	tool := WriteFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path":        "nested/file.txt",
		"content":     "hello world",
		"create_dirs": true,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("unexpected content %q", string(data))
	}

	var parsed struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytes_written"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "nested/file.txt" || parsed.BytesWritten != len("hello world") {
		t.Fatalf("unexpected result %#v", parsed)
	}
}

func TestWriteFileToolOverwritesExistingFile(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "file.txt"), "old")

	tool := WriteFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path":    "file.txt",
		"content": "new",
	})
	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("expected overwritten content, got %q", string(data))
	}
}

func TestWriteFileToolRejectsEscapedPath(t *testing.T) {
	workspace := t.TempDir()
	tool := WriteFileTool{}
	payload, _ := json.Marshal(map[string]any{
		"path":    filepath.Join("..", "outside.txt"),
		"content": "hello",
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

func TestWriteFileToolAllowsConfiguredWritableRoot(t *testing.T) {
	workspace := t.TempDir()
	writableRoot := filepath.Join(t.TempDir(), "external-output")
	tool := WriteFileTool{}
	target := filepath.Join(writableRoot, "nested", "file.txt")
	payload, _ := json.Marshal(map[string]any{
		"path":        target,
		"content":     "hello writable root",
		"create_dirs": true,
	})

	result, err := tool.Run(context.Background(), payload, &ExecutionContext{
		Workspace:     workspace,
		WritableRoots: []string{writableRoot},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello writable root" {
		t.Fatalf("unexpected content %q", string(data))
	}

	var parsed struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Path != filepath.ToSlash(target) {
		t.Fatalf("expected external absolute path %q in result, got %q", filepath.ToSlash(target), parsed.Path)
	}
}
