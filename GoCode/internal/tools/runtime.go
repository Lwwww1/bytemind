package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	stdruntime "runtime"
	"strings"
	"time"

	"gocode/internal/llm"
	"gocode/internal/session"
)

type ConfirmFunc func(prompt string) (bool, error)

type Runtime struct {
	workspace       string
	allowedCommands map[string]struct{}
	session         *session.Store
	undoLog         []undoAction
	taskMarks       []taskMark
}

type undoAction struct {
	description string
	undo        func() error
}

type taskMark struct {
	label string
	start int
}

type PatchOperation struct {
	Old        string `json:"old"`
	New        string `json:"new"`
	ReplaceAll bool   `json:"replace_all"`
}

type snapshot struct {
	Target  string
	Entries []snapshotEntry
}

type snapshotEntry struct {
	Path  string
	Mode  fs.FileMode
	IsDir bool
	Data  []byte
}

func NewRuntime(workspace string, allowedCommands []string, store *session.Store) *Runtime {
	allowed := make(map[string]struct{}, len(allowedCommands))
	for _, command := range allowedCommands {
		trimmed := strings.ToLower(strings.TrimSpace(command))
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	return &Runtime{
		workspace:       workspace,
		allowedCommands: allowed,
		session:         store,
	}
}

func (r *Runtime) Workspace() string {
	return r.workspace
}

func (r *Runtime) BeginTask(label string) {
	r.taskMarks = append(r.taskMarks, taskMark{label: strings.TrimSpace(label), start: len(r.undoLog)})
}

func (r *Runtime) UndoLastTask(_ context.Context) (string, error) {
	if len(r.taskMarks) == 0 {
		return "", fmt.Errorf("no task is available to undo")
	}
	mark := r.taskMarks[len(r.taskMarks)-1]
	r.taskMarks = r.taskMarks[:len(r.taskMarks)-1]
	if mark.start == len(r.undoLog) {
		return "最近一次任务没有可回滚的写操作。", nil
	}

	count := 0
	for i := len(r.undoLog) - 1; i >= mark.start; i-- {
		if err := r.undoLog[i].undo(); err != nil {
			return "", fmt.Errorf("undo %s: %w", r.undoLog[i].description, err)
		}
		count++
	}
	r.undoLog = r.undoLog[:mark.start]
	return fmt.Sprintf("已回滚最近一次任务，撤销 %d 个写操作。", count), nil
}

func (r *Runtime) Definitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		toolDef(
			"list_dir",
			"List files and directories under a workspace path.",
			objectSchema(map[string]any{
				"path": stringProp("Workspace-relative or absolute path inside the workspace."),
			}),
		),
		toolDef(
			"read_file",
			"Read a text file inside the workspace.",
			objectSchema(map[string]any{
				"path":      stringProp("Workspace-relative or absolute file path."),
				"max_chars": intProp("Maximum number of characters to return."),
			}, "path"),
		),
		toolDef(
			"search_text",
			"Search text or regex matches under a file or directory.",
			objectSchema(map[string]any{
				"query":       stringProp("Plain text or regex query."),
				"path":        stringProp("Root path to search. Defaults to the workspace root."),
				"use_regex":   boolProp("Whether query should be treated as a regular expression."),
				"max_results": intProp("Maximum number of matches to return."),
			}, "query"),
		),
		toolDef(
			"write_file",
			"Create a new file or overwrite an existing file inside the workspace.",
			objectSchema(map[string]any{
				"path":      stringProp("File path to write."),
				"content":   stringProp("Complete file content."),
				"overwrite": boolProp("Set true when replacing an existing file."),
			}, "path", "content"),
		),
		toolDef(
			"apply_patch",
			"Apply exact search-and-replace operations to an existing file.",
			objectSchema(map[string]any{
				"path": stringProp("File path to patch."),
				"operations": map[string]any{
					"type": "array",
					"items": objectSchema(map[string]any{
						"old":         stringProp("Exact text to replace."),
						"new":         stringProp("Replacement text."),
						"replace_all": boolProp("Replace all occurrences instead of only the first one."),
					}, "old", "new"),
					"description": "Ordered patch operations.",
				},
			}, "path", "operations"),
		),
		toolDef(
			"make_dir",
			"Create a directory inside the workspace.",
			objectSchema(map[string]any{
				"path": stringProp("Directory path to create."),
			}, "path"),
		),
		toolDef(
			"move_path",
			"Move or rename a file or directory inside the workspace.",
			objectSchema(map[string]any{
				"from":      stringProp("Source path."),
				"to":        stringProp("Destination path."),
				"overwrite": boolProp("Set true when replacing an existing destination file."),
			}, "from", "to"),
		),
		toolDef(
			"delete_path",
			"Delete a file or directory inside the workspace.",
			objectSchema(map[string]any{
				"path":      stringProp("Path to delete."),
				"recursive": boolProp("Required for directories."),
			}, "path"),
		),
		toolDef(
			"run_command",
			"Run an allowlisted command inside the workspace and capture output.",
			objectSchema(map[string]any{
				"cmd":         stringProp("Command string to execute."),
				"cwd":         stringProp("Command working directory, relative to the workspace when not absolute."),
				"timeout_sec": intProp("Timeout in seconds, default 30, max 120."),
			}, "cmd"),
		),
		toolDef(
			"git_diff",
			"Show the current git diff for the workspace.",
			objectSchema(map[string]any{}),
		),
	}
}

