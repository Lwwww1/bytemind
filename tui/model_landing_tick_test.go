package tui

import "testing"

func TestInitReturnsBatchCommand(t *testing.T) {
	m := model{}
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("expected Init to return a non-nil command")
	}
}

func TestLandingGlowTickCmdEmitsLandingGlowTickMsg(t *testing.T) {
	cmd := landingGlowTickCmd()
	if cmd == nil {
		t.Fatalf("expected landing glow tick command to be non-nil")
	}
	if _, ok := cmd().(landingGlowTickMsg); !ok {
		t.Fatalf("expected landingGlowTickCmd to emit landingGlowTickMsg")
	}
}

func TestUpdateLandingGlowTickWrapsAndSchedulesNextTick(t *testing.T) {
	m := model{landingGlowStep: 2047}

	updatedAny, cmd := m.Update(landingGlowTickMsg{})
	updated, ok := updatedAny.(model)
	if !ok {
		t.Fatalf("expected updated model type, got %T", updatedAny)
	}
	if updated.landingGlowStep != 0 {
		t.Fatalf("expected landingGlowStep to wrap to 0, got %d", updated.landingGlowStep)
	}
	if cmd == nil {
		t.Fatalf("expected Update to schedule next landing glow tick command")
	}
	if _, ok := cmd().(landingGlowTickMsg); !ok {
		t.Fatalf("expected scheduled command to emit landingGlowTickMsg")
	}
}
