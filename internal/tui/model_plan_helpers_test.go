package tui

import (
	"strings"
	"testing"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
)

func TestResolveSessionIDScenarios(t *testing.T) {
	summaries := []session.Summary{
		{ID: "abc12345"},
		{ID: "abc99999"},
		{ID: "def67890"},
	}

	got, err := resolveSessionID(summaries, "def67890")
	if err != nil || got != "def67890" {
		t.Fatalf("expected exact match, got id=%q err=%v", got, err)
	}

	got, err = resolveSessionID(summaries, "def")
	if err != nil || got != "def67890" {
		t.Fatalf("expected unique prefix match, got id=%q err=%v", got, err)
	}

	_, err = resolveSessionID(summaries, "zzz")
	if err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("expected not found error, got %v", err)
	}

	_, err = resolveSessionID(summaries, "abc")
	if err == nil || !strings.Contains(err.Error(), "matched multiple") {
		t.Fatalf("expected ambiguous prefix error, got %v", err)
	}
}

func TestParsePlanStepsTrimsAndSkipsEmpty(t *testing.T) {
	got := parsePlanSteps("  step one | | step two|step three  ")
	if len(got) != 3 {
		t.Fatalf("expected 3 non-empty steps, got %d (%v)", len(got), got)
	}
	if got[0] != "step one" || got[1] != "step two" || got[2] != "step three" {
		t.Fatalf("unexpected parsed steps: %v", got)
	}
}

func TestCanContinuePlan(t *testing.T) {
	if canContinuePlan(planpkg.State{}) {
		t.Fatalf("expected empty plan to be non-continuable")
	}

	ready := planpkg.State{
		Phase: planpkg.PhaseReady,
		Steps: []planpkg.Step{{Title: "run tests", Status: planpkg.StepPending}},
	}
	if !canContinuePlan(ready) {
		t.Fatalf("expected ready plan with steps to be continuable")
	}

	blocked := ready
	blocked.Phase = planpkg.PhaseBlocked
	if canContinuePlan(blocked) {
		t.Fatalf("expected blocked plan to be non-continuable")
	}

	completed := ready
	completed.Phase = planpkg.PhaseCompleted
	if canContinuePlan(completed) {
		t.Fatalf("expected completed plan to be non-continuable")
	}
}

func TestPreparePlanForContinuationPromotesPendingStep(t *testing.T) {
	state := planpkg.State{
		Phase: planpkg.PhaseReady,
		Steps: []planpkg.Step{
			{ID: "s1", Title: "inspect", Status: planpkg.StepPending},
			{ID: "s2", Title: "patch", Status: planpkg.StepPending},
		},
	}

	next, err := preparePlanForContinuation(state)
	if err != nil {
		t.Fatalf("expected continuation to succeed, got %v", err)
	}
	if next.Phase != planpkg.PhaseExecuting {
		t.Fatalf("expected phase executing, got %q", next.Phase)
	}
	if next.Steps[0].Status != planpkg.StepInProgress {
		t.Fatalf("expected first pending step promoted to in_progress, got %q", next.Steps[0].Status)
	}
	if strings.TrimSpace(next.NextAction) == "" {
		t.Fatalf("expected next_action to be set")
	}
}

func TestSessionTextAndInitialScreen(t *testing.T) {
	if got := initialScreen(nil); got != screenLanding {
		t.Fatalf("expected nil session to start at landing, got %q", got)
	}

	sess := session.New("E:\\code\\bytemind")
	if got := initialScreen(sess); got != screenLanding {
		t.Fatalf("expected empty session to start at landing, got %q", got)
	}

	sess.Messages = append(sess.Messages, llm.NewUserTextMessage("hello"))
	if got := initialScreen(sess); got != screenChat {
		t.Fatalf("expected non-empty session to start at chat, got %q", got)
	}

	sess.UpdatedAt = time.Unix(1710000000, 0).UTC()
	m := model{sess: sess}
	text := m.sessionText()
	if !strings.Contains(text, "Session ID: "+sess.ID) {
		t.Fatalf("expected session id in session text, got %q", text)
	}
	if !strings.Contains(text, "Messages: 1") {
		t.Fatalf("expected message count in session text, got %q", text)
	}
}

func TestPlanPanelWidthAndStatusGlyphHelpers(t *testing.T) {
	m := model{width: 120}
	if got, want := m.planPanelWidth(), m.chatPanelInnerWidth(); got != want {
		t.Fatalf("expected plan panel width to match chat width when sidebar disabled, got=%d want=%d", got, want)
	}

	if !strings.Contains(statusGlyph(string(planpkg.StepCompleted)), "v") {
		t.Fatalf("expected completed glyph to contain v")
	}
	if !strings.Contains(statusGlyph(string(planpkg.StepInProgress)), ">") {
		t.Fatalf("expected in-progress glyph to contain >")
	}
	if !strings.Contains(statusGlyph("warn"), "!") {
		t.Fatalf("expected warn glyph to contain !")
	}
	if !strings.Contains(statusGlyph("error"), "x") {
		t.Fatalf("expected error glyph to contain x")
	}
	if !strings.Contains(statusGlyph("unknown"), "-") {
		t.Fatalf("expected default glyph to contain -")
	}
}

func TestEmptyDot(t *testing.T) {
	if got := emptyDot(" "); got != "." {
		t.Fatalf("expected empty dot fallback, got %q", got)
	}
	if got := emptyDot("internal/tui"); got != "internal/tui" {
		t.Fatalf("expected non-empty path passthrough, got %q", got)
	}
}
