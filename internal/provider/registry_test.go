package provider

import (
	"context"
	"errors"
	"testing"

	"bytemind/internal/config"
)

type stubRegistryClient struct {
	providerID ProviderID
	models     []ModelInfo
	err        error
}

func (s stubRegistryClient) ProviderID() ProviderID                                { return s.providerID }
func (s stubRegistryClient) ListModels(context.Context) ([]ModelInfo, error)       { return s.models, s.err }
func (s stubRegistryClient) Stream(context.Context, Request) (<-chan Event, error) { return nil, nil }

func TestNewRegistryFromProviderConfigSupportsLegacyMode(t *testing.T) {
	reg, err := NewRegistryFromProviderConfig(config.ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	ids, err := reg.List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids) != 1 || ids[0] != ProviderOpenAI {
		t.Fatalf("unexpected ids %#v", ids)
	}
}

func TestRegistryRejectsDuplicateProvider(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI}); err != nil {
		t.Fatalf("unexpected first register error %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI}); err == nil {
		t.Fatal("expected duplicate provider error")
	} else {
		var providerErr *Error
		if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeDuplicateProvider {
			t.Fatalf("unexpected error %#v", err)
		}
	}
}

func TestListModelsAggregatesWarningsAndDeduplicates(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI, models: []ModelInfo{{ProviderID: ProviderOpenAI, ModelID: "gpt-5.4"}, {ProviderID: ProviderOpenAI, ModelID: "gpt-5.4"}}}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderAnthropic, err: errors.New("list failed")}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	models, warnings, err := ListModels(context.Background(), reg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(models) != 1 || models[0].ProviderID != ProviderOpenAI || models[0].ModelID != "gpt-5.4" {
		t.Fatalf("unexpected models %#v", models)
	}
	if len(warnings) != 1 || warnings[0].ProviderID != ProviderAnthropic {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
}
