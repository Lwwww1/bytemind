package provider

import "context"

func ListModels(ctx context.Context, reg Registry) ([]ModelInfo, []Warning, error) {
	ids, err := reg.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	models := make([]ModelInfo, 0)
	warnings := make([]Warning, 0)
	seen := make(map[string]struct{})
	for _, id := range ids {
		client, ok := reg.Get(ctx, id)
		if !ok {
			warnings = append(warnings, Warning{ProviderID: id, Reason: string(ErrCodeProviderNotFound)})
			continue
		}
		providerModels, err := client.ListModels(ctx)
		if err != nil {
			warnings = append(warnings, Warning{ProviderID: id, Reason: err.Error()})
			continue
		}
		for _, model := range providerModels {
			providerID := normalizeProviderID(model.ProviderID)
			if providerID == "" {
				providerID = id
			}
			key := string(providerID) + "\x00" + string(model.ModelID)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			model.ProviderID = providerID
			models = append(models, model)
		}
	}
	return models, warnings, nil
}
