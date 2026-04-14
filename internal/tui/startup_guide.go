package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"bytemind/internal/config"
	"bytemind/internal/provider"
)

func (m *model) handleStartupGuideSubmission(rawInput string) error {
	rawInput = strings.TrimSpace(rawInput)

	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	if explicitField, explicitValue, ok := parseStartupConfigInput(rawInput); ok {
		field = explicitField
		rawInput = explicitValue
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		value, err := m.resolveStartupFieldValue(field, rawInput)
		if err != nil {
			return err
		}
		if err := m.applyStartupConfigField(field, value); err != nil {
			return err
		}
		next := startupNextField(field)
		if next == "" {
			next = startupFieldAPIKey
		}
		m.setStartupGuideStep(next, "")
		m.input.Reset()
		return nil
	case startupFieldAPIKey:
		return m.verifyAndFinalizeStartupAPIKey(rawInput)
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}
}

func (m *model) verifyAndFinalizeStartupAPIKey(rawInput string) error {
	apiKey := sanitizeAPIKeyInput(rawInput)
	if apiKey == "" {
		return fmt.Errorf("please paste a non-empty API key")
	}

	checkCfg := m.cfg.Provider
	checkCfg.APIKey = apiKey
	check := provider.CheckAvailability(context.Background(), checkCfg)
	if !check.Ready {
		m.llmConnected = false
		m.phase = "error"
		m.setStartupGuideStep(startupFieldAPIKey, startupGuideIssueHint(check))
		return nil
	}

	writtenPath, saveErr := config.UpsertProviderAPIKey(m.startupGuide.ConfigPath, apiKey)

	if envName := strings.TrimSpace(checkCfg.APIKeyEnv); envName != "" {
		if err := os.Setenv(envName, apiKey); err != nil {
			warnSetenv(envName, err)
		}
	} else {
		if err := os.Setenv("BYTEMIND_API_KEY", apiKey); err != nil {
			warnSetenv("BYTEMIND_API_KEY", err)
		}
	}

	client, err := provider.NewClient(checkCfg)
	if err != nil {
		return err
	}
	if m.runner != nil {
		m.runner.UpdateProvider(checkCfg, client)
	}
	m.cfg.Provider = checkCfg
	m.startupGuide.Active = false
	m.statusNote = "Provider configured and verified. You can start chatting."
	m.llmConnected = true
	m.phase = "idle"
	if saveErr != nil {
		m.statusNote = "Provider verified, but config save failed: " + compact(saveErr.Error(), 80)
	} else if strings.TrimSpace(writtenPath) != "" {
		m.statusNote = "Provider configured and verified. Saved to " + compact(writtenPath, 48)
	}
	m.syncInputStyle()
	m.input.Reset()
	return nil
}

func (m *model) applyStartupConfigField(field, value string) error {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	persistValue := value
	previousRecommendedModel := startupProviderDefaultModel(m.cfg.Provider.Type, m.cfg.Provider.BaseURL)

	switch field {
	case "model":
		m.cfg.Provider.Model = value
	case "base_url":
		m.cfg.Provider.BaseURL = value
	case "type":
		normalized, ok := normalizeStartupProviderType(value)
		if !ok {
			return fmt.Errorf("provider must be openai-compatible or anthropic")
		}
		m.cfg.Provider.Type = normalized
		persistValue = normalized
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}

	writtenPath, err := config.UpsertProviderField(m.startupGuide.ConfigPath, field, persistValue)
	if err != nil {
		return err
	}
	if strings.TrimSpace(writtenPath) != "" {
		m.startupGuide.ConfigPath = writtenPath
	}

	// If provider/base_url changes and model still matches previous default,
	// auto-switch to a better provider-specific default (for example DeepSeek).
	if field != "model" {
		currentModel := strings.TrimSpace(m.cfg.Provider.Model)
		nextRecommendedModel := startupProviderDefaultModel(m.cfg.Provider.Type, m.cfg.Provider.BaseURL)
		lowerCurrent := strings.ToLower(currentModel)
		legacyOpenAIDefault := strings.HasPrefix(lowerCurrent, "gpt-")
		if nextRecommendedModel != "" &&
			(currentModel == "" || currentModel == previousRecommendedModel || (nextRecommendedModel == "deepseek-chat" && legacyOpenAIDefault)) &&
			currentModel != nextRecommendedModel {
			m.cfg.Provider.Model = nextRecommendedModel
			modelPath, modelErr := config.UpsertProviderField(m.startupGuide.ConfigPath, "model", nextRecommendedModel)
			if modelErr != nil {
				return modelErr
			}
			if strings.TrimSpace(modelPath) != "" {
				m.startupGuide.ConfigPath = modelPath
			}
		}
	}
	return nil
}

