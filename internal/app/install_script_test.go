package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func loadInstallPSScript(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "scripts", "install.ps1")

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read install.ps1: %v", err)
	}

	return string(content)
}

func TestInstallPSScript_ArchitectureFallbackForLegacyPowerShell(t *testing.T) {
	script := loadInstallPSScript(t)

	requiredSnippets := []string{
		`GetProperty("OSArchitecture")`,
		`PROCESSOR_ARCHITEW6432`,
		`PROCESSOR_ARCHITECTURE`,
		`"AMD64" { return "amd64" }`,
		`"ARM64" { return "arm64" }`,
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("install.ps1 missing legacy-compat architecture logic snippet: %q", snippet)
		}
	}
}