func (r *Runtime) Execute(ctx context.Context, name string, argsJSON []byte, confirm ConfirmFunc) (string, error) {
	switch name {
	case "list_dir":
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.listDir(args.Path)
	case "read_file":
		var args struct {
			Path     string `json:"path"`
			MaxChars int    `json:"max_chars"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.readFile(args.Path, args.MaxChars)
	case "search_text":
		var args struct {
			Query      string `json:"query"`
			Path       string `json:"path"`
			UseRegex   bool   `json:"use_regex"`
			MaxResults int    `json:"max_results"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.searchText(args.Query, args.Path, args.UseRegex, args.MaxResults)
	case "write_file":
		var args struct {
			Path      string `json:"path"`
			Content   string `json:"content"`
			Overwrite bool   `json:"overwrite"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.writeFile(args.Path, args.Content, args.Overwrite, confirm)
	case "apply_patch":
		var args struct {
			Path       string           `json:"path"`
			Operations []PatchOperation `json:"operations"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.applyPatch(args.Path, args.Operations)
	case "make_dir":
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.makeDir(args.Path)
	case "move_path":
		var args struct {
			From      string `json:"from"`
			To        string `json:"to"`
			Overwrite bool   `json:"overwrite"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.movePath(args.From, args.To, args.Overwrite, confirm)
	case "delete_path":
		var args struct {
			Path      string `json:"path"`
			Recursive bool   `json:"recursive"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.deletePath(args.Path, args.Recursive, confirm)
	case "run_command":
		var args struct {
			Cmd        string `json:"cmd"`
			Cwd        string `json:"cwd"`
			TimeoutSec int    `json:"timeout_sec"`
		}
		if err := decodeArgs(argsJSON, &args); err != nil {
			return "", err
		}
		return r.runCommand(ctx, args.Cmd, args.Cwd, args.TimeoutSec)
	case "git_diff":
		return r.gitDiff(ctx)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (r *Runtime) listDir(inputPath string) (string, error) {
	target, err := resolveWorkspacePath(r.workspace, defaultPath(inputPath))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", inputPath)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return "", fmt.Errorf("read directory: %w", err)
	}
	type item struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	items := make([]item, 0, len(entries))
	for _, entry := range entries {
		entryType := "file"
		if entry.IsDir() {
			entryType = "dir"
		}
		items = append(items, item{Name: entry.Name(), Type: entryType})
	}
	return encodeResult(map[string]any{
		"ok":      true,
		"path":    r.relativePath(target),
		"entries": items,
	}), nil
}

func (r *Runtime) readFile(inputPath string, maxChars int) (string, error) {
	target, err := resolveWorkspacePath(r.workspace, inputPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("refusing to read binary file: %s", inputPath)
	}
	if maxChars <= 0 {
		maxChars = 12000
	}
	content, truncated := limitString(string(data), maxChars)
	r.session.AddFile(r.relativePath(target))
	return encodeResult(map[string]any{
		"ok":        true,
		"path":      r.relativePath(target),
		"truncated": truncated,
		"content":   content,
	}), nil
}

func (r *Runtime) searchText(query, inputPath string, useRegex bool, maxResults int) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	target, err := resolveWorkspacePath(r.workspace, defaultPath(inputPath))
	if err != nil {
		return "", err
	}
	pattern := query
	if !useRegex {
		pattern = regexp.QuoteMeta(query)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("compile query: %w", err)
	}
	if maxResults <= 0 {
		maxResults = 30
	}
	if maxResults > 100 {
		maxResults = 100
	}

	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches := make([]match, 0, maxResults)

	visit := func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= maxResults {
			return errSearchDone
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		buffer := make([]byte, 0, 64*1024)
		scanner.Buffer(buffer, 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches, match{Path: r.relativePath(path), Line: lineNo, Text: truncateLine(line, 240)})
				r.session.AddFile(r.relativePath(path))
				if len(matches) >= maxResults {
					return errSearchDone
				}
			}
		}
		return nil
	}

	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat search root: %w", err)
	}
	if info.IsDir() {
		err = filepath.WalkDir(target, visit)
	} else {
		err = visit(target, dirEntryFromInfo(info), nil)
	}
	if err != nil && !errors.Is(err, errSearchDone) {
		return "", fmt.Errorf("search text: %w", err)
	}

	return encodeResult(map[string]any{
		"ok":      true,
		"query":   query,
		"matches": matches,
	}), nil
}