func parseStartupConfigInput(raw string) (field, value string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if lower == "" {
		return "", "", false
	}

	parse := func(alias, normalized string) (string, string, bool) {
		for _, sep := range []string{"=", ":"} {
			prefix := alias + sep
			if strings.HasPrefix(lower, prefix) {
				val := strings.TrimSpace(trimmed[len(prefix):])
				return normalized, val, true
			}
		}
		return "", "", false
	}

	for _, candidate := range []struct {
		alias      string
		normalized string
	}{
		{alias: "model", normalized: "model"},
		{alias: "base_url", normalized: "base_url"},
		{alias: "baseurl", normalized: "base_url"},
		{alias: "base-url", normalized: "base_url"},
		{alias: "provider", normalized: "type"},
		{alias: "type", normalized: "type"},
		{alias: "provider_type", normalized: "type"},
		{alias: "api_key", normalized: "api_key"},
		{alias: "apikey", normalized: "api_key"},
		{alias: "key", normalized: "api_key"},
	} {
		if field, value, ok := parse(candidate.alias, candidate.normalized); ok {
			return field, value, true
		}
	}

	return "", "", false
}

func sanitizeAPIKeyInput(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, "\"'")
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "authorization: bearer ") {
		value = strings.TrimSpace(value[len("authorization: bearer "):])
	}
	if strings.HasPrefix(lower, "bearer ") {
		value = strings.TrimSpace(value[len("bearer "):])
	}
	return strings.TrimSpace(value)
}

func normalizeStartupProviderType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai-compatible", "openai_compatible", "openai":
		return "openai-compatible", true
	case "anthropic":
		return "anthropic", true
	default:
		return "", false
	}
}

func isStartupGuideField(field string) bool {
	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel, startupFieldAPIKey:
		return true
	default:
		return false
	}
}

func startupNextField(current string) string {
	for i, field := range startupFieldOrder {
		if field == current {
			if i+1 >= len(startupFieldOrder) {
				return ""
			}
			return startupFieldOrder[i+1]
		}
	}
	return startupFieldType
}

func startupFieldStep(field string) (int, int) {
	for i, item := range startupFieldOrder {
		if item == field {
			return i + 1, len(startupFieldOrder)
		}
	}
	return 1, len(startupFieldOrder)
}

func startupFieldName(field string) string {
	switch field {
	case startupFieldType:
		return "provider"
	case startupFieldBaseURL:
		return "base_url"
	case startupFieldModel:
		return "model"
	case startupFieldAPIKey:
		return "api_key"
	default:
		return field
	}
}

func startupProviderDefaultBaseURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return "https://api.openai.com/v1"
	}
}

func startupProviderDefaultModel(providerType, baseURL string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return ""
	default:
		if strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "deepseek.com") {
			return "deepseek-chat"
		}
		return "gpt-5.4-mini"
	}
}

func (m model) startupCurrentValue(field string) string {
	switch field {
	case startupFieldType:
		return strings.TrimSpace(m.cfg.Provider.Type)
	case startupFieldBaseURL:
		return strings.TrimSpace(m.cfg.Provider.BaseURL)
	case startupFieldModel:
		return strings.TrimSpace(m.cfg.Provider.Model)
	default:
		return ""
	}
}

