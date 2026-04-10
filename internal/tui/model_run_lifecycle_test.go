package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalBannerRendersAboveInput(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
		approval: &approvalPrompt{
			Command: "go test ./internal/tui",
			Reason:  "run tests",
		},
	}

	footer := m.renderFooter()
	for _, want := range []string{
		"go test ./internal/tui",
		"run tests",
		"Y / Enter",
		"N / Esc",
	} {
		if !strings.Contains(footer, want) {
			t.Fatalf("expected approval banner to contain %q", want)
		}
	}
	if strings.Contains(footer, "Approval Request") {
		t.Fatalf("did not expect old centered approval modal title in footer")
	}
}

func TestRenderFooterShowsActiveSkillBanner(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
		sess: &session.Session{
			ActiveSkill: &session.ActiveSkill{
				Name: "review",
				Args: map[string]string{"severity": "high"},
			},
		},
	}

	footer := m.renderFooter()
	if !strings.Contains(footer, "Active skill: review") {
		t.Fatalf("expected footer to show active skill banner, got %q", footer)
	}
	if !strings.Contains(footer, "severity=high") {
		t.Fatalf("expected footer to show active skill args, got %q", footer)
	}
}

func TestUpdateApprovalRequestMsgSetsApprovalPhase(t *testing.T) {
	reply := make(chan approvalDecision, 1)
	m := model{async: make(chan tea.Msg, 1)}

	got, cmd := m.Update(approvalRequestMsg{
		Request: tools.ApprovalRequest{
			Command: "go test ./internal/tui",
			Reason:  "run focused tests",
		},
		Reply: reply,
	})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected approval request to keep waiting for async events")
	}
	if updated.approval == nil {
		t.Fatalf("expected approval prompt to be stored on the model")
	}
	if updated.approval.Command != "go test ./internal/tui" || updated.approval.Reason != "run focused tests" {
		t.Fatalf("expected approval prompt contents to be preserved, got %+v", updated.approval)
	}
	if updated.phase != "approval" || updated.statusNote != "Approval required." {
		t.Fatalf("expected approval request to switch UI into approval state, got phase=%q note=%q", updated.phase, updated.statusNote)
	}
}

func TestApprovalKeysTransitionStateAndSendDecision(t *testing.T) {
	t.Run("approve", func(t *testing.T) {
		reply := make(chan approvalDecision, 1)
		m := model{
			approval: &approvalPrompt{
				Command: "go test ./internal/tui",
				Reason:  "run focused tests",
				Reply:   reply,
			},
			phase: "approval",
		}

		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		updated := got.(model)

		if updated.approval != nil {
			t.Fatalf("expected approval prompt to clear after approval")
		}
		if updated.phase != "tool" || updated.statusNote != "Shell command approved." {
			t.Fatalf("expected approval to move UI into tool phase, got phase=%q note=%q", updated.phase, updated.statusNote)
		}

		select {
		case decision := <-reply:
			if !decision.Approved {
				t.Fatalf("expected approval decision to be true")
			}
		default:
			t.Fatalf("expected approval decision to be sent")
		}
	})

	t.Run("reject", func(t *testing.T) {
		reply := make(chan approvalDecision, 1)
		m := model{
			approval: &approvalPrompt{
				Command: "go test ./internal/tui",
				Reason:  "run focused tests",
				Reply:   reply,
			},
			phase: "approval",
		}

		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
		updated := got.(model)

		if updated.approval != nil {
			t.Fatalf("expected approval prompt to clear after rejection")
		}
		if updated.phase != "thinking" || updated.statusNote != "Shell command rejected." {
			t.Fatalf("expected rejection to return UI to thinking phase, got phase=%q note=%q", updated.phase, updated.statusNote)
		}

		select {
		case decision := <-reply:
			if decision.Approved {
				t.Fatalf("expected rejection decision to be false")
			}
		default:
			t.Fatalf("expected rejection decision to be sent")
		}
	})
}