func (r *Runtime) writeFile(inputPath, content string, overwrite bool, confirm ConfirmFunc) (string, error) {
	target, err := resolveWorkspacePath(r.workspace, inputPath)
	if err != nil {
		return "", err
	}
	existing, err := captureSnapshot(target)
	if err != nil {
		return "", err
	}
	exists := len(existing.Entries) > 0
	if exists && !overwrite {
		return "", fmt.Errorf("file already exists, set overwrite=true to replace it")
	}
	if exists {
		if err := requestConfirmation(confirm, fmt.Sprintf("将覆盖文件 %s，是否继续？", r.relativePath(target))); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	r.pushUndo("restore "+r.relativePath(target), func() error {
		return restoreSnapshot(existing)
	})
	action := "create"
	if exists {
		action = "update"
	}
	r.session.AddFile(r.relativePath(target))
	r.session.AddChange(action, r.relativePath(target), fmt.Sprintf("写入 %d 字节", len(content)))
	return encodeResult(map[string]any{
		"ok":     true,
		"path":   r.relativePath(target),
		"action": action,
	}), nil
}

func (r *Runtime) applyPatch(inputPath string, operations []PatchOperation) (string, error) {
	if len(operations) == 0 {
		return "", fmt.Errorf("operations are required")
	}
	target, err := resolveWorkspacePath(r.workspace, inputPath)
	if err != nil {
		return "", err
	}
	original, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read patch target: %w", err)
	}
	updated, details, err := applyOperations(string(original), operations)
	if err != nil {
		return "", err
	}
	if updated == string(original) {
		return encodeResult(map[string]any{
			"ok":      true,
			"path":    r.relativePath(target),
			"changed": false,
		}), nil
	}
	previous, err := captureSnapshot(target)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write patched file: %w", err)
	}
	r.pushUndo("restore "+r.relativePath(target), func() error {
		return restoreSnapshot(previous)
	})
	r.session.AddFile(r.relativePath(target))
	r.session.AddChange("patch", r.relativePath(target), strings.Join(details, "; "))
	return encodeResult(map[string]any{
		"ok":      true,
		"path":    r.relativePath(target),
		"changed": true,
		"details": details,
	}), nil
}

