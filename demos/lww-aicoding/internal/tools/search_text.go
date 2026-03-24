package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aicoding/internal/llm"
)

type SearchTextTool struct{}

func (SearchTextTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "search_text",
			Description: "Search text across workspace files",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Text to search for.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Optional directory or file to limit the search.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of matches to return. Defaults to 50.",
					},
					"case_sensitive": map[string]any{
						"type":        "boolean",
						"description": "Whether the search is case sensitive.",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (SearchTextTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	var args struct {
		Query         string `json:"query"`
		Path          string `json:"path"`
		Limit         int    `json:"limit"`
		CaseSensitive bool   `json:"case_sensitive"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if args.Limit <= 0 {
		args.Limit = 50
	}
	root, err := resolvePath(execCtx.Workspace, args.Path)
	if err != nil {
		return "", err
	}

	needle := args.Query
	if !args.CaseSensitive {
		needle = strings.ToLower(needle)
	}

	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches := make([]match, 0, args.Limit)

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && isHidden(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil || !isText(data) {
			return nil
		}

		scanner := bufio.NewScanner(bytes.NewReader(data))
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			text := scanner.Text()
			haystack := text
			if !args.CaseSensitive {
				haystack = strings.ToLower(text)
			}
			if strings.Contains(haystack, needle) {
				matches = append(matches, match{
					Path: filepath.ToSlash(mustRel(execCtx.Workspace, path)),
					Line: lineNumber,
					Text: text,
				})
				if len(matches) >= args.Limit {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if walkErr != nil && walkErr != fs.SkipAll {
		return "", walkErr
	}

	return toJSON(map[string]any{
		"ok":      true,
		"query":   args.Query,
		"matches": matches,
	})
}
