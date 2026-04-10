package tui

import (
	"strings"
	"testing"
	"time"

	"bytemind/internal/session"
)

func TestSetUsageMarksOfficialAndClearsUnavailable(t *testing.T) {
	m := model{
		tokenUsage: newTokenUsageComponent(),
	}
	m.tokenUsage.SetUnavailable(true)
	m.tokenHasOfficialUsage = false

	_ = m.SetUsage(321, 999)

	if !m.tokenHasOfficialUsage {
		t.Fatalf("expected model usage to be marked official")
	}
	if m.tokenUsage.unavailable {
		t.Fatalf("expected token usage to be available after SetUsage")
	}
	if m.tokenUsage.used != 321 {
		t.Fatalf("expected used tokens to update, got %d", m.tokenUsage.used)
	}
}

func TestStripANSITextRemovesEscapeCodes(t *testing.T) {
	raw := "\u001b[31merror\u001b[0m details"
	got := stripANSIText(raw)
	if got != "error details" {
		t.Fatalf("expected ansi stripped text, got %q", got)
	}
}

func TestRenderSessionsModalAndHelpModal(t *testing.T) {
	m := model{
		width:         100,
		sessionCursor: 1,
		sessions: []session.Summary{
			{
				ID:           "111111111111aaaa",
				Workspace:    "E:\\code\\repo-a",
				UpdatedAt:    time.Unix(1710000000, 0).UTC(),
				MessageCount: 3,
			},
			{
				ID:              "222222222222bbbb",
				Workspace:       "E:\\code\\repo-b",
				UpdatedAt:       time.Unix(1710000300, 0).UTC(),
				MessageCount:    9,
				LastUserMessage: "please split tests by responsibility",
			},
		},
	}

	sessionsModal := m.renderSessionsModal()
	if !strings.Contains(sessionsModal, "Recent Sessions") {
		t.Fatalf("expected sessions modal title, got %q", sessionsModal)
	}
	if !strings.Contains(sessionsModal, shortID("222222222222bbbb")) {
		t.Fatalf("expected selected session id to be rendered, got %q", sessionsModal)
	}
	if !strings.Contains(sessionsModal, "repo-b") {
		t.Fatalf("expected workspace label in modal, got %q", sessionsModal)
	}
	if !strings.Contains(sessionsModal, "please split tests") {
		t.Fatalf("expected last user message preview in modal, got %q", sessionsModal)
	}

	helpModal := m.renderHelpModal()
	if !strings.Contains(helpModal, "Help") {
		t.Fatalf("expected help modal title, got %q", helpModal)
	}
	if !strings.Contains(helpModal, "/quit") {
		t.Fatalf("expected help modal to include slash command guidance, got %q", helpModal)
	}
}