func TestUpdateRunFinishedMsgResetsBusyState(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := model{
			async:          make(chan tea.Msg, 1),
			busy:           true,
			streamingIndex: 3,
			statusNote:     "Running...",
			phase:          "responding",
			llmConnected:   true,
		}

		got, cmd := m.Update(runFinishedMsg{})
		updated := got.(model)

		if cmd == nil {
			t.Fatalf("expected run finished to schedule follow-up async/session work")
		}
		if updated.busy {
			t.Fatalf("expected run finished to clear busy state")
		}
		if updated.streamingIndex != -1 {
			t.Fatalf("expected run finished to clear streaming index, got %d", updated.streamingIndex)
		}
		if updated.phase != "idle" || updated.statusNote != "Ready." {
			t.Fatalf("expected successful run to return to idle, got phase=%q note=%q", updated.phase, updated.statusNote)
		}
		if !updated.llmConnected {
			t.Fatalf("expected successful run to keep llmConnected=true")
		}
	})

	t.Run("error", func(t *testing.T) {
		m := model{
			async:          make(chan tea.Msg, 1),
			busy:           true,
			streamingIndex: 1,
			statusNote:     "Running...",
			phase:          "responding",
			llmConnected:   true,
			chatItems: []chatEntry{
				{Kind: "user", Title: "You", Body: "inspect repo", Status: "final"},
				{Kind: "assistant", Title: thinkingLabel, Body: "thinking", Status: "thinking"},
			},
		}

		got, _ := m.Update(runFinishedMsg{Err: errors.New("provider unavailable")})
		updated := got.(model)

		if updated.busy {
			t.Fatalf("expected failed run to clear busy state")
		}
		if updated.phase != "error" || !strings.Contains(updated.statusNote, "provider unavailable") {
			t.Fatalf("expected failed run to switch UI into error state, got phase=%q note=%q", updated.phase, updated.statusNote)
		}
		if updated.llmConnected {
			t.Fatalf("expected failed run to mark llmConnected=false")
		}
		last := updated.chatItems[len(updated.chatItems)-1]
		if last.Status != "error" || !strings.Contains(last.Body, "provider unavailable") {
			t.Fatalf("expected latest assistant card to show failure, got %+v", last)
		}
	})
}

func TestRunFinishedKeepsStreamingSlotForLateAssistantMessage(t *testing.T) {
	m := model{
		async: make(chan tea.Msg, 1),
		busy:  true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "test", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "received,", Status: "streaming"},
		},
		streamingIndex: 1,
	}

	got, _ := m.Update(runFinishedMsg{})
	updated := got.(model)
	if updated.streamingIndex != 1 {
		t.Fatalf("expected run finished to keep streaming index for late final message, got %d", updated.streamingIndex)
	}

	updated.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantMessage,
		Content: "received, response looks good.",
	})

	if len(updated.chatItems) != 2 {
		t.Fatalf("expected late final message to update existing assistant card, got %d items", len(updated.chatItems))
	}
	last := updated.chatItems[1]
	if last.Status != "final" || strings.TrimSpace(last.Body) != "received, response looks good." {
		t.Fatalf("expected assistant card to be finalized in place, got %+v", last)
	}
}
func TestBusyInputStillEditable(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen:    screenChat,
		busy:      true,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated := got.(model)
	if updated.input.Value() != "a" {
		t.Fatalf("expected busy input to stay editable, got %q", updated.input.Value())
	}
}

func TestBusyEnterQueuesBTWAndCancelsRun(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("focus only on unit tests")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:    screenChat,
		busy:      true,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		runCancel: func() { canceled = true },
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if !canceled {
		t.Fatalf("expected busy enter to cancel the active run")
	}
	if !updated.interrupting {
		t.Fatalf("expected model to enter interrupting state")
	}
	if len(updated.pendingBTW) != 1 || updated.pendingBTW[0] != "focus only on unit tests" {
		t.Fatalf("expected pending btw queue to capture input, got %#v", updated.pendingBTW)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected btw submit to reset input, got %q", updated.input.Value())
	}
	if len(updated.chatItems) != 1 || updated.chatItems[0].Body != "focus only on unit tests" {
		t.Fatalf("expected btw submit to append a user chat entry, got %#v", updated.chatItems)
	}
	if !strings.Contains(updated.chatItems[0].Meta, "btw") {
		t.Fatalf("expected btw marker in chat meta, got %q", updated.chatItems[0].Meta)
	}
}

