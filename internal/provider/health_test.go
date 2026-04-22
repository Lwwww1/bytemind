package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthCheckerStateMachineTransitions(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 2, RecoverProbeSec: 10, RecoverSuccessThreshold: 2, WindowSize: 4}, nil).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeTimeout, Retryable: true})
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusDegraded || snapshot.FailureCount != 1 {
		t.Fatalf("unexpected degraded snapshot %#v", snapshot)
	}
	now = now.Add(time.Second)
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusUnavailable || snapshot.FailureCount != 2 || snapshot.NextProbeAt.IsZero() {
		t.Fatalf("unexpected unavailable snapshot %#v", snapshot)
	}
	if err := checker.Check(context.Background(), "openai"); err == nil {
		t.Fatal("expected unavailable before probe window")
	}
	now = snapshot.NextProbeAt
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected half-open probe to pass without checker, got %v", err)
	}
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHalfOpen {
		t.Fatalf("expected half_open after probe, got %#v", snapshot)
	}
	checker.RecordSuccess(context.Background(), "openai")
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHalfOpen || snapshot.SuccessCount != 1 {
		t.Fatalf("unexpected half-open recovery snapshot %#v", snapshot)
	}
	checker.RecordSuccess(context.Background(), "openai")
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy || snapshot.SuccessCount != 2 {
		t.Fatalf("unexpected healthy snapshot %#v", snapshot)
	}
}

func TestHealthCheckerIgnoresNonAvailabilityFailures(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1}, nil)
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnauthorized})
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy || snapshot.FailureCount != 0 {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
}

func TestHealthCheckerCheckUsesProbeAndRecordsResults(t *testing.T) {
	calls := 0
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 5, RecoverSuccessThreshold: 1}, func(_ context.Context, id ProviderID) error {
		calls++
		if id != "openai" {
			t.Fatalf("unexpected provider %q", id)
		}
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeTimeout, Retryable: true})
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusUnavailable {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
	if err := checker.Check(context.Background(), "openai"); err == nil {
		t.Fatal("expected probe window block")
	}
	now = snapshot.NextProbeAt
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected second probe success, got %v", err)
	}
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy {
		t.Fatalf("unexpected recovered snapshot %#v", snapshot)
	}
	if calls != 1 {
		t.Fatalf("expected one active probe call, got %d", calls)
	}
}