func (r *Runtime) makeDir(inputPath string) (string, error) {
	target, err := resolveWorkspacePath(r.workspace, inputPath)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(target); statErr == nil {
		if info.IsDir() {
			return encodeResult(map[string]any{
				"ok":     true,
				"path":   r.relativePath(target),
				"action": "exists",
			}), nil
		}
		return "", fmt.Errorf("path already exists and is not a directory: %s", inputPath)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", fmt.Errorf("make directory: %w", err)
	}
	r.pushUndo("remove "+r.relativePath(target), func() error {
		return os.RemoveAll(target)
	})
	r.session.AddFile(r.relativePath(target))
	r.session.AddChange("mkdir", r.relativePath(target), "创建目录")
	return encodeResult(map[string]any{
		"ok":     true,
		"path":   r.relativePath(target),
		"action": "created",
	}), nil
}

func (r *Runtime) movePath(fromInput, toInput string, overwrite bool, confirm ConfirmFunc) (string, error) {
	from, err := resolveWorkspacePath(r.workspace, fromInput)
	if err != nil {
		return "", err
	}
	to, err := resolveWorkspacePath(r.workspace, toInput)
	if err != nil {
		return "", err
	}
	if from == to {
		return "", fmt.Errorf("source and destination are the same")
	}
	if _, err := os.Stat(from); err != nil {
		return "", fmt.Errorf("stat source: %w", err)
	}
	var overwritten *snapshot
	if info, err := os.Stat(to); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("destination directory already exists: %s", toInput)
		}
		if !overwrite {
			return "", fmt.Errorf("destination exists, set overwrite=true to replace it")
		}
		if err := requestConfirmation(confirm, fmt.Sprintf("将覆盖目标 %s，是否继续？", r.relativePath(to))); err != nil {
			return "", err
		}
		overwritten, err = captureSnapshot(to)
		if err != nil {
			return "", err
		}
		if err := os.RemoveAll(to); err != nil {
			return "", fmt.Errorf("remove existing destination: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return "", fmt.Errorf("create destination parent: %w", err)
	}
	if err := os.Rename(from, to); err != nil {
		return "", fmt.Errorf("move path: %w", err)
	}
	r.pushUndo("move back "+r.relativePath(to), func() error {
		if err := os.MkdirAll(filepath.Dir(from), 0o755); err != nil {
			return err
		}
		if err := os.Rename(to, from); err != nil {
			return err
		}
		if overwritten != nil {
			return restoreSnapshot(overwritten)
		}
		return nil
	})
	r.session.AddFile(r.relativePath(from))
	r.session.AddFile(r.relativePath(to))
	r.session.AddChange("move", r.relativePath(from), fmt.Sprintf("移动到 %s", r.relativePath(to)))
	return encodeResult(map[string]any{
		"ok":   true,
		"from": r.relativePath(from),
		"to":   r.relativePath(to),
	}), nil
}