func TestBusyEnterSuppressedAfterRecentMultilinePaste(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("Design a plugin platform\n- dynamic plugin loading\n- permission isolation")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:      screenChat,
		busy:        true,
		input:       input,
		lastPasteAt: time.Now(),
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
		runCancel:   func() { canceled = true },
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd != nil {
		t.Fatalf("expected suppressed enter not to schedule a command")
	}
	if canceled {
		t.Fatalf("expected suppressed enter not to cancel current run")
	}
	if updated.interrupting || len(updated.pendingBTW) != 0 || len(updated.chatItems) != 0 {
		t.Fatalf("expected no BTW side effects, got interrupting=%v pending=%#v chat=%#v", updated.interrupting, updated.pendingBTW, updated.chatItems)
	}
}

func TestBusyEnterSuppressedForRecentPasteBurstSingleLine(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("dynamic plugin loading")
	input.CursorEnd()

	canceled := false
	now := time.Now()
	m := model{
		screen:      screenChat,
		busy:        true,
		input:       input,
		lastPasteAt: now,
		lastInputAt: now,
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
		runCancel:   func() { canceled = true },
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd != nil {
		t.Fatalf("expected suppressed burst enter not to schedule a command")
	}
	if canceled {
		t.Fatalf("expected suppressed burst enter not to cancel current run")
	}
	if updated.interrupting || len(updated.pendingBTW) != 0 || len(updated.chatItems) != 0 {
		t.Fatalf("expected no BTW side effects, got interrupting=%v pending=%#v chat=%#v", updated.interrupting, updated.pendingBTW, updated.chatItems)
	}
}

func TestBusyEnterInToolPhaseDefersBTWCancel(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("change plan after this step")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:    screenChat,
		busy:      true,
		phase:     "tool",
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		runCancel: func() { canceled = true },
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if canceled {
		t.Fatalf("expected tool phase btw to defer cancel until tool step completes")
	}
	if !updated.interrupting || !updated.interruptSafe {
		t.Fatalf("expected deferred interrupt flags, got interrupting=%v interruptSafe=%v", updated.interrupting, updated.interruptSafe)
	}
	if updated.statusNote != "BTW queued. Waiting for current tool step to finish..." {
		t.Fatalf("expected deferred tool note, got %q", updated.statusNote)
	}
}

func TestRenderChatCardToolUsesVisualSeparator(t *testing.T) {
	got := renderChatCard(chatEntry{
		Kind:   "tool",
		Title:  "Tool Call | read_file",
		Body:   "Read internal/tui/model.go lines 1-20",
		Status: "done",
	}, 64)

	if !strings.Contains(got, "\u2502") && !strings.Contains(got, "|") {
		t.Fatalf("expected tool card to include a left border separator, got %q", got)
	}
	if !strings.Contains(got, "Tool Call | read_file") {
		t.Fatalf("expected tool card title to render, got %q", got)
	}
}

func TestSubmitBTWWithoutRunCancelRestartsImmediately(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		async:     make(chan tea.Msg, 1),
		busy:      true,
		mode:      modeBuild,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}

	got, cmd := m.submitBTW("continue with deletion")
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected fallback btw path to start a new run command")
	}
	if !updated.busy {
		t.Fatalf("expected model to become busy after immediate btw restart")
	}
	if updated.interrupting {
		t.Fatalf("expected interrupting state to clear after immediate restart")
	}
	if updated.interruptSafe {
		t.Fatalf("expected interruptSafe to be false after immediate restart")
	}
	if len(updated.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue to be consumed, got %#v", updated.pendingBTW)
	}
	if updated.runCancel == nil {
		t.Fatalf("expected immediate restart to set runCancel")
	}
	if updated.statusNote != "BTW accepted. Restarting with your update..." {
		t.Fatalf("expected immediate restart status note, got %q", updated.statusNote)
	}
}

