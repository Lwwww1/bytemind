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

type stubListRegistry struct {
	listErr error
}

func (s stubListRegistry) Register(context.Context, Client) error         { return nil }
func (s stubListRegistry) Get(context.Context, ProviderID) (Client, bool) { return nil, false }
func (s stubListRegistry) List(context.Context) ([]ProviderID, error)     { return nil, s.listErr }

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
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderOpenAI, models: []ModelInfo{{ProviderID: ProviderOpenAI, ModelID: "gpt-5.4"}, {ProviderID: ProviderOpenAI, ModelID: "gpt-5.4"}, {ProviderID: "", ModelID: "gpt-4.1"}}}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ProviderAnthropic, err: errors.New("list failed")}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	models, warnings, err := ListModels(context.Background(), reg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(models) != 2 || models[0].ProviderID != ProviderOpenAI || models[0].ModelID != "gpt-4.1" || models[1].ProviderID != ProviderOpenAI || models[1].ModelID != "gpt-5.4" {
		t.Fatalf("unexpected models %#v", models)
	}
	if len(warnings) != 1 || warnings[0].ProviderID != ProviderAnthropic || warnings[0].Reason != listModelsWarningReason {
		t.Fatalf("unexpected warnings %#v", warnings)
	}
}

func TestListModelsReturnsRegistryError(t *testing.T) {
	reg := stubListRegistry{listErr: errors.New("boom")}
	if _, _, err := ListModels(context.Background(), reg); err == nil {
		t.Fatal("expected registry list error")
	}
}

func TestRegistryCoversLookupAndNormalizationBranches(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: "openai-compatible"}); err != nil {
		t.Fatalf("unexpected register error %v", err)
	}
	client, ok := reg.Get(context.Background(), "openai")
	if !ok || client.ProviderID() != "openai-compatible" {
		t.Fatalf("unexpected client lookup result ok=%v client=%#v", ok, client)
	}
	if _, ok := reg.Get(context.Background(), "missing"); ok {
		t.Fatal("expected missing provider lookup to fail")
	}
	ids, err := reg.List(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != ProviderOpenAI {
		t.Fatalf("unexpected ids %#v err=%v", ids, err)
	}
}

func TestRegistryHandlesProviderNotFoundAndConfigErrors(t *testing.T) {
	reg, err := NewRegistry(config.ProviderRuntimeConfig{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := reg.Register(context.Background(), nil); err != nil {
		t.Fatalf("expected nil client register to be ignored, got %v", err)
	}
	if err := reg.Register(context.Background(), stubRegistryClient{providerID: ""}); err == nil {
		t.Fatal("expected provider not found error")
	} else {
		var providerErr *Error
		if !errors.As(err, &providerErr) || providerErr.Code != ErrCodeProviderNotFound {
			t.Fatalf("unexpected error %#v", err)
		}
	}
	if _, err := NewRegistry(config.ProviderRuntimeConfig{Providers: map[string]config.ProviderConfig{"broken": {BaseURL: "https://example.com", APIKey: "key", Model: "m"}}}); err == nil {
		t.Fatal("expected invalid provider type error")
	}
}
