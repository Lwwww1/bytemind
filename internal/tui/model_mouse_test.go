package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleMouseScrollsViewport(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	updated := got.(model)
	if updated.viewport.YOffset == 0 {
		t.Fatalf("expected viewport to scroll down, got offset %d", updated.viewport.YOffset)
	}
}

func TestNormalizeMouseMsgAppliesYOffset(t *testing.T) {
	m := model{mouseYOffset: 2}
	msg := tea.MouseMsg{X: 10, Y: 8}
	got := m.normalizeMouseMsg(msg)
	if got.X != 10 || got.Y != 10 {
		t.Fatalf("expected normalized mouse msg to keep X and shift Y by offset, got %+v", got)
	}
}

func TestResolveMouseYOffsetFromEnv(t *testing.T) {
	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "2")
	if got := resolveMouseYOffset(); got != 2 {
		t.Fatalf("expected env-configured y offset 2, got %d", got)
	}

	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "99")
	if got := resolveMouseYOffset(); got != 10 {
		t.Fatalf("expected y offset to clamp to 10, got %d", got)
	}
}

func TestResolveMouseYOffsetDefaultIsZero(t *testing.T) {
	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "")
	if got := resolveMouseYOffset(); got != 0 {
		t.Fatalf("expected default y offset 0, got %d", got)
	}
}

func TestHandleMouseWheelUpScrollsViewport(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			vp.LineDown(4)
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})
	updated := got.(model)
	if updated.viewport.YOffset >= m.viewport.YOffset {
		t.Fatalf("expected viewport to scroll up, got offset %d", updated.viewport.YOffset)
	}
}

func TestHandleMouseEnablesViewportMouseForwarding(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			vp.MouseWheelEnabled = false
			vp.MouseWheelDelta = 0
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	updated := got.(model)
	if !updated.viewport.MouseWheelEnabled {
		t.Fatalf("expected mouse wheel support to be enabled for viewport updates")
	}
	if updated.viewport.MouseWheelDelta != scrollStep {
		t.Fatalf("expected viewport wheel delta %d, got %d", scrollStep, updated.viewport.MouseWheelDelta)
	}
	if updated.viewport.YOffset == 0 {
		t.Fatalf("expected mouse wheel to scroll viewport")
	}
}

func TestHandleMouseDragSelectionArmsCopyableSelection(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:        screenChat,
		width:         120,
		height:        28,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("alpha line\nbeta line\ngamma line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatal("expected conversation viewport bounds to be available")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      top,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      left + 4,
		Y:      top,
	})
	moved := got.(model)

	got, _ = moved.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left + 4,
		Y:      top,
	})
	released := got.(model)

	if writer.last != "" {
		t.Fatalf("expected drag release not to copy before ctrl+c, got %q", writer.last)
	}
	if !released.mouseSelectionActive {
		t.Fatalf("expected drag release to keep an active selection")
	}
	if !strings.Contains(released.statusNote, "Press Ctrl+C to copy") {
		t.Fatalf("expected copy hint after drag selection, got %q", released.statusNote)
	}
}

func TestHandleMouseReleaseAtDifferentPointArmsSelectionWithoutMotion(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:        screenChat,
		width:         120,
		height:        28,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("alpha line\nbeta line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatal("expected conversation viewport bounds to be available")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      top,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left + 4,
		Y:      top,
	})
	released := got.(model)

	if writer.last != "" {
		t.Fatalf("expected release-with-range not to copy before ctrl+c, got %q", writer.last)
	}
	if !released.mouseSelectionActive {
		t.Fatalf("expected release at different point to keep an active selection")
	}
	if !strings.Contains(released.statusNote, "Press Ctrl+C to copy") {
		t.Fatalf("expected copy hint after selection, got %q", released.statusNote)
	}
}

func TestHandleMouseSingleClickStartsSelectionWithoutCopy(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:        screenChat,
		width:         120,
		height:        28,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("alpha line\nbeta line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatal("expected conversation viewport bounds to be available")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left + 2,
		Y:      top,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left + 2,
		Y:      top,
	})
	released := got.(model)

	if writer.last != "" {
		t.Fatalf("expected click without drag not to copy text, got %q", writer.last)
	}
	if released.mouseSelecting {
		t.Fatalf("expected click without drag to leave selection mode")
	}
	if released.mouseSelectionActive {
		t.Fatalf("expected click without drag not to keep an active selection")
	}
}

func TestCtrlCCopiesActiveSelectionAndShowsToast(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:               screenChat,
		width:                120,
		height:               28,
		input:                input,
		viewport:             viewport.New(60, 10),
		tokenUsage:           newTokenUsageComponent(),
		clipboardText:        writer,
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 4},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := got.(model)

	if writer.last != "alpha" {
		t.Fatalf("expected ctrl+c copied selection %q, got %q", "alpha", writer.last)
	}
	if updated.mouseSelectionActive {
		t.Fatalf("expected successful copy to clear active selection")
	}
	if updated.selectionToast != "Copied selection" {
		t.Fatalf("expected copy toast, got %q", updated.selectionToast)
	}
	if cmd == nil {
		t.Fatalf("expected ctrl+c copy to schedule toast expiry")
	}
}

