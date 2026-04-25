package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderInputEditorViewDefaultUsesLandingVariantOnLandingScreen(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("hello")
	input.SetCursor(2)

	m := model{
		screen:          screenLanding,
		landingGlowStep: 0,
		input:           input,
	}
	m.syncInputStyle()

	got := xansi.Strip(renderInputEditorViewDefault(m))
	want := xansi.Strip(m.renderLandingInputEditorView())
	if got != want {
		t.Fatalf("expected landing renderer output, got %q want %q", got, want)
	}
}

func TestApplyInputThemesAllowNilModelReceiver(t *testing.T) {
	var m *model
	m.applyInputThemeForScreen()
	m.applyDefaultInputTheme()
	m.applyLandingInputTheme()
}
