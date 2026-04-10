package tui

import tuidialog "bytemind/internal/tui/dialog"

func parseStartupConfigInput(raw string) (field, value string, ok bool) {
	return tuidialog.ParseStartupConfigInput(raw)
}

func sanitizeAPIKeyInput(raw string) string {
	return tuidialog.SanitizeAPIKeyInput(raw)
}

func normalizeStartupProviderType(value string) (string, bool) {
	return tuidialog.NormalizeStartupProviderType(value)
}

func parseSkillArgs(parts []string) (map[string]string, error) {
	return tuidialog.ParseSkillArgs(parts)
}

func parsePromptSearchQuery(raw string) (tokens []string, workspaceFilter, sessionFilter string) {
	return tuidialog.ParsePromptSearchQuery(raw)
}
