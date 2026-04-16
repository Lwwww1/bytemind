package provider

import (
	"context"
	"sort"
	"strings"
	"sync"

	"bytemind/internal/config"
)

type providerRegistry struct {
	mu      sync.RWMutex
	clients map[ProviderID]Client
}

func NewRegistry(cfg config.ProviderRuntimeConfig) (Registry, error) {
	reg := &providerRegistry{clients: make(map[ProviderID]Client)}
	ids := make([]string, 0, len(cfg.Providers))
	for id := range cfg.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		providerCfg := cfg.Providers[id]
		if strings.TrimSpace(providerCfg.Type) == "" {
			providerCfg.Type = id
		}
		client, err := NewDomainClient(providerCfg)
		if err != nil {
			return nil, err
		}
		if err := reg.Register(context.Background(), client); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func NewRegistryFromProviderConfig(cfg config.ProviderConfig) (Registry, error) {
	return NewRegistry(config.LegacyProviderRuntimeConfig(cfg))
}

func (r *providerRegistry) Register(_ context.Context, client Client) error {
	if client == nil {
		return nil
	}
	id := normalizeProviderID(client.ProviderID())
	if id == "" {
		return &Error{Code: ErrCodeProviderNotFound, Provider: id, Message: string(ErrCodeProviderNotFound), Retryable: false, Err: ErrProviderNotFound}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.clients[id]; exists {
		return &Error{Code: ErrCodeDuplicateProvider, Provider: id, Message: string(ErrCodeDuplicateProvider), Retryable: false, Err: ErrDuplicateProvider}
	}
	r.clients[id] = client
	return nil
}

func (r *providerRegistry) Get(_ context.Context, id ProviderID) (Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[normalizeProviderID(id)]
	return client, ok
}

func (r *providerRegistry) List(_ context.Context) ([]ProviderID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]ProviderID, 0, len(r.clients))
	for id := range r.clients {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func normalizeProviderID(id ProviderID) ProviderID {
	value := strings.ToLower(strings.TrimSpace(string(id)))
	switch value {
	case "openai-compatible", "openai":
		return ProviderOpenAI
	case "anthropic":
		return ProviderAnthropic
	default:
		return ProviderID(value)
	}
}
