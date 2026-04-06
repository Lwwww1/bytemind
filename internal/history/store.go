package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bytemind/internal/config"
)

const (
	defaultRecentLimit  = 5000
	promptHistoryFile   = "prompt_history.jsonl"
	scannerMaxLineBytes = 1024 * 1024
)

type PromptEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Workspace string    `json:"workspace"`
	SessionID string    `json:"session_id"`
	Prompt    string    `json:"prompt"`
}

var appendMu sync.Mutex

func AppendPrompt(workspace, sessionID, prompt string, at time.Time) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	path, err := historyFilePath()
	if err != nil {
		return err
	}

	entry := PromptEntry{
		Timestamp: at,
		Workspace: strings.TrimSpace(workspace),
		SessionID: strings.TrimSpace(sessionID),
		Prompt:    prompt,
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	appendMu.Lock()
	defer appendMu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func LoadRecentPrompts(limit int) ([]PromptEntry, error) {
	if limit <= 0 {
		limit = defaultRecentLimit
	}

	path, err := historyFilePath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	entries := make([]PromptEntry, 0, minInt(limit, 256))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerMaxLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry PromptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entry.Prompt = strings.TrimSpace(entry.Prompt)
		if entry.Prompt == "" {
			continue
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now().UTC()
		} else {
			entry.Timestamp = entry.Timestamp.UTC()
		}

		entries = append(entries, entry)
		if len(entries) > limit {
			copy(entries, entries[len(entries)-limit:])
			entries = entries[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func historyFilePath() (string, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return "", err
	}

	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, promptHistoryFile), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