func (m *model) resolveStartupFieldValue(field, rawInput string) (string, error) {
	value := strings.TrimSpace(rawInput)
	if value != "" {
		return value, nil
	}

	current := m.startupCurrentValue(field)
	if current != "" {
		return current, nil
	}

	switch field {
	case startupFieldType:
		return "openai-compatible", nil
	case startupFieldBaseURL:
		return startupProviderDefaultBaseURL(m.cfg.Provider.Type), nil
	case startupFieldModel:
		if fallback := startupProviderDefaultModel(m.cfg.Provider.Type, m.cfg.Provider.BaseURL); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("please enter model name for provider %s", strings.TrimSpace(m.cfg.Provider.Type))
	default:
		return "", fmt.Errorf("%s cannot be empty", startupFieldName(field))
	}
}

func (m *model) initializeStartupGuide() {
	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	m.setStartupGuideStep(field, "")
}

func (m *model) setStartupGuideStep(field, issue string) {
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	step, total := startupFieldStep(field)
	fieldName := startupFieldName(field)
	if strings.TrimSpace(issue) == "" {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s.", step, total, fieldName)
	} else {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s. %s", step, total, fieldName, issue)
	}
	m.statusNote = m.startupGuide.Status
	m.startupGuide.CurrentField = field
	m.startupGuide.Lines = startupGuideStepLines(field, m.cfg, m.startupGuide.ConfigPath, issue)
	m.syncInputStyle()
}

func startupGuideStepLines(field string, cfg config.Config, configPath, issue string) []string {
	lines := make([]string, 0, 8)
	switch field {
	case startupFieldType:
		lines = append(lines, "Enter provider: openai-compatible or anthropic.")
	case startupFieldBaseURL:
		lines = append(lines, "Enter provider base_url.")
		lines = append(lines, "Example: https://api.deepseek.com")
	case startupFieldModel:
		lines = append(lines, "Enter model name.")
		lines = append(lines, "Example: deepseek-chat or gpt-5.4-mini")
	case startupFieldAPIKey:
		lines = append(lines, "Paste API key and press Enter.")
		lines = append(lines, "Bytemind will verify it automatically.")
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		current := ""
		switch field {
		case startupFieldType:
			current = strings.TrimSpace(cfg.Provider.Type)
		case startupFieldBaseURL:
			current = strings.TrimSpace(cfg.Provider.BaseURL)
		case startupFieldModel:
			current = strings.TrimSpace(cfg.Provider.Model)
		}
		if current == "" {
			lines = append(lines, "Press Enter to use default.")
		} else {
			lines = append(lines, "Press Enter to keep current: "+current)
		}
	}
	if strings.TrimSpace(issue) != "" {
		lines = append(lines, "Issue: "+issue)
	}
	if strings.TrimSpace(configPath) != "" {
		lines = append(lines, "Config file: "+configPath)
	}
	return lines
}

func startupGuideIssueHint(check provider.Availability) string {
	reason := strings.ToLower(strings.TrimSpace(check.Reason))
	switch {
	case strings.Contains(reason, "missing api key"):
		return "No API key is configured yet."
	case strings.Contains(reason, "unauthorized"):
		return "The API key was rejected by the provider."
	case strings.Contains(reason, "failed to reach"):
		return "Cannot reach provider endpoint. Check proxy or network."
	case strings.Contains(reason, "not found"):
		return "Provider endpoint path looks incorrect."
	default:
		if strings.TrimSpace(check.Reason) == "" {
			return "Provider check failed."
		}
		return compact(strings.TrimSpace(check.Reason), 90)
	}
}

func startupGuideInputHint(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Enter provider and press Enter."
	case startupFieldBaseURL:
		return "Enter base_url and press Enter."
	case startupFieldModel:
		return "Enter model and press Enter."
	case startupFieldAPIKey:
		return "Paste API key and press Enter to verify."
	default:
		return "Input value then press Enter."
	}
}

func startupGuideInputPlaceholder(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Step 1/4: provider (openai-compatible or anthropic)"
	case startupFieldBaseURL:
		return "Step 2/4: base_url (example: https://api.deepseek.com)"
	case startupFieldModel:
		return "Step 3/4: model (example: deepseek-chat)"
	case startupFieldAPIKey:
		return "Step 4/4: paste API key and press Enter..."
	default:
		return "Complete setup and press Enter..."
	}
}