func (r *Runtime) deletePath(inputPath string, recursive bool, confirm ConfirmFunc) (string, error) {
	target, err := resolveWorkspacePath(r.workspace, inputPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if info.IsDir() && !recursive {
		return "", fmt.Errorf("recursive=true is required to delete directories")
	}
	if err := requestConfirmation(confirm, fmt.Sprintf("将删除 %s，是否继续？", r.relativePath(target))); err != nil {
		return "", err
	}
	previous, err := captureSnapshot(target)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(target); err != nil {
		return "", fmt.Errorf("delete path: %w", err)
	}
	r.pushUndo("restore "+r.relativePath(target), func() error {
		return restoreSnapshot(previous)
	})
	r.session.AddFile(r.relativePath(target))
	r.session.AddChange("delete", r.relativePath(target), "删除路径")
	return encodeResult(map[string]any{
		"ok":   true,
		"path": r.relativePath(target),
	}), nil
}

func (r *Runtime) runCommand(ctx context.Context, command, cwdInput string, timeoutSec int) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("cmd is required")
	}
	exeName, err := normalizeExecutable(command)
	if err != nil {
		return "", err
	}
	if _, ok := r.allowedCommands[exeName]; !ok {
		return "", fmt.Errorf("command %q is not allowlisted", exeName)
	}
	cwd, err := resolveWorkspacePath(r.workspace, defaultPath(cwdInput))
	if err != nil {
		return "", err
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if timeoutSec > 120 {
		timeoutSec = 120
	}
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-lc", command)
	}
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			exitCode = exitErr.ExitCode()
		case errors.Is(cmdCtx.Err(), context.DeadlineExceeded):
			exitCode = -1
		default:
			return "", fmt.Errorf("run command: %w", err)
		}
	}
	outputText, _ := limitString(string(output), 16000)
	result := session.CommandResult{
		Command:  command,
		Cwd:      r.relativePath(cwd),
		ExitCode: exitCode,
		Output:   outputText,
		At:       time.Now(),
	}
	r.session.AddCommand(result)
	return encodeResult(map[string]any{
		"ok":        exitCode == 0,
		"command":   command,
		"cwd":       r.relativePath(cwd),
		"exit_code": exitCode,
		"output":    outputText,
	}), nil
}

func (r *Runtime) gitDiff(ctx context.Context) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", "-C", r.workspace, "diff", "--", ".")
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("git diff: %w", err)
		}
	}
	text, _ := limitString(string(output), 16000)
	if strings.TrimSpace(text) == "" {
		text = "(no diff)"
	}
	r.session.AddCommand(session.CommandResult{
		Command:  "git diff -- .",
		Cwd:      ".",
		ExitCode: exitCode,
		Output:   text,
		At:       time.Now(),
	})
	return encodeResult(map[string]any{
		"ok":        exitCode == 0,
		"command":   "git diff -- .",
		"exit_code": exitCode,
		"output":    text,
	}), nil
}

func (r *Runtime) pushUndo(description string, undo func() error) {
	if undo == nil {
		return
	}
	r.undoLog = append(r.undoLog, undoAction{description: description, undo: undo})
}

func (r *Runtime) relativePath(abs string) string {
	rel, err := filepath.Rel(r.workspace, abs)
	if err != nil {
		return abs
	}
	if rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func resolveWorkspacePath(workspace, input string) (string, error) {
	workspace, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	path := strings.TrimSpace(input)
	if path == "" {
		path = "."
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	rel, err := filepath.Rel(workspace, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace: %s", input)
	}
	return absPath, nil
}

func captureSnapshot(target string) (*snapshot, error) {
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &snapshot{Target: target}, nil
		}
		return nil, fmt.Errorf("stat snapshot target: %w", err)
	}
	shot := &snapshot{Target: target}
	if !info.IsDir() {
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, fmt.Errorf("read snapshot file: %w", err)
		}
		shot.Entries = append(shot.Entries, snapshotEntry{Path: target, Mode: info.Mode(), Data: data})
		return shot, nil
	}
	err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entryInfo, err := d.Info()
		if err != nil {
			return err
		}
		entry := snapshotEntry{Path: path, Mode: entryInfo.Mode(), IsDir: d.IsDir()}
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.Data = data
		}
		shot.Entries = append(shot.Entries, entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("capture snapshot: %w", err)
	}
	return shot, nil
}

