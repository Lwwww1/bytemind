package tools

import (
	"os"
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

func TestResolvePathAllowsConfiguredWritableRoot(t *testing.T) {
	workspace := t.TempDir()
	writableRoot := filepath.Join(t.TempDir(), "external-output")
	got, err := resolvePath(workspace, filepath.Join(writableRoot, "ok.txt"), writableRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != writableRoot {
		t.Fatalf("expected writable root path, got %s", got)
	}
}

func TestResolvePathRejectsOutsideWritableRoots(t *testing.T) {
	workspace := t.TempDir()
	writableRoot := filepath.Join(t.TempDir(), "external-output")
	anotherRoot := filepath.Join(t.TempDir(), "blocked-output")
	if _, err := resolvePath(workspace, filepath.Join(anotherRoot, "blocked.txt"), writableRoot); err == nil {
		t.Fatal("expected path outside writable roots to be rejected")
	}
}

func TestResolvePathRejectsSymlinkFileEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "leak.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if _, err := resolvePath(workspace, "leak.txt"); err == nil {
		t.Fatal("expected symlink file escape to be rejected")
	}
}

func TestResolvePathRejectsSymlinkDirectoryEscapeForNewFile(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	linkDir := filepath.Join(workspace, "out-link")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if _, err := resolvePath(workspace, filepath.Join("out-link", "new.txt")); err == nil {
		t.Fatal("expected symlink directory escape to be rejected")
	}
}

func TestResolvePathAllowsSymlinkWhenTargetInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	targetDir := filepath.Join(workspace, "safe")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(workspace, "safe-link")
	if err := os.Symlink(targetDir, linkDir); err != nil {
		t.Skipf("symlink unavailable on this environment: %v", err)
	}
	if _, err := resolvePath(workspace, filepath.Join("safe-link", "ok.txt")); err != nil {
		t.Fatalf("expected symlink within workspace to be allowed, got %v", err)
	}
}
