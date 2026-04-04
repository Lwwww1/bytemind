package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindActiveMentionToken(t *testing.T) {
	t.Run("detects trailing mention", func(t *testing.T) {
		token, ok := findActiveMentionToken("please check @model")
		if !ok {
			t.Fatalf("expected trailing mention to be detected")
		}
		if token.Query != "model" {
			t.Fatalf("expected query model, got %q", token.Query)
		}
	})

	t.Run("supports empty query", func(t *testing.T) {
		token, ok := findActiveMentionToken("@")
		if !ok {
			t.Fatalf("expected single @ to open mention mode")
		}
		if token.Query != "" {
			t.Fatalf("expected empty mention query, got %q", token.Query)
		}
	})

	t.Run("ignores whitespace tail", func(t *testing.T) {
		if _, ok := findActiveMentionToken("@model "); ok {
			t.Fatalf("did not expect mention detection when trailing whitespace exists")
		}
	})

	t.Run("ignores email-like token", func(t *testing.T) {
		if _, ok := findActiveMentionToken("mail a@b.com"); ok {
			t.Fatalf("did not expect mention detection for email token")
		}
	})
}

func TestInsertMentionIntoInput(t *testing.T) {
	token := mentionToken{
		Query: "mod",
		Start: len([]rune("open ")),
		End:   len([]rune("open @mod")),
	}
	got := insertMentionIntoInput("open @mod", token, "internal/tui/model.go")
	want := "open @internal/tui/model.go "
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestWorkspaceFileIndexSearchSkipsIgnoredDirectories(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "README.md"), "hello")
	mustWriteMentionFile(t, filepath.Join(workspace, "internal", "tui", "model.go"), "package tui")
	mustWriteMentionFile(t, filepath.Join(workspace, ".git", "config"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "node_modules", "pkg", "index.js"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "vendor", "pkg", "a.go"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "dist", "bundle.js"), "ignored")
	mustWriteMentionFile(t, filepath.Join(workspace, "build", "artifact.txt"), "ignored")

	index := newWorkspaceFileIndex(workspace)
	all := index.Search("", 100)
	paths := make([]string, 0, len(all))
	for _, item := range all {
		paths = append(paths, item.Path)
	}
	for _, want := range []string{"README.md", "internal/tui/model.go"} {
		if !containsString(paths, want) {
			t.Fatalf("expected indexed files to include %q, got %v", want, paths)
		}
	}
	for _, unwanted := range []string{
		".git/config",
		"node_modules/pkg/index.js",
		"vendor/pkg/a.go",
		"dist/bundle.js",
		"build/artifact.txt",
	} {
		if containsString(paths, unwanted) {
			t.Fatalf("did not expect indexed files to include %q", unwanted)
		}
	}

	filtered := index.Search("model", 5)
	if len(filtered) == 0 {
		t.Fatalf("expected mention search to return matches for model")
	}
	if filtered[0].Path != "internal/tui/model.go" {
		t.Fatalf("expected best match to be internal/tui/model.go, got %q", filtered[0].Path)
	}
	if filtered[0].TypeTag != "go" {
		t.Fatalf("expected model.go tag to be go, got %q", filtered[0].TypeTag)
	}
}

func TestWorkspaceFileIndexSearchWithRecencyPrioritizesRecent(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "alpha.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "beta.go"), "package main")

	index := newWorkspaceFileIndex(workspace)
	results := index.SearchWithRecency("", 10, map[string]int{
		"beta.go":  20,
		"alpha.go": 1,
	})
	if len(results) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(results))
	}
	if results[0].Path != "beta.go" {
		t.Fatalf("expected recent file beta.go first, got %q", results[0].Path)
	}
}

func TestWorkspaceFileIndexSupportsConfigurableIgnoreRules(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "keep.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "envskip.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "logs", "debug.log"), "line")
	mustWriteMentionFile(t, filepath.Join(workspace, "custom", "skip.txt"), "line")
	mustWriteMentionFile(t, filepath.Join(workspace, ".bytemindignore"), "custom/*\n")
	t.Setenv("BYTEMIND_MENTION_IGNORE", "envskip.go,logs/*")

	index := newWorkspaceFileIndex(workspace)
	results := index.Search("", 50)
	paths := make([]string, 0, len(results))
	for _, item := range results {
		paths = append(paths, item.Path)
	}

	if !containsString(paths, "keep.go") {
		t.Fatalf("expected keep.go in results, got %v", paths)
	}
	for _, unwanted := range []string{"envskip.go", "logs/debug.log", "custom/skip.txt"} {
		if containsString(paths, unwanted) {
			t.Fatalf("did not expect ignored path %q in results %v", unwanted, paths)
		}
	}
}

func TestWorkspaceFileIndexRespectsMaxFilesLimitFromEnv(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "a.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "b.go"), "package main")
	mustWriteMentionFile(t, filepath.Join(workspace, "c.go"), "package main")
	t.Setenv("BYTEMIND_MENTION_MAX_FILES", "2")

	index := newWorkspaceFileIndex(workspace)
	results := index.Search("", 50)
	if len(results) != 2 {
		t.Fatalf("expected max-files limited result count 2, got %d", len(results))
	}
	stats := index.Stats()
	if !stats.Truncated {
		t.Fatalf("expected stats to mark index as truncated")
	}
	if stats.MaxFiles != 2 {
		t.Fatalf("expected max files 2 from env, got %d", stats.MaxFiles)
	}
}

func TestWorkspaceFileIndexRebuildsAfterRefreshInterval(t *testing.T) {
	workspace := t.TempDir()
	mustWriteMentionFile(t, filepath.Join(workspace, "a.txt"), "a")
	index := newWorkspaceFileIndex(workspace)
	first := index.Search("", 10)
	if len(first) != 1 {
		t.Fatalf("expected initial index size 1, got %d", len(first))
	}

	mustWriteMentionFile(t, filepath.Join(workspace, "b.txt"), "b")
	index.mu.Lock()
	index.lastBuild = time.Now().Add(-mentionIndexRefreshInterval - time.Second)
	index.mu.Unlock()

	deadline := time.Now().Add(800 * time.Millisecond)
	for {
		second := index.Search("", 10)
		if len(second) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected rebuilt index size 2, got %d", len(second))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMentionTypeTag(t *testing.T) {
	cases := map[string]string{
		"main.go":        "go",
		"README.md":      "md",
		"script.ps1":     "ps1",
		"archive.tar.gz": "gz",
		"noext":          "file",
	}
	for input, want := range cases {
		if got := mentionTypeTag(input); got != want {
			t.Fatalf("expected mentionTypeTag(%q)=%q, got %q", input, want, got)
		}
	}
}

func mustWriteMentionFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
