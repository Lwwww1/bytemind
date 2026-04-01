package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func TestSearchTextToolFindsMatchesCaseInsensitiveAndSkipsHiddenDirsAndBinary(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "a.txt"), "Alpha\nbeta\n")
	mustWriteFile(t, filepath.Join(workspace, "b.txt"), "alpha again\n")
	mustWriteFile(t, filepath.Join(workspace, ".hidden", "ignored.txt"), "alpha hidden\n")
	mustWriteBinaryFile(t, filepath.Join(workspace, "data.bin"), []byte{'a', 0, 'b'})

	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query": "ALPHA",
		"limit": 2,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Query   string `json:"query"`
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Query != "ALPHA" {
		t.Fatalf("expected original query, got %q", parsed.Query)
	}
	if len(parsed.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %#v", parsed.Matches)
	}
	paths := []string{parsed.Matches[0].Path, parsed.Matches[1].Path}
	for _, unwanted := range []string{".hidden/ignored.txt", "data.bin"} {
		if slices.Contains(paths, unwanted) {
			t.Fatalf("did not expect %q in matches, got %v", unwanted, paths)
		}
	}
}

func TestSearchTextToolSupportsCaseSensitiveAndSubpathSearch(t *testing.T) {
	workspace := t.TempDir()
	mustMkdirAll(t, filepath.Join(workspace, "src"))
	mustWriteFile(t, filepath.Join(workspace, "src", "main.go"), "Alpha\nalpha\n")
	mustWriteFile(t, filepath.Join(workspace, "other.txt"), "Alpha elsewhere\n")

	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query":          "Alpha",
		"path":           "src",
		"case_sensitive": true,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Matches) != 1 {
		t.Fatalf("expected 1 case-sensitive subpath match, got %#v", parsed.Matches)
	}
	if parsed.Matches[0].Path != "src/main.go" || parsed.Matches[0].Line != 1 {
		t.Fatalf("unexpected match %#v", parsed.Matches[0])
	}
}

func TestSearchTextToolRejectsEscapedPath(t *testing.T) {
	workspace := t.TempDir()
	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query": "hello",
		"path":  filepath.Join("..", "outside"),
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}
