package provider

import (
	"context"
	"errors"
)

type ExternalHealthTicker struct {
	health HealthChecker
	ids    func(context.Context) ([]ProviderID, error)
}

func NewExternalHealthTicker(health HealthChecker, ids func(context.Context) ([]ProviderID, error)) *ExternalHealthTicker {
	return &ExternalHealthTicker{health: health, ids: ids}
}

func (t *ExternalHealthTicker) Tick(ctx context.Context) error {
	if t == nil || t.health == nil || t.ids == nil {
		return nil
	}
	ids, err := t.ids(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := t.health.Check(ctx, id); err != nil {
			if ctx.Err() != nil {
				return err
			}
			var providerErr *Error
			if errors.As(err, &providerErr) && providerErr.Code == ErrCodeUnavailable {
				continue
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
