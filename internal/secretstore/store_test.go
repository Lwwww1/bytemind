package secretstore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envBytemindHome, home)

	if err := Save("BYTEMIND_API_KEY", "secret-value"); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := Load("BYTEMIND_API_KEY")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got != "secret-value" {
		t.Fatalf("expected persisted secret, got %q", got)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envBytemindHome, home)

	got, err := Load("BYTEMIND_API_KEY")
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty secret for missing store, got %q", got)
	}
}

func TestSaveUsesDefaultNameWhenEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envBytemindHome, home)

	if err := Save("", "default-secret"); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := Load(defaultKeyName)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got != "default-secret" {
		t.Fatalf("expected default-name secret, got %q", got)
	}
}

func TestSaveWritesStoreFileWithTightPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envBytemindHome, home)

	if err := Save("BYTEMIND_API_KEY", "secret-value"); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	path := filepath.Join(home, "auth", storeFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected store file, got %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("expected mode 0600, got %o", mode)
	}
}