func TestCtrlCCopyFailureKeepsSelectionAndSetsStatus(t *testing.T) {
	writer := &fakeClipboardTextWriter{err: errors.New("clipboard write failed")}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:               screenChat,
		width:                120,
		height:               28,
		input:                input,
		viewport:             viewport.New(60, 10),
		tokenUsage:           newTokenUsageComponent(),
		clipboardText:        writer,
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 2},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	released := got.(model)

	if !strings.Contains(released.statusNote, "clipboard write failed") {
		t.Fatalf("expected copy error in status note, got %q", released.statusNote)
	}
	if !released.mouseSelectionActive {
		t.Fatalf("expected failed copy to keep active selection")
	}
}

func TestCtrlCCopyTimeoutKeepsSelectionAndSetsTimeoutStatus(t *testing.T) {
	writer := &fakeClipboardTextWriter{waitForCtx: true}
	input := textarea.New()
	input.Focus()

	previousTimeout := clipboardWriteTimeout
	clipboardWriteTimeout = 5 * time.Millisecond
	defer func() { clipboardWriteTimeout = previousTimeout }()

	m := model{
		screen:               screenChat,
		width:                120,
		height:               28,
		input:                input,
		viewport:             viewport.New(60, 10),
		tokenUsage:           newTokenUsageComponent(),
		clipboardText:        writer,
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 2},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := got.(model)
	if !strings.Contains(updated.statusNote, "timed out") {
		t.Fatalf("expected timeout status note, got %q", updated.statusNote)
	}
	if !updated.mouseSelectionActive {
		t.Fatalf("expected timeout to keep active selection")
	}
}

func TestRenderConversationCopyUsesPlainMessageText(t *testing.T) {
	m := model{
		width:  120,
		height: 28,
		viewport: func() viewport.Model {
			vp := viewport.New(60, 10)
			return vp
		}(),
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "line one\nline two", Status: "final"},
		},
	}

	got := m.renderConversationCopy()
	if strings.Contains(got, "\u2502") || strings.Contains(got, "\u2503") {
		t.Fatalf("expected copy conversation without card borders, got %q", got)
	}
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Fatalf("expected copy conversation to contain message body, got %q", got)
	}
}

func TestHandleMouseWheelScrollsInputWhenPointerIsOverInput(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("line1\nline2\nline3\nline4\nline5\nline6")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		width:     100,
		height:    24,
		input:     input,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			vp.LineDown(2)
			return vp
		}(),
	}

	beforeLine := m.input.Line()
	beforeOffset := m.viewport.YOffset
	inputY := -1
	for y := 0; y < m.height; y++ {
		if m.mouseOverInput(y) {
			inputY = y
			break
		}
	}
	if inputY < 0 {
		t.Fatalf("expected to find chat input region")
	}
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
		Y:      inputY,
	})
	updated := got.(model)

	if updated.input.Line() >= beforeLine {
		t.Fatalf("expected input cursor to move up, got line %d -> %d", beforeLine, updated.input.Line())
	}
	if updated.viewport.YOffset != beforeOffset {
		t.Fatalf("expected conversation viewport to stay put, got offset %d -> %d", beforeOffset, updated.viewport.YOffset)
	}
}

func TestHandleMouseWheelScrollsLandingInputWhenPointerIsOverInput(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("line1\nline2\nline3\nline4\nline5\nline6")
	input.CursorEnd()

	m := model{
		screen: screenLanding,
		width:  100,
		height: 32,
		input:  input,
	}

	beforeLine := m.input.Line()
	inputY := -1
	for y := 0; y < m.height; y++ {
		if m.mouseOverInput(y) {
			inputY = y
			break
		}
	}
	if inputY < 0 {
		t.Fatalf("expected to find landing input region")
	}
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
		Y:      inputY,
	})
	updated := got.(model)

	if updated.input.Line() >= beforeLine {
		t.Fatalf("expected landing input cursor to move up, got line %d -> %d", beforeLine, updated.input.Line())
	}
}

func TestWrapPlainTextPrefersWordBoundariesForEnglish(t *testing.T) {
	text := "Risks - this section should keep words intact"
	got := wrapPlainText(text, 8)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Ris") && !strings.Contains(line, "Risks") {
			t.Fatalf("expected not to split 'Risks' across lines, got %q", got)
		}
		if strings.Contains(line, "Act") && !strings.Contains(line, "Action") {
			t.Fatalf("expected not to split words abruptly, got %q", got)
		}
	}
}
