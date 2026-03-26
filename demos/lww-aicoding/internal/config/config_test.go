package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesEnvOverrides(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("AICODING_MODEL", "override-model")
	t.Setenv("AICODING_API_KEY", "secret")
	t.Setenv("AICODING_PROVIDER_TYPE", "anthropic")
	t.Setenv("AICODING_STREAM", "false")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "override-model" {
		t.Fatalf("expected override model, got %q", cfg.Provider.Model)
	}
	if cfg.Provider.Type != "anthropic" {
		t.Fatalf("expected anthropic provider, got %q", cfg.Provider.Type)
	}
	if cfg.Stream {
		t.Fatalf("expected stream override to disable streaming")
	}
	if cfg.MaxIterations != 32 {
		t.Fatalf("expected default max iterations 32, got %d", cfg.MaxIterations)
	}
	if cfg.Provider.ResolveAPIKey() != "secret" {
		t.Fatalf("expected api key from env")
	}
	if filepath.Dir(cfg.SessionDir) != filepath.Join(workspace, ".aicoding") {
		t.Fatalf("unexpected session dir: %s", cfg.SessionDir)
	}
}

func TestResolveConfigPathExplicit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveConfigPath(t.TempDir(), file)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected explicit path")
	}
}

func TestLoadUsesWorkspaceRootConfigFile(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "aicoding.config.json")
	data, err := json.Marshal(map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"approval_policy": "never",
		"max_iterations":  16,
		"stream":          false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.ResolveAPIKey() != "test-key" {
		t.Fatalf("expected api key from workspace root config, got %q", cfg.Provider.ResolveAPIKey())
	}
	if cfg.Provider.Model != "gpt-5.4-mini" {
		t.Fatalf("expected model from workspace root config, got %q", cfg.Provider.Model)
	}
	if cfg.ApprovalPolicy != "never" {
		t.Fatalf("expected approval policy never, got %q", cfg.ApprovalPolicy)
	}
	if cfg.MaxIterations != 16 {
		t.Fatalf("expected max iterations 16, got %d", cfg.MaxIterations)
	}
	if cfg.Stream {
		t.Fatalf("expected stream false from workspace root config")
	}
}
