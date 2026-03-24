package tools

import (
	"path/filepath"
	"testing"
)

func TestResolveWorkspacePathRejectsEscape(t *testing.T) {
	workspace := t.TempDir()
	if _, err := resolveWorkspacePath(workspace, filepath.Join("..", "outside.txt")); err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestApplyOperations(t *testing.T) {
	content := "alpha\nfoo\nfoo\n"
	updated, details, err := applyOperations(content, []PatchOperation{
		{Old: "foo", New: "bar"},
		{Old: "foo", New: "baz"},
	})
	if err != nil {
		t.Fatalf("applyOperations returned error: %v", err)
	}
	expected := "alpha\nbar\nbaz\n"
	if updated != expected {
		t.Fatalf("unexpected patched content\nwant: %q\ngot:  %q", expected, updated)
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 detail entries, got %d", len(details))
	}
}

func TestNormalizeExecutable(t *testing.T) {
	exe, err := normalizeExecutable(`"C:\Program Files\Go\bin\go.exe" test ./...`)
	if err != nil {
		t.Fatalf("normalizeExecutable returned error: %v", err)
	}
	if exe != "go" {
		t.Fatalf("expected go, got %q", exe)
	}
}
