package tui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderMainPanelShowsTokenUsageBadge(t *testing.T) {
	m := model{
		screen:     screenChat,
		width:      120,
		height:     28,
		input:      textarea.New(),
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Repeat("line\n", 40))
	m.tokenUsage.displayUsed = 1234
	_ = m.tokenUsage.SetUsage(1234, 5000)

	panel := m.renderMainPanel()
	if !strings.Contains(panel, "token: 1,234") {
		t.Fatalf("expected token usage badge text in main panel, got %q", panel)
	}
}

func TestHandleMouseHoverTokenUsageConsumesEvent(t *testing.T) {
	m := model{
		screen:     screenChat,
		width:      120,
		height:     28,
		input:      textarea.New(),
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Repeat("line\n", 60))
	_ = m.tokenUsage.SetUsage(1500, 5000)
	m.refreshViewport()

	x := m.tokenUsage.bounds.x + max(0, m.tokenUsage.bounds.w/2)
	y := m.tokenUsage.bounds.y
	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      x,
		Y:      y,
	})
	updated := got.(model)
	if !updated.tokenUsage.hover {
		t.Fatalf("expected hover state to activate over token badge")
	}
}

func TestHandleAgentEventUsageUpdatedAccumulatesRealTokens(t *testing.T) {
	m := model{
		tokenUsage:  newTokenUsageComponent(),
		tokenBudget: 5000,
	}

	m.handleAgentEvent(agent.Event{
		Type: agent.EventUsageUpdated,
		Usage: llm.Usage{
			InputTokens:   120,
			OutputTokens:  40,
			ContextTokens: 30,
			TotalTokens:   190,
		},
	})

	if m.tokenUsedTotal != 190 {
		t.Fatalf("expected cumulative used tokens 190, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 120 || m.tokenOutput != 40 || m.tokenContext != 30 {
		t.Fatalf("unexpected token breakdown input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
	if m.tokenUsage.used != 190 {
		t.Fatalf("expected token component used value 190, got %d", m.tokenUsage.used)
	}
}

func TestAssistantDeltaDoesNotChangeUsageWithoutOfficialUsage(t *testing.T) {
	m := model{
		tokenUsage:  newTokenUsageComponent(),
		tokenBudget: 5000,
	}

	m.handleAgentEvent(agent.Event{Type: agent.EventRunStarted})
	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "This streamed delta should not change usage counters.",
	})

	if m.tokenUsedTotal != 0 || m.tokenOutput != 0 {
		t.Fatalf("expected no provisional usage without official usage, used=%d output=%d", m.tokenUsedTotal, m.tokenOutput)
	}

	m.handleAgentEvent(agent.Event{
		Type: agent.EventUsageUpdated,
		Usage: llm.Usage{
			InputTokens:   20,
			OutputTokens:  7,
			ContextTokens: 3,
			TotalTokens:   30,
		},
	})

	if m.tokenUsedTotal != 30 {
		t.Fatalf("expected total tokens to follow official total 30, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 20 || m.tokenOutput != 7 || m.tokenContext != 3 {
		t.Fatalf("expected official breakdown after calibration, got input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
}

func TestApplyUsageFallsBackToBreakdownWhenTotalIsZero(t *testing.T) {
	m := model{
		tokenUsage:  newTokenUsageComponent(),
		tokenBudget: 5000,
	}

	m.handleAgentEvent(agent.Event{
		Type: agent.EventUsageUpdated,
		Usage: llm.Usage{
			InputTokens:   11,
			OutputTokens:  5,
			ContextTokens: 4,
			TotalTokens:   0,
		},
	})

	if m.tokenUsedTotal != 20 {
		t.Fatalf("expected fallback sum of usage breakdown (20), got %d", m.tokenUsedTotal)
	}
}

func TestFetchRemoteTokenUsageCmdReturnsErrorMsgWhenConfigMissing(t *testing.T) {
	m := model{cfg: config.Config{}}
	cmd := m.fetchRemoteTokenUsageCmd()
	if cmd == nil {
		t.Fatalf("expected remote usage command")
	}
	msg := cmd()
	pulled, ok := msg.(tokenUsagePulledMsg)
	if !ok {
		t.Fatalf("expected tokenUsagePulledMsg, got %T", msg)
	}
	if pulled.Err == nil || !strings.Contains(pulled.Err.Error(), "missing base url or api key") {
		t.Fatalf("expected missing config error, got %v", pulled.Err)
	}
}

func TestFetchRemoteTokenUsageCmdReturnsUsageMsgOnSuccess(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"data":[{"results":[{"input_tokens":12,"output_tokens":8,"input_cached_tokens":3}]}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	m := model{
		cfg: config.Config{
			Provider: config.ProviderConfig{
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "test-key",
			},
		},
	}

	cmd := m.fetchRemoteTokenUsageCmd()
	msg := cmd()
	pulled, ok := msg.(tokenUsagePulledMsg)
	if !ok {
		t.Fatalf("expected tokenUsagePulledMsg, got %T", msg)
	}
	if pulled.Err != nil {
		t.Fatalf("expected successful usage pull message, got %v", pulled.Err)
	}
	if pulled.Used != 23 || pulled.Input != 12 || pulled.Output != 8 || pulled.Context != 3 {
		t.Fatalf("unexpected pulled usage payload: %+v", pulled)
	}
}

func TestUpdateTokenUsagePulledMsgIgnoredForSessionOnly(t *testing.T) {
	m := model{
		tokenUsage:     newTokenUsageComponent(),
		tokenBudget:    5000,
		tokenUsedTotal: 100,
		tokenInput:     60,
		tokenOutput:    20,
		tokenContext:   5,
	}

	got, _ := m.Update(tokenUsagePulledMsg{
		Used:    90,
		Input:   40,
		Output:  30,
		Context: 10,
	})
	updated := got.(model)
	if updated.tokenUsedTotal != 100 || updated.tokenInput != 60 || updated.tokenOutput != 20 || updated.tokenContext != 5 {
		t.Fatalf("expected remote usage pull to be ignored, got used=%d input=%d output=%d context=%d", updated.tokenUsedTotal, updated.tokenInput, updated.tokenOutput, updated.tokenContext)
	}

	got, _ = updated.Update(tokenUsagePulledMsg{Err: errors.New("boom")})
	still := got.(model)
	if still.tokenUsedTotal != updated.tokenUsedTotal || still.tokenInput != updated.tokenInput || still.tokenOutput != updated.tokenOutput || still.tokenContext != updated.tokenContext {
		t.Fatalf("expected error usage message to leave counters unchanged, got %+v", still)
	}
}

func TestAccumulateTokenUsageFallbackAndClamp(t *testing.T) {
	m := model{}
	m.accumulateTokenUsage([]llm.Message{
		{},
		{Usage: &llm.Usage{InputTokens: 10, OutputTokens: 4, ContextTokens: 1, TotalTokens: 0}},
		{Usage: &llm.Usage{InputTokens: -5, OutputTokens: 8, ContextTokens: 0, TotalTokens: -1}},
		{Usage: &llm.Usage{InputTokens: 1, OutputTokens: 1, ContextTokens: 1, TotalTokens: 20}},
	})

	if m.tokenUsedTotal != 38 {
		t.Fatalf("expected used total 38, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 11 || m.tokenOutput != 13 || m.tokenContext != 2 {
		t.Fatalf("unexpected breakdown input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
}

func TestRestoreTokenUsageFromSessionUsesCurrentSessionOnly(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	workspace := t.TempDir()
	current := session.New(workspace)
	current.Messages = []llm.Message{
		{Role: "assistant", Parts: []llm.Part{{Type: llm.PartText, Text: &llm.TextPart{Value: "ok"}}}, Usage: &llm.Usage{InputTokens: 30, OutputTokens: 20, ContextTokens: 10, TotalTokens: 60}},
	}
	other := session.New(workspace)
	other.Messages = []llm.Message{
		{Role: "assistant", Parts: []llm.Part{{Type: llm.PartText, Text: &llm.TextPart{Value: "ok"}}}, Usage: &llm.Usage{InputTokens: 200, OutputTokens: 100, ContextTokens: 50, TotalTokens: 350}},
	}
	if err := store.Save(current); err != nil {
		t.Fatalf("failed to save current session: %v", err)
	}
	if err := store.Save(other); err != nil {
		t.Fatalf("failed to save other session: %v", err)
	}

	m := model{
		store:     store,
		workspace: workspace,
	}
	m.restoreTokenUsageFromSession(current)

	if m.tokenUsedTotal != 60 {
		t.Fatalf("expected current session total 60, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 30 || m.tokenOutput != 20 || m.tokenContext != 10 {
		t.Fatalf("unexpected breakdown input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
}

func TestPlanModeDoesNotShowDetailedPlanPanel(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		width:     140,
		height:    24,
		input:     input,
		viewport:  viewport.New(0, 0),
		planView:  viewport.New(0, 0),
		mode:      modePlan,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		plan: planpkg.State{
			Phase: planpkg.PhaseReady,
			Goal:  "Create a plan",
			Steps: []planpkg.Step{
				{Title: "Step 1", Status: planpkg.StepInProgress},
				{Title: "Step 2", Status: planpkg.StepPending},
			},
		},
	}

	m.refreshViewport()

	if m.hasPlanPanel() {
		t.Fatalf("expected detailed plan panel to stay hidden in plan mode")
	}
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			if m.mouseOverPlan(x, y) {
				t.Fatalf("did not expect a mouse-active plan panel region in plan mode")
			}
		}
	}
}

func TestCtrlLFromLandingOpensSessions(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	m := model{
		screen:       screenLanding,
		sessionLimit: defaultSessionLimit,
		store:        store,
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	updated := got.(model)

	if !updated.sessionsOpen {
		t.Fatalf("expected ctrl+l on landing screen to open sessions")
	}
	if cmd == nil {
		t.Fatalf("expected ctrl+l on landing screen to trigger session loading")
	}
}

func TestCtrlGOpensAndClosesHelp(t *testing.T) {
	m := model{}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlG})
	opened := got.(model)
	if !opened.helpOpen {
		t.Fatalf("expected ctrl+g to open help")
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyCtrlG})
	closed := got.(model)
	if closed.helpOpen {
		t.Fatalf("expected ctrl+g to close help")
	}
}

func TestTabTogglesBetweenBuildAndPlanModes(t *testing.T) {
	m := model{
		mode: modeBuild,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	updated := got.(model)
	if updated.mode != modePlan {
		t.Fatalf("expected tab to switch to plan mode, got %q", updated.mode)
	}

	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	updated = got.(model)
	if updated.mode != modeBuild {
		t.Fatalf("expected second tab to switch back to build mode, got %q", updated.mode)
	}
}

func TestCtrlFOpensPromptSearchAndFiltersEntries(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "fix tui layout spacing"},
			{Prompt: "add model test case"},
			{Prompt: "review runner error handling"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	if !opened.promptSearchOpen {
		t.Fatalf("expected ctrl+f to open prompt search")
	}
	if len(opened.promptSearchMatches) != 3 {
		t.Fatalf("expected 3 prompt matches, got %d", len(opened.promptSearchMatches))
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
	filtered := got.(model)
	if filtered.promptSearchQuery != "test" {
		t.Fatalf("expected query to become test, got %q", filtered.promptSearchQuery)
	}
	if len(filtered.promptSearchMatches) != 1 {
		t.Fatalf("expected one filtered prompt, got %d", len(filtered.promptSearchMatches))
	}
	if !strings.Contains(filtered.promptSearchMatches[0].Prompt, "test case") {
		t.Fatalf("unexpected filtered prompt: %+v", filtered.promptSearchMatches[0])
	}
}

func TestCtrlFWhilePromptSearchOpenMovesSelection(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "first prompt"},
			{Prompt: "second prompt"},
			{Prompt: "third prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	if opened.promptSearchCursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", opened.promptSearchCursor)
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	moved := got.(model)
	if moved.promptSearchCursor != 1 {
		t.Fatalf("expected ctrl+f to move cursor to 1, got %d", moved.promptSearchCursor)
	}
}

func TestPromptSearchEnterRestoresSelectedPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("draft")
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "first prompt"},
			{Prompt: "second prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	down := got.(model)

	got, _ = down.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	applied := got.(model)
	if applied.promptSearchOpen {
		t.Fatalf("expected prompt search to close after enter")
	}
	if applied.input.Value() != "first prompt" {
		t.Fatalf("expected selected prompt to be restored, got %q", applied.input.Value())
	}
}

func TestPromptSearchEscRestoresOriginalInput(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("work in progress")
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "old prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("old")})
	filtered := got.(model)
	got, _ = filtered.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	closed := got.(model)

	if closed.promptSearchOpen {
		t.Fatalf("expected prompt search to close on esc")
	}
	if closed.input.Value() != "work in progress" {
		t.Fatalf("expected original input to be restored, got %q", closed.input.Value())
	}
}

func TestCtrlHDoesNotOpenPromptSearch(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "first prompt"},
			{Prompt: "second prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlH})
	updated := got.(model)
	if updated.promptSearchOpen {
		t.Fatalf("expected ctrl+h to have no prompt search binding")
	}
}

func TestPromptSearchQuerySupportsWorkspaceAndSessionFilters(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "fix test", Workspace: "repo-a", SessionID: "sess-alpha"},
			{Prompt: "fix test", Workspace: "repo-b", SessionID: "sess-beta"},
			{Prompt: "add docs", Workspace: "repo-a", SessionID: "sess-alpha"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix ws:repo-a sid:alpha")})
	filtered := got.(model)

	if len(filtered.promptSearchMatches) != 1 {
		t.Fatalf("expected one filtered match, got %d", len(filtered.promptSearchMatches))
	}
	match := filtered.promptSearchMatches[0]
	if match.Workspace != "repo-a" || match.SessionID != "sess-alpha" {
		t.Fatalf("unexpected filtered match: %+v", match)
	}
}

func TestPromptSearchPanelSupportsPageNavigation(t *testing.T) {
	input := textarea.New()
	input.Focus()
	entries := make([]history.PromptEntry, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, history.PromptEntry{Prompt: "prompt " + string(rune('a'+i))})
	}
	m := model{
		input:                input,
		promptHistoryLoaded:  true,
		promptHistoryEntries: entries,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	if opened.promptSearchCursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", opened.promptSearchCursor)
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	paged := got.(model)
	if paged.promptSearchCursor != promptSearchPageSize {
		t.Fatalf("expected pgdown to move cursor to %d, got %d", promptSearchPageSize, paged.promptSearchCursor)
	}

	got, _ = paged.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	back := got.(model)
	if back.promptSearchCursor != 0 {
		t.Fatalf("expected pgup to move cursor back to 0, got %d", back.promptSearchCursor)
	}
}

func TestStartupGuideSequentialFlowAdvancesAndClearsInput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.Focus()
	input.SetValue("openai-compatible")
	m := model{
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:  "openai-compatible",
				Model: "gpt-5.4",
			},
		},
		startupGuide: StartupGuide{
			Active:       true,
			Status:       "Bytemind needs a working API key before chat can start.",
			ConfigPath:   configPath,
			CurrentField: startupFieldType,
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !updated.startupGuide.Active {
		t.Fatalf("expected startup guide to remain active before api key")
	}
	if updated.startupGuide.CurrentField != startupFieldBaseURL {
		t.Fatalf("expected next step base_url, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after step submit, got %q", updated.input.Value())
	}

	updated.input.SetValue("https://api.deepseek.com")
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.cfg.Provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("expected base_url update, got %q", updated.cfg.Provider.BaseURL)
	}
	if updated.startupGuide.CurrentField != startupFieldModel {
		t.Fatalf("expected next step model, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after base_url submit, got %q", updated.input.Value())
	}

	updated.input.SetValue("deepseek-chat")
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected model update, got %q", updated.cfg.Provider.Model)
	}
	if updated.startupGuide.CurrentField != startupFieldAPIKey {
		t.Fatalf("expected next step api_key, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after model submit, got %q", updated.input.Value())
	}
}

func TestStartupGuideAcceptsValidKeyAndDisablesGuide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"provider":{"type":"openai-compatible","base_url":"`+server.URL+`","model":"gpt-5.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.Focus()
	input.SetValue("test-key")
	m := model{
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:      "openai-compatible",
				BaseURL:   server.URL,
				Model:     "gpt-5.4",
				APIKeyEnv: "BYTEMIND_API_KEY",
			},
		},
		startupGuide: StartupGuide{
			Active:       true,
			Status:       "Bytemind needs a working API key before chat can start.",
			ConfigPath:   configPath,
			CurrentField: startupFieldAPIKey,
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.startupGuide.Active {
		t.Fatalf("expected startup guide to be disabled after valid key")
	}
	if !strings.Contains(updated.statusNote, "Provider configured and verified") {
		t.Fatalf("unexpected status after setup: %q", updated.statusNote)
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `"api_key": "test-key"`) {
		t.Fatalf("expected config file to store api key, got %q", string(written))
	}
}

func TestStartupGuideSupportsModelAndBaseURLInput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.Focus()
	input.SetValue("model=deepseek-chat")
	m := model{
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:      "openai-compatible",
				BaseURL:   "https://api.openai.com/v1",
				Model:     "gpt-5.4",
				APIKeyEnv: "BYTEMIND_API_KEY",
			},
		},
		startupGuide: StartupGuide{
			Active:       true,
			Status:       "Bytemind needs a working API key before chat can start.",
			ConfigPath:   configPath,
			CurrentField: startupFieldType,
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !updated.startupGuide.Active {
		t.Fatalf("expected startup guide to remain active before key verification")
	}
	if updated.cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected model to update in memory, got %q", updated.cfg.Provider.Model)
	}
	if updated.startupGuide.CurrentField != startupFieldAPIKey {
		t.Fatalf("expected explicit model input to move to api_key step, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after model update, got %q", updated.input.Value())
	}

	updated.input.SetValue("base_url=https://api.deepseek.com")
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.cfg.Provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("expected base url to update in memory, got %q", updated.cfg.Provider.BaseURL)
	}
	if updated.startupGuide.CurrentField != startupFieldModel {
		t.Fatalf("expected explicit base_url input to move to model step, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after base_url update, got %q", updated.input.Value())
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"model": "deepseek-chat"`) {
		t.Fatalf("expected model to be persisted, got %q", string(raw))
	}
	if !strings.Contains(string(raw), `"base_url": "https://api.deepseek.com"`) {
		t.Fatalf("expected base_url to be persisted, got %q", string(raw))
	}
}

func TestStartupGuideStillAllowsSlashCommands(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/help")
	m := model{
		input: input,
		startupGuide: StartupGuide{
			Active: true,
			Status: "Startup check failed: API key unauthorized",
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !strings.Contains(updated.statusNote, "Help opened") {
		t.Fatalf("expected /help to execute under startup guide, got status %q", updated.statusNote)
	}
}

func TestRenderStartupGuidePanelInFooter(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	m := model{
		input: input,
		width: 100,
		startupGuide: StartupGuide{
			Active: true,
			Title:  "AI provider not ready",
			Status: "Startup check failed: missing API key",
			Lines:  []string{"1) Add API key"},
		},
	}

	footer := m.renderFooter()
	for _, want := range []string{"AI provider not ready", "missing API key", "Add API key"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("expected footer to contain %q", want)
		}
	}
}

func TestRefreshViewportPreservesManualScrollOffset(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		width:     100,
		height:    24,
		input:     input,
		viewport:  viewport.New(0, 0),
		planView:  viewport.New(0, 0),
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}
	for i := 0; i < 20; i++ {
		m.chatItems = append(m.chatItems, chatEntry{
			Kind:   "assistant",
			Title:  "Bytemind",
			Body:   strings.Repeat("message ", 12),
			Status: "final",
		})
	}

	m.chatAutoFollow = true
	m.refreshViewport()
	m.viewport.LineUp(5)
	m.chatAutoFollow = false
	beforeOffset := m.viewport.YOffset
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  "Bytemind",
		Body:   "new content should not force the viewport to jump",
		Status: "final",
	})

	m.refreshViewport()

	if m.viewport.YOffset != beforeOffset {
		t.Fatalf("expected manual scroll offset %d to be preserved, got %d", beforeOffset, m.viewport.YOffset)
	}
}

func TestContinueExecutionInputPreparesPlanAndSubmitsPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("\u7ee7\u7eed\u6267\u884c")
	input.CursorEnd()
	m := model{
		screen:    screenChat,
		width:     100,
		height:    24,
		input:     input,
		viewport:  viewport.New(0, 0),
		planView:  viewport.New(0, 0),
		mode:      modePlan,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		plan: planpkg.State{
			Goal:       "Finish plan mode",
			Phase:      planpkg.PhaseReady,
			NextAction: "Start: Implement continuation",
			Steps: []planpkg.Step{
				{Title: "Implement continuation", Status: planpkg.StepPending},
				{Title: "Verify workflow", Status: planpkg.StepPending},
			},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.mode != modeBuild {
		t.Fatalf("expected continue execution to switch to build mode, got %q", updated.mode)
	}
	if updated.plan.Phase != planpkg.PhaseExecuting {
		t.Fatalf("expected plan phase to become executing, got %q", updated.plan.Phase)
	}
	if len(updated.chatItems) < 1 {
		t.Fatalf("expected continue execution to submit a prompt")
	}
	if updated.chatItems[0].Body != "\u7ee7\u7eed\u6267\u884c" {
		t.Fatalf("expected original continue input to be appended, got %q", updated.chatItems[0].Body)
	}
	if updated.plan.Steps[0].Status != planpkg.StepInProgress {
		t.Fatalf("expected first pending step to become in progress, got %q", updated.plan.Steps[0].Status)
	}
}

func TestIsContinueExecutionInputSupportsPlanAlias(t *testing.T) {
	for _, input := range []string{"continue plan", "\u7ee7\u7eed"} {
		if !isContinueExecutionInput(input) {
			t.Fatalf("expected %q to be treated as continue input", input)
		}
	}
}

func TestWindowSizeMsgUpdatesViewportDimensions(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		input:     input,
	}

	got, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	updated := got.(model)

	if updated.viewport.Width <= 0 {
		t.Fatalf("expected viewport width to be updated, got %d", updated.viewport.Width)
	}
	if updated.viewport.Height <= 0 {
		t.Fatalf("expected viewport height to be updated, got %d", updated.viewport.Height)
	}
}

func TestSubmitPromptRecomputesInputWidthWhenEnteringChat(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:    screenLanding,
		width:     120,
		height:    36,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}
	m.syncLayoutForCurrentScreen()
	beforeWidth := lipgloss.Width(m.input.View())

	got, _ := m.submitPrompt("hello from landing")
	updated := got.(model)
	afterWidth := lipgloss.Width(updated.input.View())

	if updated.screen != screenChat {
		t.Fatalf("expected submit prompt to switch to chat screen")
	}
	if afterWidth <= beforeWidth {
		t.Fatalf("expected chat input width to expand after screen switch, got %d -> %d", beforeWidth, afterWidth)
	}
}

func TestChatViewOmitsRedundantChrome(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		width:     120,
		height:    36,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		input:     input,
	}

	m.syncLayoutForCurrentScreen()
	view := m.View()

	for _, unwanted := range []string{
		"Conversation",
		"Bytemind TUI",
		"? help",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("did not expect chat view to contain %q", unwanted)
		}
	}
	for _, wanted := range []string{
		"tab agents",
		"/ commands",
		"Ctrl+L sessions",
		"Ctrl+C copy/quit",
		"Build",
		"Plan",
	} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("expected chat view to contain %q", wanted)
		}
	}
	if strings.Contains(view, "PgUp/PgDn") {
		t.Fatalf("did not expect chat view to advertise PgUp/PgDn anymore")
	}
	if m.viewport.Height <= 20 {
		t.Fatalf("expected viewport height to stay roomy after removing header/footer text, got %d", m.viewport.Height)
	}
}

func TestRefreshViewportKeepsLatestMessagesVisible(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		width:     100,
		height:    24,
		input:     input,
		chatItems: make([]chatEntry, 0, 12),
	}
	for i := 0; i < 12; i++ {
		m.chatItems = append(m.chatItems, chatEntry{
			Kind:   "user",
			Title:  "You",
			Body:   strings.Repeat("message ", 8),
			Status: "final",
		})
	}

	m.refreshViewport()

	if m.viewport.YOffset == 0 {
		t.Fatalf("expected viewport to follow latest content")
	}
}
