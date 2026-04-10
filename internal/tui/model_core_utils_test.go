package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestCommandItemFilterValue(t *testing.T) {
	item := commandItem{
		Usage:       "/Skill Run",
		Description: "Activate current mode",
	}
	got := item.FilterValue()
	if got != "skill run activate current mode" {
		t.Fatalf("unexpected filter value: %q", got)
	}
}

func TestNormalizeKeyName(t *testing.T) {
	if got := normalizeKeyName(" Shift_Enter "); got != "shiftenter" {
		t.Fatalf("expected normalized key name shiftenter, got %q", got)
	}
	if got := normalizeKeyName("CTRL+J"); got != "ctrl+j" {
		t.Fatalf("expected plus sign to be preserved, got %q", got)
	}
}

func TestLenCommonPrefix(t *testing.T) {
	if got := lenCommonPrefix("abcdef", "abcXYZ"); got != 3 {
		t.Fatalf("expected common prefix len 3, got %d", got)
	}
	if got := lenCommonPrefix("same", "same"); got != 4 {
		t.Fatalf("expected full prefix len 4, got %d", got)
	}
}

func TestBeginRunSetsBusyState(t *testing.T) {
	m := model{
		async:   make(chan tea.Msg, 4),
		spinner: spinner.New(),
	}
	cmd := m.beginRun("ship it", "build", "")
	if cmd == nil {
		t.Fatalf("expected beginRun to return command")
	}
	if !m.busy || m.phase != "thinking" || !m.llmConnected {
		t.Fatalf("expected busy thinking connected state, got busy=%v phase=%q connected=%v", m.busy, m.phase, m.llmConnected)
	}
	if m.runSeq != 1 || m.activeRunID != 1 {
		t.Fatalf("expected run sequence initialized, got runSeq=%d activeRunID=%d", m.runSeq, m.activeRunID)
	}
	if m.runCancel == nil {
		t.Fatalf("expected run cancel func to be set")
	}
	if m.statusNote != "Request sent to LLM. Waiting for response..." {
		t.Fatalf("unexpected status note: %q", m.statusNote)
	}
	m.runCancel()
}

func TestAppendChatAndSelectionHelpers(t *testing.T) {
	m := model{}
	m.appendChat(chatEntry{Kind: "user", Body: "a"})
	m.appendChat(chatEntry{Kind: "assistant", Body: "b"})
	if len(m.chatItems) != 2 {
		t.Fatalf("expected two chat items, got %d", len(m.chatItems))
	}
	if m.chatItems[0].Body != "a" || m.chatItems[1].Body != "b" {
		t.Fatalf("unexpected chat order: %#v", m.chatItems)
	}

	if selectionHasRange(viewportSelectionPoint{Row: 1, Col: 2}, viewportSelectionPoint{Row: 1, Col: 2}) {
		t.Fatalf("expected same points to have no range")
	}
	if !selectionHasRange(viewportSelectionPoint{Row: 1, Col: 2}, viewportSelectionPoint{Row: 1, Col: 3}) {
		t.Fatalf("expected different points to have range")
	}
}

func TestSpacerWrapper(t *testing.T) {
	if got := spacer(0); got != "" {
		t.Fatalf("expected zero-width spacer to be empty")
	}
	if got := spacer(3); lipgloss.Width(got) != 3 {
		t.Fatalf("expected spacer width 3, got %d", lipgloss.Width(got))
	}
}

func TestEnsureZoneManagerIdempotent(t *testing.T) {
	ensureZoneManager()
	ensureZoneManager()
}
