package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBinaryName(t *testing.T) {
	if got := defaultBinaryName("windows"); got != "bytemind.exe" {
		t.Fatalf("expected windows binary name with .exe, got %q", got)
	}
	if got := defaultBinaryName("linux"); got != "bytemind" {
		t.Fatalf("expected non-windows binary name without extension, got %q", got)
	}
}

func TestResolveInstallTargetUsesBytemindHomeByDefault(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".bytemind-home")
	t.Setenv("BYTEMIND_HOME", home)

	target, err := resolveInstallTarget("", "custom-bin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "bin", "custom-bin")
	if !samePath(target, want) {
		t.Fatalf("expected target %q, got %q", want, target)
	}
}

func TestResolveInstallTargetRejectsNamePath(t *testing.T) {
	_, err := resolveInstallTarget("", "nested/bytemind")
	if err == nil {
		t.Fatal("expected install target resolution to reject path-like name")
	}
}

func TestInstallBinaryCopiesExecutableFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.exe")
	content := []byte("binary-content")
	if err := os.WriteFile(source, content, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "bin", "bytemind.exe")
	if err := installBinary(source, target); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("expected target content %q, got %q", string(content), string(got))
	}
}

func TestPathContainsDirForOS(t *testing.T) {
	pathEnv := strings.Join([]string{"C:/Tools", "C:/Users/Wheat/.bytemind/bin"}, ";")
	if !pathContainsDirForOS(pathEnv, `c:\users\wheat\.bytemind\bin`, true) {
		t.Fatal("expected windows path lookup to be case-insensitive and slash-insensitive")
	}
	if pathContainsDirForOS(pathEnv, `C:\missing`, true) {
		t.Fatal("did not expect missing path entry")
	}
}

func TestAppendPathEntryAvoidsDuplicates(t *testing.T) {
	current := strings.Join([]string{`C:\Tools`, `C:\Users\wheat\.bytemind\bin`}, ";")
	next, changed := appendPathEntry(current, `c:\users\wheat\.bytemind\bin`, true)
	if changed {
		t.Fatalf("expected duplicate path to be ignored, got changed=true next=%q", next)
	}
	if next != current {
		t.Fatalf("expected unchanged path, got %q", next)
	}
}

func TestAppendPathEntryAddsMissingEntry(t *testing.T) {
	current := `C:\Tools`
	next, changed := appendPathEntry(current, `C:\Users\wheat\.bytemind\bin`, true)
	if !changed {
		t.Fatal("expected missing path entry to be appended")
	}
	if !strings.Contains(next, `C:\Users\wheat\.bytemind\bin`) {
		t.Fatalf("expected appended path entry, got %q", next)
	}
}

func TestAddToWindowsUserPathUsesGetterAndSetter(t *testing.T) {
	originalGetter := windowsUserPathGetter
	originalSetter := windowsUserPathSetter
	t.Cleanup(func() {
		windowsUserPathGetter = originalGetter
		windowsUserPathSetter = originalSetter
	})

	windowsUserPathGetter = func() (string, error) {
		return `C:\Tools`, nil
	}
	captured := ""
	windowsUserPathSetter = func(newPath string) error {
		captured = newPath
		return nil
	}

	changed, err := addToWindowsUserPath(`C:\Users\wheat\.bytemind\bin`)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected user path to change")
	}
	if !strings.Contains(captured, `C:\Users\wheat\.bytemind\bin`) {
		t.Fatalf("expected setter to receive appended path, got %q", captured)
	}
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