func restoreSnapshot(shot *snapshot) error {
	if shot == nil {
		return nil
	}
	if err := os.RemoveAll(shot.Target); err != nil {
		return err
	}
	if len(shot.Entries) == 0 {
		return nil
	}
	for _, entry := range shot.Entries {
		if entry.IsDir {
			if err := os.MkdirAll(entry.Path, entry.Mode.Perm()); err != nil {
				return err
			}
			if err := os.Chmod(entry.Path, entry.Mode); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(entry.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(entry.Path, entry.Data, entry.Mode.Perm()); err != nil {
			return err
		}
		if err := os.Chmod(entry.Path, entry.Mode); err != nil {
			return err
		}
	}
	return nil
}

func applyOperations(content string, operations []PatchOperation) (string, []string, error) {
	updated := content
	details := make([]string, 0, len(operations))
	for i, operation := range operations {
		if operation.Old == "" {
			return "", nil, fmt.Errorf("operation %d has empty old text", i+1)
		}
		occurrences := strings.Count(updated, operation.Old)
		if occurrences == 0 {
			return "", nil, fmt.Errorf("operation %d could not find target text", i+1)
		}
		if operation.ReplaceAll {
			updated = strings.ReplaceAll(updated, operation.Old, operation.New)
			details = append(details, fmt.Sprintf("operation %d replaced %d occurrence(s)", i+1, occurrences))
			continue
		}
		updated = strings.Replace(updated, operation.Old, operation.New, 1)
		details = append(details, fmt.Sprintf("operation %d replaced first of %d occurrence(s)", i+1, occurrences))
	}
	return updated, details, nil
}

func decodeArgs(raw []byte, target any) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		trimmed = "{}"
	}
	if err := json.Unmarshal([]byte(trimmed), target); err != nil {
		return fmt.Errorf("decode tool arguments: %w", err)
	}
	return nil
}

func encodeResult(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fallback, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return string(fallback)
	}
	return string(data)
}

func requestConfirmation(confirm ConfirmFunc, prompt string) error {
	if confirm == nil {
		return fmt.Errorf("confirmation is required but unavailable")
	}
	ok, err := confirm(prompt)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("user declined action")
	}
	return nil
}

func normalizeExecutable(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("empty command")
	}
	var token string
	if strings.HasPrefix(command, "\"") || strings.HasPrefix(command, "'") {
		quote := command[0]
		rest := command[1:]
		idx := strings.IndexByte(rest, quote)
		if idx < 0 {
			token = rest
		} else {
			token = rest[:idx]
		}
	} else {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return "", fmt.Errorf("empty command")
		}
		token = parts[0]
	}
	token = strings.Trim(token, "\"'")
	token = strings.ToLower(filepath.Base(token))
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		token = strings.TrimSuffix(token, suffix)
	}
	if token == "" {
		return "", fmt.Errorf("could not determine executable")
	}
	return token, nil
}

func defaultPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "."
	}
	return path
}

func limitString(value string, maxChars int) (string, bool) {
	if maxChars <= 0 || len(value) <= maxChars {
		return value, false
	}
	if maxChars <= 3 {
		return value[:maxChars], true
	}
	return value[:maxChars-3] + "...", true
}

func truncateLine(value string, maxChars int) string {
	trimmed := strings.TrimSpace(value)
	limited, _ := limitString(trimmed, maxChars)
	return limited
}

func toolDef(name, description string, params map[string]any) llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  params,
		},
	}
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func intProp(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

type staticDirEntry struct {
	info fs.FileInfo
}

func dirEntryFromInfo(info fs.FileInfo) fs.DirEntry {
	return staticDirEntry{info: info}
}

func (s staticDirEntry) Name() string               { return s.info.Name() }
func (s staticDirEntry) IsDir() bool                { return s.info.IsDir() }
func (s staticDirEntry) Type() fs.FileMode          { return s.info.Mode().Type() }
func (s staticDirEntry) Info() (fs.FileInfo, error) { return s.info, nil }

var errSearchDone = errors.New("search complete")
