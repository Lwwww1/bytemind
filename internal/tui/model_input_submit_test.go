package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytemind/internal/config"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterSubmitsPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("ship this prompt")
	input.CursorEnd()

	m := model{
		screen:         screenChat,
		input:          input,
		workspace:      "E:\\bytemind",
		sess:           session.New("E:\\bytemind"),
		streamingIndex: -1,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 1 {
		t.Fatalf("expected enter to submit prompt, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "ship this prompt" {
		t.Fatalf("expected submitted user prompt to match input, got %q", updated.chatItems[0].Body)
	}
}

func TestAltEnterInsertsNewlineWithoutSubmitting(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected alt+enter not to submit prompt")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected alt+enter to insert newline, got %q", updated.input.Value())
	}
}

func TestShiftEnterInsertsNewlineWithoutSubmitting(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shift+enter")})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected shift+enter not to submit")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected shift+enter to insert newline, got %q", updated.input.Value())
	}
}
func TestCtrlJInsertsNewlineWithoutSubmitting(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected ctrl+j not to submit prompt")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected ctrl+j to insert newline, got %q", updated.input.Value())
	}
}

func TestAltVPastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected alt+v to paste clipboard image placeholder, got %q", updated.input.Value())
	}
	if !strings.Contains(updated.statusNote, "Attached image from clipboard") {
		t.Fatalf("expected clipboard status note, got %q", updated.statusNote)
	}
}

func TestCtrlVPastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected ctrl+v to paste clipboard image placeholder, got %q", updated.input.Value())
	}
}

func TestCtrlVControlMarkerRunePastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x16'}})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected ctrl+v control marker to paste clipboard image placeholder, got %q", updated.input.Value())
	}
}

func TestCtrlVWithoutImageShowsStatusNote(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		err: errors.New("clipboard has no image"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	updated := got.(model)
	if updated.input.Value() != "" {
		t.Fatalf("expected input to stay empty, got %q", updated.input.Value())
	}
	if !strings.Contains(strings.ToLower(updated.statusNote), "clipboard has no image") {
		t.Fatalf("expected no-image status note, got %q", updated.statusNote)
	}
}

func TestTerminalPasteEventWithEmptyPayloadPastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Paste: true})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected empty paste event to attach clipboard image, got %q", updated.input.Value())
	}
}

func TestTerminalPasteEventWithTextDoesNotForceClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		err: errors.New("clipboard image unavailable"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello"), Paste: true})
	updated := got.(model)
	if updated.input.Value() != "hello" {
		t.Fatalf("expected text paste to remain text, got %q", updated.input.Value())
	}
	if strings.Contains(updated.input.Value(), "[Image #") {
		t.Fatalf("expected no image placeholder for text paste")
	}
}

func TestRapidRuneInputForImagePathTriggersFallbackPlaceholder(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat

	imagePath := filepath.Join(m.workspace, "drag.jpg")
	if err := os.WriteFile(imagePath, []byte("jpg-bytes"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	for _, r := range imagePath {
		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		next := got.(model)
		m = &next
	}
	if m.input.Value() != "[Image #1]" {
		t.Fatalf("expected rapid path input to convert to placeholder, got %q", m.input.Value())
	}
}

func TestImmediateEnterAfterPasteStillSubmits(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 1 {
		t.Fatalf("expected enter to submit immediately, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "first line" {
		t.Fatalf("expected submitted body to match input text, got %q", updated.chatItems[0].Body)
	}
}

func TestPasteEnterDoesNotSubmitAndKeepsNewline(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected paste enter not to submit, got %d chat items", len(updated.chatItems))
	}
	if !strings.Contains(updated.input.Value(), "\n") {
		t.Fatalf("expected pasted enter to be inserted as newline, got %q", updated.input.Value())
	}
}

func TestSuppressedEnterAfterPasteIsInsertedAsNewline(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("line1")
	input.CursorEnd()

	m := model{
		screen:         screenChat,
		input:          input,
		workspace:      "E:\\bytemind",
		sess:           session.New("E:\\bytemind"),
		lastPasteAt:    time.Now(),
		lastInputAt:    time.Now(),
		inputBurstSize: 12,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected suppressed enter not to submit, got %d chat items", len(updated.chatItems))
	}
	if updated.input.Value() != "line1\n" {
		t.Fatalf("expected suppressed enter to become newline, got %q", updated.input.Value())
	}
}

func TestEnterSubmitsMultilinePrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line\nsecond line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 1 {
		t.Fatalf("expected enter to submit multiline prompt, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "first line\nsecond line" {
		t.Fatalf("expected multiline body to be preserved, got %q", updated.chatItems[0].Body)
	}
}

func TestHelpTextOnlyMentionsSupportedEntryPoints(t *testing.T) {
	text := model{}.helpText()

	for _, unwanted := range []string{
		"scripts\\install.ps1",
		"aicoding chat",
		"aicoding run",
		"/plan",
		"/skill use",
		"/skill show",
	} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("help text should not mention %q", unwanted)
		}
	}

	for _, wanted := range []string{
		"go run ./cmd/bytemind chat",
		"go run ./cmd/bytemind run -prompt",
		"/session",
		"/quit",
		"/new",
		"Ctrl+G",
		"continue execution",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("help text should mention %q", wanted)
		}
	}
}