func TestToolCallCompletedTriggersDeferredBTWCancel(t *testing.T) {
	canceled := false
	m := model{
		interrupting:  true,
		interruptSafe: true,
		pendingBTW:    []string{"change plan"},
		runCancel:     func() { canceled = true },
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":3}`,
	})

	if !canceled {
		t.Fatalf("expected deferred btw cancel to trigger after tool completion")
	}
	if m.interruptSafe {
		t.Fatalf("expected deferred interrupt flag to clear after cancel")
	}
	if m.phase != "interrupting" {
		t.Fatalf("expected phase to switch to interrupting, got %q", m.phase)
	}
}

func TestRunFinishedWithPendingBTWRestartsRun(t *testing.T) {
	m := model{
		async:        make(chan tea.Msg, 1),
		busy:         true,
		activeRunID:  2,
		runSeq:       2,
		interrupting: true,
		pendingBTW:   []string{"first update", "second update"},
		mode:         modeBuild,
		sess:         session.New("E:\\bytemind"),
		workspace:    "E:\\bytemind",
	}

	got, cmd := m.Update(runFinishedMsg{RunID: 2, Err: context.Canceled})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected interrupted run to schedule a follow-up run")
	}
	if !updated.busy {
		t.Fatalf("expected model to restart immediately with pending btw prompt")
	}
	if len(updated.chatItems) != 1 || updated.chatItems[0].Kind != "system" {
		t.Fatalf("expected system restart notice before resumed run, got %#v", updated.chatItems)
	}
	if !strings.Contains(updated.chatItems[0].Body, "BTW interrupt accepted") {
		t.Fatalf("expected btw restart notice, got %#v", updated.chatItems[0])
	}
	if !strings.Contains(updated.chatItems[0].Body, "2 updates") {
		t.Fatalf("expected restart notice to include update count, got %#v", updated.chatItems[0])
	}
	if updated.interrupting {
		t.Fatalf("expected interrupting state to clear after restart")
	}
	if len(updated.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue to be consumed, got %#v", updated.pendingBTW)
	}
	if updated.runCancel == nil {
		t.Fatalf("expected restart to register a new cancel function")
	}
	if updated.phase != "thinking" {
		t.Fatalf("expected restart phase to return to thinking, got %q", updated.phase)
	}
	if !strings.Contains(updated.statusNote, "Restarting with 2 updates") {
		t.Fatalf("expected restart status note, got %q", updated.statusNote)
	}
	if updated.activeRunID == 0 {
		t.Fatalf("expected resumed run to have a new active run id")
	}
}

func TestClassifyRunFinish(t *testing.T) {
	if got := classifyRunFinish(nil, false); got != runFinishReasonCompleted {
		t.Fatalf("expected completed, got %q", got)
	}
	if got := classifyRunFinish(context.Canceled, false); got != runFinishReasonCanceled {
		t.Fatalf("expected canceled, got %q", got)
	}
	if got := classifyRunFinish(errors.New("boom"), false); got != runFinishReasonFailed {
		t.Fatalf("expected failed, got %q", got)
	}
	if got := classifyRunFinish(nil, true); got != runFinishReasonBTWRestart {
		t.Fatalf("expected btw restart, got %q", got)
	}
}

