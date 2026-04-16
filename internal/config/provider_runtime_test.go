package config

import "testing"

func TestLegacyProviderRuntimeConfigNormalizesProviderIDs(t *testing.T) {
	tests := []struct {
		name      string
		typeValue string
		want      string
	}{
		{name: "openai compatible", typeValue: "openai-compatible", want: "openai"},
		{name: "openai alias", typeValue: "openai", want: "openai"},
		{name: "empty defaults openai", typeValue: "", want: "openai"},
		{name: "whitespace defaults openai", typeValue: "   ", want: "openai"},
		{name: "anthropic", typeValue: "anthropic", want: "anthropic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ProviderConfig{Type: tt.typeValue, Model: "test-model"}
			runtime := LegacyProviderRuntimeConfig(cfg)
			if runtime.DefaultProvider != tt.want {
				t.Fatalf("unexpected default provider %q", runtime.DefaultProvider)
			}
			if runtime.DefaultModel != "test-model" {
				t.Fatalf("unexpected default model %q", runtime.DefaultModel)
			}
			if len(runtime.Providers) != 1 || runtime.Providers[tt.want].Type != tt.typeValue {
				t.Fatalf("unexpected providers %#v", runtime.Providers)
			}
		})
	}
}
