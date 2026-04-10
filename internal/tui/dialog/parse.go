package dialog

import (
	"fmt"
	"strings"
)

func ParseStartupConfigInput(raw string) (field, value string, ok bool) {
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

func SanitizeAPIKeyInput(raw string) string {
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

func NormalizeStartupProviderType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai-compatible", "openai_compatible", "openai":
		return "openai-compatible", true
	case "anthropic":
		return "anthropic", true
	default:
		return "", false
	}
}

func ParseSkillArgs(parts []string) (map[string]string, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	args := make(map[string]string, len(parts))
	for _, part := range parts {
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("invalid skill arg %q, expected k=v", part)
		}
		key := strings.TrimSpace(pieces[0])
		value := strings.TrimSpace(pieces[1])
		if key == "" || value == "" {
			return nil, fmt.Errorf("invalid skill arg %q, expected k=v", part)
		}
		args[key] = value
	}
	if len(args) == 0 {
		return nil, nil
	}
	return args, nil
}

func ParsePromptSearchQuery(raw string) (tokens []string, workspaceFilter, sessionFilter string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	tokens = make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		switch {
		case strings.HasPrefix(field, "ws:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "ws:"))
		case strings.HasPrefix(field, "workspace:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "workspace:"))
		case strings.HasPrefix(field, "sid:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "sid:"))
		case strings.HasPrefix(field, "session:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "session:"))
		default:
			tokens = append(tokens, field)
		}
	}
	return tokens, workspaceFilter, sessionFilter
}