func TestQueueBTWUpdateKeepsMostRecentEntries(t *testing.T) {
	queue, dropped := queueBTWUpdate([]string{"1", "2", "3", "4", "5"}, "6")
	if dropped != 1 {
		t.Fatalf("expected one dropped entry, got %d", dropped)
	}
	if len(queue) != maxPendingBTW {
		t.Fatalf("expected capped queue length %d, got %d", maxPendingBTW, len(queue))
	}
	want := []string{"2", "3", "4", "5", "6"}
	for i := range want {
		if queue[i] != want[i] {
			t.Fatalf("expected queue[%d]=%q, got %q", i, want[i], queue[i])
		}
	}
}

func TestFormatBTWUpdateScope(t *testing.T) {
	if got := formatBTWUpdateScope(0); got != "your latest update" {
		t.Fatalf("expected default scope text, got %q", got)
	}
	if got := formatBTWUpdateScope(1); got != "your latest update" {
		t.Fatalf("expected single-entry scope text, got %q", got)
	}
	if got := formatBTWUpdateScope(3); got != "3 updates" {
		t.Fatalf("expected multi-entry scope text, got %q", got)
	}
}

func TestComposeBTWPromptSingleEntryKeepsContinuationContext(t *testing.T) {
	got := composeBTWPrompt([]string{"delete calculator.py"})
	if !strings.Contains(got, "Continue the same task") {
		t.Fatalf("expected single btw prompt to preserve continuation context, got %q", got)
	}
	if !strings.Contains(got, "delete calculator.py") {
		t.Fatalf("expected single btw prompt to include update content, got %q", got)
	}
}

func TestComposeBTWPromptIgnoresEmptyEntries(t *testing.T) {
	got := composeBTWPrompt([]string{"", "   ", "\n\t"})
	if got != "" {
		t.Fatalf("expected empty btw prompt when all entries are blank, got %q", got)
	}
}

func TestSubmitBTWShowsDropHintWhenQueueCapped(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("new update")
	input.CursorEnd()

	m := model{
		screen:       screenChat,
		busy:         true,
		interrupting: true,
		input:        input,
		pendingBTW:   []string{"1", "2", "3", "4", "5"},
		sess:         session.New("E:\\bytemind"),
		workspace:    "E:\\bytemind",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.pendingBTW) != maxPendingBTW {
		t.Fatalf("expected capped pending queue length %d, got %d", maxPendingBTW, len(updated.pendingBTW))
	}
	if updated.pendingBTW[0] != "2" || updated.pendingBTW[len(updated.pendingBTW)-1] != "new update" {
		t.Fatalf("expected oldest entry to be dropped, got %#v", updated.pendingBTW)
	}
	if !strings.Contains(updated.statusNote, "dropped 1 older") {
		t.Fatalf("expected drop hint in status note, got %q", updated.statusNote)
	}
}

func TestNewSessionClearsInterruptState(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.SetValue("pending input")
	m := model{
		store:         store,
		sess:          current,
		workspace:     workspace,
		input:         input,
		pendingBTW:    []string{"keep this"},
		interrupting:  true,
		interruptSafe: true,
		runCancel:     func() {},
		activeRunID:   9,
	}

	if err := m.newSession(); err != nil {
		t.Fatalf("expected newSession to succeed, got %v", err)
	}
	if m.interrupting || m.interruptSafe {
		t.Fatalf("expected interrupt flags to clear, got interrupting=%v interruptSafe=%v", m.interrupting, m.interruptSafe)
	}
	if len(m.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue cleared, got %#v", m.pendingBTW)
	}
	if m.runCancel != nil {
		t.Fatalf("expected runCancel cleared on new session")
	}
	if m.activeRunID != 0 {
		t.Fatalf("expected activeRunID reset, got %d", m.activeRunID)
	}
	if m.screen != screenLanding {
		t.Fatalf("expected new session to switch to landing screen, got %q", m.screen)
	}
}

