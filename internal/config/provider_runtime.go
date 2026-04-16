package config

type ProviderRuntimeConfig struct {
	DefaultProvider string                    `json:"default_provider"`
	DefaultModel    string                    `json:"default_model"`
	Providers       map[string]ProviderConfig `json:"providers"`
}

func LegacyProviderRuntimeConfig(cfg ProviderConfig) ProviderRuntimeConfig {
	providerID := cfg.Type
	if providerID == "openai-compatible" || providerID == "openai" || providerID == "" {
		providerID = "openai"
	}
	if providerID == "anthropic" {
		providerID = "anthropic"
	}
	return ProviderRuntimeConfig{
		DefaultProvider: providerID,
		DefaultModel:    cfg.Model,
		Providers: map[string]ProviderConfig{
			providerID: cfg,
		},
	}
}
