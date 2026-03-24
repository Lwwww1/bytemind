package tools

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func resolvePath(workspace, input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return workspace, nil
	}

	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspace, candidate)
	}

	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absWorkspace, absCandidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("path escapes workspace")
	}
	return absCandidate, nil
}

func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func toJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func mustRel(workspace, path string) string {
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "."
	}
	return rel
}

func depthFromRoot(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}
