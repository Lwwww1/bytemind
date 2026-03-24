package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultBaseURL         = "https://api.openai.com/v1"
	defaultModel           = "gpt-4.1-mini"
	defaultMaxTurns        = 10
	defaultAllowedCommands = "go,git,npm,pnpm,bun,node,python,pytest"
)

type Config struct {
	Workspace       string
	BaseURL         string
	Model           string
	APIKey          string
	MaxTurns        int
	AllowedCommands []string
}

func Load(workspaceFlag string) (Config, error) {
	workspace := strings.TrimSpace(workspaceFlag)
	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Config{}, fmt.Errorf("get working directory: %w", err)
		}
		workspace = cwd
	}

	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return Config{}, fmt.Errorf("resolve workspace: %w", err)
	}

	info, err := os.Stat(absWorkspace)
	if err != nil {
		return Config{}, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("workspace is not a directory: %s", absWorkspace)
	}

	maxTurns := defaultMaxTurns
	if raw := strings.TrimSpace(os.Getenv("GOCODE_MAX_TURNS")); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value <= 0 {
			return Config{}, fmt.Errorf("invalid GOCODE_MAX_TURNS: %q", raw)
		}
		maxTurns = value
	}

	allowed := parseCSV(os.Getenv("GOCODE_ALLOWED_COMMANDS"))
	if len(allowed) == 0 {
		allowed = parseCSV(defaultAllowedCommands)
	}

	return Config{
		Workspace:       absWorkspace,
		BaseURL:         firstNonEmpty(os.Getenv("OPENAI_BASE_URL"), defaultBaseURL),
		Model:           firstNonEmpty(os.Getenv("OPENAI_MODEL"), defaultModel),
		APIKey:          strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		MaxTurns:        maxTurns,
		AllowedCommands: allowed,
	}, nil
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