func TestExternalHealthTickerTick(t *testing.T) {
	var checked []ProviderID
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 1, RecoverSuccessThreshold: 1}, func(_ context.Context, id ProviderID) error {
		checked = append(checked, id)
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.RecordFailure(context.Background(), "anthropic", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.providers["openai"].nextProbeAt = now
	checker.providers["anthropic"].nextProbeAt = now
	ticker := NewExternalHealthTicker(checker, func(context.Context) ([]ProviderID, error) {
		return []ProviderID{"openai", "anthropic"}, nil
	})
	if err := ticker.Tick(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(checked) != 2 {
		t.Fatalf("unexpected checked ids %#v", checked)
	}
}

func TestExternalHealthTickerTickReturnsAggregatedErrors(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 1, RecoverSuccessThreshold: 1}, func(_ context.Context, id ProviderID) error {
		return &Error{Code: ErrCodeBadRequest, Provider: id, Message: "bad", Retryable: false}
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.RecordFailure(context.Background(), "anthropic", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.providers["openai"].nextProbeAt = now
	checker.providers["anthropic"].nextProbeAt = now
	ticker := NewExternalHealthTicker(checker, func(context.Context) ([]ProviderID, error) {
		return []ProviderID{"openai", "anthropic"}, nil
	})
	if err := ticker.Tick(context.Background()); err == nil {
		t.Fatal("expected aggregated error")
	}
}

func TestExternalHealthTickerTickIgnoresExpectedUnavailableWindow(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 10, RecoverSuccessThreshold: 1}, func(_ context.Context, _ ProviderID) error {
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	ticker := NewExternalHealthTicker(checker, func(context.Context) ([]ProviderID, error) {
		return []ProviderID{"openai"}, nil
	})
	if err := ticker.Tick(context.Background()); err != nil {
		t.Fatalf("expected recovery-window unavailable to be ignored, got %v", err)
	}
}

func TestHealthCheckerOnlyProbesUnavailableOrHalfOpen(t *testing.T) {
	calls := 0
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 5, RecoverSuccessThreshold: 1}, func(_ context.Context, _ ProviderID) error {
		calls++
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected healthy check to be local-only, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no active probe for healthy provider, got %d", calls)
	}
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusUnavailable {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
	now = snapshot.NextProbeAt
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected half-open probe success, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one active probe, got %d", calls)
	}
}

func TestHealthCheckerOnlyAllowsOneHalfOpenProbe(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 1, RecoverSuccessThreshold: 1}, func(_ context.Context, _ ProviderID) error {
		started <- struct{}{}
		<-release
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.providers["openai"].nextProbeAt = now
	errCh := make(chan error, 2)
	go func() { errCh <- checker.Check(context.Background(), "openai") }()
	<-started
	go func() { errCh <- checker.Check(context.Background(), "openai") }()
	secondErr := <-errCh
	if secondErr == nil {
		secondErr = <-errCh
	}
	if secondErr == nil {
		t.Fatal("expected concurrent half-open probe to be rejected")
	}
	close(release)
	firstErr := <-errCh
	if firstErr != nil {
		t.Fatalf("expected active probe to succeed, got %v", firstErr)
	}
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHealthy {
		t.Fatalf("expected provider to recover, got %#v", snapshot)
	}
}

func TestHealthCheckerCancelledProbeReleasesGate(t *testing.T) {
	calls := 0
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 1, RecoverSuccessThreshold: 1}, func(_ context.Context, _ ProviderID) error {
		calls++
		if calls == 1 {
			return context.Canceled
		}
		return nil
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.providers["openai"].nextProbeAt = now
	if err := checker.Check(context.Background(), "openai"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled probe, got %v", err)
	}
	if checker.providers["openai"].probeInFlight {
		t.Fatal("expected canceled probe to release gate")
	}
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected next probe to proceed, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected probe to retry after cancel, got %d calls", calls)
	}
}

func TestHealthCheckerProbeCompletionCommitsBeforeGateRelease(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	calls := 0
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 1, RecoverSuccessThreshold: 1}, func(_ context.Context, _ ProviderID) error {
		calls++
		started <- struct{}{}
		<-release
		return &Error{Code: ErrCodeUnavailable, Retryable: true}
	}).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.providers["openai"].nextProbeAt = now
	errCh := make(chan error, 2)
	go func() { errCh <- checker.Check(context.Background(), "openai") }()
	<-started
	close(release)
	firstErr := <-errCh
	if firstErr == nil {
		t.Fatal("expected first probe to fail")
	}
	secondErr := checker.Check(context.Background(), "openai")
	if secondErr == nil {
		t.Fatal("expected second check to observe unavailable state")
	}
	if calls != 1 {
		t.Fatalf("expected exactly one real probe call, got %d", calls)
	}
}

func TestHealthCheckerCheckWithoutProbeDoesNotAutoRecover(t *testing.T) {
	checker := NewHealthChecker(HealthConfig{FailThreshold: 1, RecoverProbeSec: 1, RecoverSuccessThreshold: 2}, nil).(*healthChecker)
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	checker.clock = func() time.Time { return now }
	checker.RecordFailure(context.Background(), "openai", &Error{Code: ErrCodeUnavailable, Retryable: true})
	checker.providers["openai"].nextProbeAt = now
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected gate-only half-open check, got %v", err)
	}
	snapshot := checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHalfOpen || snapshot.SuccessCount != 0 {
		t.Fatalf("expected no-result check to keep half_open without synthetic success, got %#v", snapshot)
	}
	if err := checker.Check(context.Background(), "openai"); err != nil {
		t.Fatalf("expected repeated gate-only check, got %v", err)
	}
	snapshot = checker.Status(context.Background(), "openai")
	if snapshot.Status != HealthStatusHalfOpen || snapshot.SuccessCount != 0 {
		t.Fatalf("expected no auto recovery without real probe, got %#v", snapshot)
	}
}

func TestCountsTowardAvailability(t *testing.T) {
	if countsTowardAvailability(nil) || countsTowardAvailability(context.Canceled) {
		t.Fatal("expected nil/canceled to be ignored")
	}
	if countsTowardAvailability(&Error{Code: ErrCodeBadRequest}) {
		t.Fatal("expected bad request to be ignored")
	}
	if !countsTowardAvailability(errors.New("boom")) || !countsTowardAvailability(&Error{Code: ErrCodeRateLimited, Retryable: true}) {
		t.Fatal("expected generic/retryable errors to count")
	}
}