func TestRenderFooterOnlyShowsInputRegion(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
	}

	footer := m.renderFooter()
	for _, unwanted := range []string{
		"Up/Down history",
		"Ctrl+Up/Down scroll",
		"? help",
		"Enter send",
		"Ctrl+N new session",
	} {
		if strings.Contains(footer, unwanted) {
			t.Fatalf("footer should not advertise %q", unwanted)
		}
	}
	for _, wanted := range []string{
		"tab agents",
		"/ commands",
		"Ctrl+L sessions",
		"Ctrl+C copy/quit",
	} {
		if !strings.Contains(footer, wanted) {
			t.Fatalf("footer should advertise %q", wanted)
		}
	}
	if strings.Contains(footer, "PgUp/PgDn") {
		t.Fatalf("footer should not advertise PgUp/PgDn anymore")
	}
}

func TestRenderFooterInfoLineCombinesModeAndHints(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 160,
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{Model: "deepseek-chat"},
		},
	}

	footer := m.renderFooter()
	lines := strings.Split(footer, "\n")
	infoLine := ""
	for _, line := range lines {
		if strings.Contains(line, "tab agents") {
			infoLine = line
			break
		}
	}
	if infoLine == "" {
		t.Fatalf("expected footer to contain a quick-hint info line")
	}
	for _, want := range []string{"Build", "Plan", "deepseek-chat", "tab agents"} {
		if !strings.Contains(infoLine, want) {
			t.Fatalf("expected combined info line to contain %q, got %q", want, infoLine)
		}
	}
}

func TestRenderStatusBarShowsCurrentRuntimeState(t *testing.T) {
	m := model{
		width:          200,
		mode:           modeBuild,
		phase:          "thinking",
		chatAutoFollow: false,
		cfg: config.Config{
			Provider: config.ProviderConfig{Model: "deepseek-chat"},
		},
		sess: &session.Session{ID: "1234567890abcdef"},
		plan: planpkg.State{
			Phase: planpkg.PhaseExecuting,
			Steps: []planpkg.Step{
				{Title: "Implement plan resumption", Status: planpkg.StepInProgress},
			},
		},
	}

	bar := m.renderStatusBar()
	for _, want := range []string{
		"Mode: BUILD",
		"Phase: executing",
		"Session: 1234567890ab",
		"Step: Implement plan resumption",
		"Follow: manual",
		"Model: deepseek-chat",
	} {
		if !strings.Contains(bar, want) {
			t.Fatalf("expected status bar to contain %q", want)
		}
	}
}

func TestSyncInputStyleUsesSingleLineSearchField(t *testing.T) {
	input := textarea.New()
	m := model{
		screen: screenChat,
		input:  input,
	}

	m.syncInputStyle()

	if m.input.Prompt != "" {
		t.Fatalf("expected empty prompt, got %q", m.input.Prompt)
	}
	if m.input.Placeholder != "Ask Bytemind to inspect, change, or verify this workspace..." {
		t.Fatalf("unexpected placeholder: %q", m.input.Placeholder)
	}
}

func TestSyncInputStyleShowsStartupStepPlaceholder(t *testing.T) {
	input := textarea.New()
	m := model{
		input: input,
		startupGuide: StartupGuide{
			Active:       true,
			CurrentField: startupFieldModel,
		},
	}

	m.syncInputStyle()

	if !strings.Contains(m.input.Placeholder, "Step 3/4") {
		t.Fatalf("expected startup step placeholder, got %q", m.input.Placeholder)
	}
	if !strings.Contains(m.input.Placeholder, "model") {
		t.Fatalf("expected startup model placeholder, got %q", m.input.Placeholder)
	}
}
