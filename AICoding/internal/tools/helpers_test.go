package tools

import (
	"path/filepath"
	"testing"
)

func TestResolvePathRejectsEscape(t *testing.T) {
	workspace := t.TempDir()
	if _, err := resolvePath(workspace, filepath.Join("..", "bad.txt")); err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestResolvePathAllowsWorkspaceFile(t *testing.T) {
	workspace := t.TempDir()
	got, err := resolvePath(workspace, "ok.txt")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != workspace {
		t.Fatalf("unexpected path: %s", got)
	}
}
