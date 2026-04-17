package provider

import "context"

type HealthScheduler struct {
	health HealthChecker
	ids    func(context.Context) ([]ProviderID, error)
}

func NewHealthScheduler(health HealthChecker, ids func(context.Context) ([]ProviderID, error), _ HealthConfig) *HealthScheduler {
	return &HealthScheduler{health: health, ids: ids}
}

func (s *HealthScheduler) Tick(ctx context.Context) error {
	if s == nil || s.health == nil || s.ids == nil {
		return nil
	}
	ids, err := s.ids(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.health.Check(ctx, id); err != nil && ctx.Err() != nil {
			return err
		}
	}
	return nil
}
