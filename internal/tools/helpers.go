package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultListFilesMaxVisits     = 6000
	defaultSearchTextMaxVisits    = 12000
	defaultSearchTextMaxFiles     = 2000
	defaultSearchTextMaxBytes     = 24 * 1024 * 1024
	defaultSearchTextMaxFileBytes = 1 * 1024 * 1024
)

func resolvePath(workspace, input string, writableRoots ...string) (string, error) {
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
	absWorkspace = filepath.Clean(absWorkspace)

	allowedRoots, err := resolveAllowedRoots(absWorkspace, writableRoots)
	if err != nil {
		return "", err
	}
	canonicalCandidate, err := canonicalPathForAccess(absCandidate)
	if err != nil {
		return "", err
	}

	for _, root := range allowedRoots {
		canonicalRoot, err := canonicalPathForAccess(root)
		if err != nil {
			return "", err
		}
		if isPathWithinRoot(canonicalRoot, canonicalCandidate) {
			return absCandidate, nil
		}
	}
	return "", fmt.Errorf("permission denied: path %q escapes workspace and writable_roots", input)
}

func canonicalPathForAccess(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return "", fmt.Errorf("invalid empty path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	existingAncestor, missingSegments, ancestorErr := splitMissingPathSegments(path)
	if ancestorErr != nil {
		return "", ancestorErr
	}
	resolvedAncestor, err := filepath.EvalSymlinks(existingAncestor)
	if err != nil {
		return "", err
	}
	canonical := filepath.Clean(resolvedAncestor)
	for _, segment := range missingSegments {
		canonical = filepath.Join(canonical, segment)
	}
	return canonical, nil
}

func splitMissingPathSegments(path string) (existingAncestor string, missingSegments []string, err error) {
	current := filepath.Clean(path)
	missing := make([]string, 0, 4)
	for {
		_, statErr := os.Lstat(current)
		if statErr == nil {
			for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
				missing[i], missing[j] = missing[j], missing[i]
			}
			return current, missing, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", nil, statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil, statErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func resolveAllowedRoots(absWorkspace string, writableRoots []string) ([]string, error) {
	roots := make([]string, 0, len(writableRoots)+1)
	roots = append(roots, absWorkspace)
	for _, root := range writableRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		absRoot = filepath.Clean(absRoot)
		if absRoot == absWorkspace {
			continue
		}
		roots = append(roots, absRoot)
	}
	return roots, nil
}

func isPathWithinRoot(root, candidate string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	candidate = filepath.Clean(strings.TrimSpace(candidate))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func writableRootsFromExecContext(execCtx *ExecutionContext) []string {
	if execCtx == nil || len(execCtx.WritableRoots) == 0 {
		return nil
	}
	roots := make([]string, 0, len(execCtx.WritableRoots))
	for _, root := range execCtx.WritableRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		roots = append(roots, root)
	}
	return roots
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
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return path
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

func maxListFilesVisits() int {
	return positiveEnvInt("BYTEMIND_LIST_FILES_MAX_VISITS", defaultListFilesMaxVisits)
}

func maxSearchTextVisits() int {
	return positiveEnvInt("BYTEMIND_SEARCH_MAX_VISITS", defaultSearchTextMaxVisits)
}

func maxSearchTextFiles() int {
	return positiveEnvInt("BYTEMIND_SEARCH_MAX_FILES", defaultSearchTextMaxFiles)
}

func maxSearchTextBytes() int64 {
	return int64(positiveEnvInt("BYTEMIND_SEARCH_MAX_BYTES", defaultSearchTextMaxBytes))
}

func maxSearchTextFileBytes() int64 {
	return int64(positiveEnvInt("BYTEMIND_SEARCH_MAX_FILE_BYTES", defaultSearchTextMaxFileBytes))
}

func shouldSkipToolDir(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	switch lower {
	case "node_modules", "vendor", "dist", "build", "target", "coverage", ".next", ".nuxt", "out", "bin", "obj":
		return true
	default:
		return false
	}
}

func positiveEnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
