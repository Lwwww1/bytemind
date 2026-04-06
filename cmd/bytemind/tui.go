package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/secretstore"
	"bytemind/internal/tui"
)

var runTUIProgram = tui.Run

const defaultAPIKeyEnvName = "BYTEMIND_API_KEY"

func runTUI(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Resume an existing session")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	workspace, err := resolveWorkspace(*workspaceOverride)
	if err != nil {
		return err
	}
	if err := ensureAPIConfigForTUI(workspace, *configPath, stdin, stdout); err != nil {
		return err
	}

	app, store, sess, err := bootstrap(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	if err != nil {
		return err
	}

	cfg, err := config.Load(workspace, *configPath)
	if err != nil {
		return err
	}
	if *model != "" {
		cfg.Provider.Model = *model
	}
	if *streamOverride != "" {
		parsed, err := strconv.ParseBool(*streamOverride)
		if err != nil {
			return err
		}
		cfg.Stream = parsed
	}
	if *maxIterations > 0 {
		cfg.MaxIterations = *maxIterations
	}
	home, err := config.EnsureHomeLayout()
	if err != nil {
		return err
	}
	imageStore, err := assets.NewFileAssetStore(home)
	if err != nil {
		return err
	}

	return runTUIProgram(tui.Options{
		Runner:     app,
		Store:      store,
		Session:    sess,
		ImageStore: imageStore,
		Config:     cfg,
		Workspace:  sess.Workspace,
	})
}

func ensureAPIConfigForTUI(workspace, configPath string, stdin io.Reader, stdout io.Writer) error {
	cfg, err := config.Load(workspace, configPath)
	if err != nil {
		if strings.TrimSpace(configPath) != "" && errors.Is(err, os.ErrNotExist) {
			return runInteractiveConfigSetup(workspace, configPath, config.Default(workspace), stdin, stdout)
		}
		return err
	}

	inlineKey, inlinePath, err := inlineAPIKeyFromConfigFile(workspace, configPath)
	if err != nil {
		return err
	}
	if inlineKey != "" {
		if err := migrateInlineAPIKeyToEnv(inlinePath, &cfg, inlineKey, stdout); err != nil {
			return err
		}
		return nil
	}

	if strings.TrimSpace(cfg.Provider.ResolveAPIKey()) == "" {
		return runInteractiveConfigSetup(workspace, configPath, cfg, stdin, stdout)
	}
	return nil
}

func runInteractiveConfigSetup(workspace, configPath string, cfg config.Config, stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	fmt.Fprintln(stdout, "\u672A\u68C0\u6D4B\u5230\u53EF\u7528 API \u914D\u7F6E\uFF0C\u8BF7\u5148\u5B8C\u6210\u521D\u59CB\u5316\u3002")
	fmt.Fprintln(stdout, "\u914D\u7F6E\u683C\u5F0F\uFF1AOpenAI-compatible\uFF08\u4F9D\u6B21\u8F93\u5165 url / key / model\uFF09\u3002")

	baseURL, err := promptSetupValue(reader, stdout, "url")
	if err != nil {
		return err
	}
	apiKey, err := promptSecretValue(reader, stdin, stdout, "key")
	if err != nil {
		return err
	}
	modelName, err := promptSetupValue(reader, stdout, "model")
	if err != nil {
		return err
	}

	baseURL, err = validateBaseURL(baseURL)
	if err != nil {
		return err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("\u914D\u7F6E\u5931\u8D25: API key \u4E0D\u80FD\u4E3A\u7A7A")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return errors.New("\u914D\u7F6E\u5931\u8D25: model \u4E0D\u80FD\u4E3A\u7A7A")
	}
	envName := strings.TrimSpace(cfg.Provider.APIKeyEnv)
	if envName == "" {
		envName = defaultAPIKeyEnvName
	}
	if err := os.Setenv(envName, apiKey); err != nil {
		return fmt.Errorf("\u914D\u7F6E\u5931\u8D25: \u65E0\u6CD5\u8BBE\u7F6E\u73AF\u5883\u53D8\u91CF %s: %w", envName, err)
	}
	if err := secretstore.Save(envName, apiKey); err != nil {
		return fmt.Errorf("\u914D\u7F6E\u5931\u8D25: \u65E0\u6CD5\u5B89\u5168\u4FDD\u5B58 API key: %w", err)
	}

	cfg.Provider.Type = "openai-compatible"
	cfg.Provider.AutoDetectType = false
	cfg.Provider.BaseURL = baseURL
	cfg.Provider.APIPath = ""
	cfg.Provider.Model = modelName
	cfg.Provider.APIKey = ""
	cfg.Provider.APIKeyEnv = envName
	cfg.Provider.AuthHeader = ""
	cfg.Provider.AuthScheme = ""
	cfg.Provider.ExtraHeaders = nil

	targetPath, err := resolveSetupConfigPath(workspace, configPath)
	if err != nil {
		return err
	}
	if err := writeConfigFile(targetPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\u914D\u7F6E\u5DF2\u5199\u5165 %s\n", targetPath)
	fmt.Fprintf(stdout, "\u5DF2\u5C06 API key \u6CE8\u5165\u73AF\u5883\u53D8\u91CF %s\uFF08\u4EC5\u5F53\u524D\u8FDB\u7A0B\uFF09\u3002\n", envName)
	fmt.Fprintln(stdout, "\u5DF2\u5B89\u5168\u4FDD\u5B58 API key\uFF0C\u540E\u7EED\u542F\u52A8\u65E0\u9700\u518D\u8F93\u5165\u3002")
	return nil
}

func promptSetupValue(reader *bufio.Reader, stdout io.Writer, label string) (string, error) {
	fmt.Fprintf(stdout, "%s: ", strings.TrimSpace(label))

	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		if errors.Is(err, io.EOF) {
			return "", errors.New("\u521D\u59CB\u5316\u5DF2\u53D6\u6D88: \u672A\u6536\u5230\u8F93\u5165")
		}
		return "", fmt.Errorf("\u914D\u7F6E\u5931\u8D25: %s \u4E0D\u80FD\u4E3A\u7A7A", label)
	}
	return line, nil
}

func promptSecretValue(reader *bufio.Reader, _ io.Reader, stdout io.Writer, label string) (string, error) {
	fmt.Fprintf(stdout, "%s: ", strings.TrimSpace(label))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		if errors.Is(err, io.EOF) {
			return "", errors.New("\u521D\u59CB\u5316\u5DF2\u53D6\u6D88: \u672A\u6536\u5230\u8F93\u5165")
		}
		return "", fmt.Errorf("\u914D\u7F6E\u5931\u8D25: %s \u4E0D\u80FD\u4E3A\u7A7A", label)
	}
	return line, nil
}

func validateBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("\u914D\u7F6E\u5931\u8D25: url \u4E0D\u80FD\u4E3A\u7A7A")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("\u914D\u7F6E\u5931\u8D25: url \u5FC5\u987B\u662F\u5408\u6CD5\u5730\u5740")
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if scheme != "https" && !(scheme == "http" && isLocalHost(host)) {
		return "", errors.New("\u914D\u7F6E\u5931\u8D25: url \u5FC5\u987B\u4F7F\u7528 https\uff08localhost/127.0.0.1 \u5141\u8BB8 http\uff09")
	}
	return strings.TrimRight(value, "/"), nil
}

func isLocalHost(host string) bool {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func resolveSetupConfigPath(workspace, configPath string) (string, error) {
	if strings.TrimSpace(configPath) != "" {
		return filepath.Abs(configPath)
	}
	return filepath.Join(workspace, "config.json"), nil
}

func writeConfigFile(path string, cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func inlineAPIKeyFromConfigFile(workspace, configPath string) (string, string, error) {
	configFile, err := resolveExistingConfigPath(workspace, configPath)
	if err != nil || configFile == "" {
		return "", configFile, err
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", configFile, nil
		}
		return "", configFile, err
	}
	var raw struct {
		Provider struct {
			APIKey string `json:"api_key"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", configFile, err
	}
	return strings.TrimSpace(raw.Provider.APIKey), configFile, nil
}

func resolveExistingConfigPath(workspace, configPath string) (string, error) {
	if strings.TrimSpace(configPath) != "" {
		return filepath.Abs(configPath)
	}

	candidates := []string{
		filepath.Join(workspace, "config.json"),
		filepath.Join(workspace, ".bytemind", "config.json"),
		filepath.Join(workspace, "bytemind.config.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	home, err := config.ResolveHomeDir()
	if err != nil {
		return "", err
	}
	homeConfig := filepath.Join(home, "config.json")
	if _, err := os.Stat(homeConfig); err == nil {
		return homeConfig, nil
	}
	return "", nil
}

func migrateInlineAPIKeyToEnv(configFile string, cfg *config.Config, inlineAPIKey string, stdout io.Writer) error {
	if cfg == nil {
		return errors.New("missing config")
	}
	inlineAPIKey = strings.TrimSpace(inlineAPIKey)
	if inlineAPIKey == "" {
		return nil
	}
	envName := strings.TrimSpace(cfg.Provider.APIKeyEnv)
	if envName == "" {
		envName = defaultAPIKeyEnvName
	}
	if err := os.Setenv(envName, inlineAPIKey); err != nil {
		return fmt.Errorf("failed to set environment variable %s: %w", envName, err)
	}
	if err := secretstore.Save(envName, inlineAPIKey); err != nil {
		return fmt.Errorf("failed to persist API key: %w", err)
	}
	cfg.Provider.APIKey = ""
	cfg.Provider.APIKeyEnv = envName
	if err := writeConfigFile(configFile, *cfg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\u68C0\u6D4B\u5230\u660E\u6587 provider.api_key\uFF0C\u5DF2\u8FC1\u79FB\u5230\u73AF\u5883\u53D8\u91CF %s\uFF08\u5F53\u524D\u8FDB\u7A0B\uFF09\u3002\n", envName)
	fmt.Fprintln(stdout, "\u5DF2\u5B89\u5168\u4FDD\u5B58 API key\uFF0C\u540E\u7EED\u542F\u52A8\u65E0\u9700\u518D\u8F93\u5165\u3002")
	return nil
}