func TestResumeSessionClearsInterruptState(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	target := session.New(workspace)
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          current,
		workspace:     workspace,
		sessions:      []session.Summary{{ID: target.ID}},
		pendingBTW:    []string{"queued"},
		interrupting:  true,
		interruptSafe: true,
		runCancel:     func() {},
		activeRunID:   7,
	}

	if err := m.resumeSession(target.ID); err != nil {
		t.Fatalf("expected resumeSession to succeed, got %v", err)
	}
	if m.sess == nil || m.sess.ID != target.ID {
		t.Fatalf("expected target session to become active, got %#v", m.sess)
	}
	if m.interrupting || m.interruptSafe {
		t.Fatalf("expected interrupt flags to clear, got interrupting=%v interruptSafe=%v", m.interrupting, m.interruptSafe)
	}
	if len(m.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue cleared, got %#v", m.pendingBTW)
	}
	if m.runCancel != nil {
		t.Fatalf("expected runCancel cleared on resume")
	}
	if m.activeRunID != 0 {
		t.Fatalf("expected activeRunID reset, got %d", m.activeRunID)
	}
	if m.screen != screenChat {
		t.Fatalf("expected resume to switch to chat screen, got %q", m.screen)
	}
}

func TestUpdateIgnoresStaleRunFinishedMsg(t *testing.T) {
	m := model{
		async:       make(chan tea.Msg, 1),
		busy:        true,
		activeRunID: 5,
		statusNote:  "Running...",
		phase:       "responding",
	}

	got, cmd := m.Update(runFinishedMsg{RunID: 4})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected stale run message handling to keep waiting for async events")
	}
	if !updated.busy {
		t.Fatalf("expected stale run message not to stop the active run")
	}
	if updated.activeRunID != 5 {
		t.Fatalf("expected active run id to remain unchanged, got %d", updated.activeRunID)
	}
	if updated.statusNote != "Running..." {
		t.Fatalf("expected stale run message not to rewrite status, got %q", updated.statusNote)
	}
}

func TestBTWCommandInIdleSubmitsPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/btw tighten the test scope")
	input.CursorEnd()
	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) == 0 || updated.chatItems[0].Body != "tighten the test scope" {
		t.Fatalf("expected /btw in idle mode to submit its message, got %#v", updated.chatItems)
	}
	if !updated.busy {
		t.Fatalf("expected /btw in idle mode to start a run")
	}
}

func TestBTWCommandRequiresMessage(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/btw")
	input.CursorEnd()
	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.statusNote != "usage: /btw <message>" {
		t.Fatalf("expected usage hint for empty /btw, got %q", updated.statusNote)
	}
	if updated.busy {
		t.Fatalf("expected empty /btw not to start a run")

	}
}

func TestUpdateSessionsLoadedMsgUpdatesAndClampsSessions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := model{
			sessionCursor: 3,
			sessions: []session.Summary{
				{ID: "old-1"},
				{ID: "old-2"},
				{ID: "old-3"},
				{ID: "old-4"},
			},
		}

		got, _ := m.Update(sessionsLoadedMsg{
			Summaries: []session.Summary{
				{ID: "new-1"},
				{ID: "new-2"},
			},
		})
		updated := got.(model)

		if len(updated.sessions) != 2 {
			t.Fatalf("expected sessions list to be replaced, got %d entries", len(updated.sessions))
		}
		if updated.sessionCursor != 1 {
			t.Fatalf("expected session cursor to clamp to last available entry, got %d", updated.sessionCursor)
		}
	})

	t.Run("error", func(t *testing.T) {
		m := model{
			sessionCursor: 1,
			sessions: []session.Summary{
				{ID: "keep-1"},
				{ID: "keep-2"},
			},
		}

		got, _ := m.Update(sessionsLoadedMsg{
			Err: errors.New("store unavailable"),
		})
		updated := got.(model)

		if len(updated.sessions) != 2 || updated.sessions[0].ID != "keep-1" || updated.sessions[1].ID != "keep-2" {
			t.Fatalf("expected session list to stay unchanged on load error, got %+v", updated.sessions)
		}
		if updated.sessionCursor != 1 {
			t.Fatalf("expected session cursor to remain unchanged on load error, got %d", updated.sessionCursor)
		}
	})
}
