package tui

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTokenUsageSetUsageClampsValues(t *testing.T) {
	c := newTokenUsageComponent()
	c.displayUsed = 0
	_ = c.SetUsage(9000, 5000)
	if c.used != 9000 || c.total != 5000 {
		t.Fatalf("expected fixed total quota with true used value, got used=%d total=%d", c.used, c.total)
	}
	_ = c.SetUsage(-5, 1000)
	if c.used != 0 {
		t.Fatalf("expected used to clamp to zero, got %d", c.used)
	}
}

func TestTokenUsageHoverShowsPercentage(t *testing.T) {
	c := newTokenUsageComponent()
	c.displayUsed = 1250
	_ = c.SetUsage(1250, 5000)
	c.SetBounds(10, 2, 20, 2)

	_, consumed := c.Update(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      12,
		Y:      2,
	})
	if !consumed {
		t.Fatalf("expected hover motion over badge to be consumed")
	}
	if !strings.Contains(c.usageText(), "%") {
		t.Fatalf("expected percentage text on hover, got %q", c.usageText())
	}
}

func TestTokenUsageClickShowsPopupAndTickHides(t *testing.T) {
	c := newTokenUsageComponent()
	c.displayUsed = 1000
	_ = c.SetUsage(1000, 5000)
	c.SetBounds(0, 0, 20, 2)

	cmd, consumed := c.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      1,
		Y:      0,
	})
	if !consumed {
		t.Fatalf("expected click on badge to be consumed")
	}
	if cmd == nil {
		t.Fatalf("expected click to schedule follow-up tick")
	}
	if !c.popup {
		t.Fatalf("expected popup to be visible after click")
	}

	c.popupUntil = time.Now().Add(-time.Millisecond)
	_, _ = c.Update(tokenMonitorTickMsg(time.Now()))
	if c.popup {
		t.Fatalf("expected popup to auto-hide after timeout")
	}
}

func TestTokenUsagePopupUsesRealBreakdown(t *testing.T) {
	c := newTokenUsageComponent()
	_ = c.SetUsage(300, 5000)
	c.SetBreakdown(120, 140, 40)
	c.popup = true

	view := c.PopupView()
	for _, want := range []string{"Input:   120", "Output:  140", "Context: 40"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected popup to contain %q, got %q", want, view)
		}
	}
}

func TestFormatIntWithCommas(t *testing.T) {
	if got := formatInt(1234567); got != "1,234,567" {
		t.Fatalf("expected comma formatted number, got %q", got)
	}
}

func TestNewTokenUsageComponentEnvFlags(t *testing.T) {
	t.Setenv("BYTEMIND_TOKEN_MONITOR_SIMPLE", "1")
	c := newTokenUsageComponent()
	if !c.simpleRing {
		t.Fatalf("expected simple ring mode from env flag")
	}
	if !c.noBraille {
		t.Fatalf("expected no-braille mode when simple mode is enabled")
	}
}

func TestTokenUsageSetPriceAndEstimatedCost(t *testing.T) {
	c := newTokenUsageComponent()
	c.SetPrice(1.2, 2.4)
	c.SetBreakdown(1_000_000, 500_000, 0)
	c.popup = true

	got := c.estimatedCost()
	if math.Abs(got-2.4) > 1e-9 {
		t.Fatalf("expected estimated cost 2.4, got %f", got)
	}
	if !strings.Contains(c.popupText(), "Estimated Cost: $2.400000") {
		t.Fatalf("expected popup text to include estimated cost, got %q", c.popupText())
	}
}

func TestTokenUsageUpdateMouseBranches(t *testing.T) {
	c := newTokenUsageComponent()
	c.total = 0
	c.hover = true
	cmd, consumed := c.Update(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      2,
		Y:      2,
	})
	if cmd != nil || consumed {
		t.Fatalf("expected zero-total motion to be ignored")
	}
	if c.hover {
		t.Fatalf("expected zero-total motion to clear hover state")
	}

	_ = c.SetUsage(100, 5000)
	c.SetBounds(0, 0, 10, 1)
	c.popup = true
	cmd, consumed = c.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      20,
		Y:      20,
	})
	if cmd != nil || consumed {
		t.Fatalf("expected outside click not to be consumed")
	}
	if c.popup {
		t.Fatalf("expected outside click to hide popup")
	}

	_, consumed = c.Update(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		X:      1,
		Y:      0,
	})
	if !consumed {
		t.Fatalf("expected release inside badge to be marked as inside")
	}
}

func TestTokenUsageTickAnimationProgress(t *testing.T) {
	c := newTokenUsageComponent()
	c.displayUsed = 0
	_ = c.SetUsage(1000, 5000)
	cmd, _ := c.Update(tokenMonitorTickMsg(time.Now()))
	if cmd == nil {
		t.Fatalf("expected tick to continue while display value is catching up")
	}
	if c.displayUsed <= 0 || c.displayUsed >= 1000 {
		t.Fatalf("expected display value to move toward used amount, got %f", c.displayUsed)
	}

	c.displayUsed = float64(c.used)
	c.popup = false
	cmd, _ = c.Update(tokenMonitorTickMsg(time.Now()))
	if cmd != nil {
		t.Fatalf("expected no follow-up tick when display is already synchronized")
	}
}

func TestTokenUsageRenderRingModes(t *testing.T) {
	c := newTokenUsageComponent()
	_ = c.SetUsage(2500, 5000)
	c.displayUsed = 2500

	c.simpleRing = true
	if got := c.renderRing(); got == "" {
		t.Fatalf("expected simple ring render output")
	}

	c.simpleRing = false
	c.noBraille = true
	if got := c.renderRing(); got == "" {
		t.Fatalf("expected non-braille ring render output")
	}

	c.noBraille = false
	if got := c.renderRing(); got == "" {
		t.Fatalf("expected braille ring render output")
	}
}

func TestTokenUsageHelpers(t *testing.T) {
	c := newTokenUsageComponent()
	c.displayUsed = 200
	_ = c.SetUsage(200, 1000)

	x, y, w, h := c.Layout(80)
	if x < 0 || y < 0 || w <= 0 || h <= 0 {
		t.Fatalf("expected non-negative layout with positive dimensions, got (%d,%d,%d,%d)", x, y, w, h)
	}
	c.SetBounds(x, y, w, h)
	if !c.contains(x, y) {
		t.Fatalf("expected bounds to include top-left point")
	}
	if c.contains(x+w, y+h) {
		t.Fatalf("expected bounds to exclude bottom-right outside point")
	}

	if ringGlyph(2, "F", "H") != "F" || ringGlyph(1, "F", "H") != "H" || ringGlyph(0, "F", "H") != " " {
		t.Fatalf("unexpected ring glyph mapping")
	}

	if got := toHex(rgb{r: 300, g: -1, b: 16}); got != "#FF0010" {
		t.Fatalf("expected clamped hex conversion, got %q", got)
	}
	mid := lerpRGB(rgb{r: 0, g: 0, b: 0}, rgb{r: 100, g: 200, b: 50}, 0.5)
	if mid.r != 50 || mid.g != 100 || mid.b != 25 {
		t.Fatalf("unexpected lerp rgb result: %+v", mid)
	}
}

func TestReadEnvFlag(t *testing.T) {
	t.Setenv("BYTEMIND_TOKEN_MONITOR_TEST_FLAG", "yes")
	if !readEnvFlag("BYTEMIND_TOKEN_MONITOR_TEST_FLAG") {
		t.Fatalf("expected env flag to parse affirmative value")
	}
	if readEnvFlag("BYTEMIND_TOKEN_MONITOR_TEST_MISSING") {
		t.Fatalf("expected missing env flag to parse as false")
	}
	t.Setenv("BYTEMIND_TOKEN_MONITOR_TEST_FLAG", "off")
	if readEnvFlag("BYTEMIND_TOKEN_MONITOR_TEST_FLAG") {
		t.Fatalf("expected non-affirmative env flag to parse as false")
	}

	_ = os.Setenv("BYTEMIND_TOKEN_MONITOR_TEST_FLAG", "on")
}

func TestTokenUsageCompactViewAndText(t *testing.T) {
	c := newTokenUsageComponent()
	_ = c.SetUsage(1234, 5000)
	c.displayUsed = 1234
	if got := c.CompactView(); !strings.Contains(got, "1,234") {
		t.Fatalf("expected compact view to include compact usage text, got %q", got)
	}

	c.hover = true
	if got := c.compactUsageText(); !strings.Contains(got, "%") {
		t.Fatalf("expected compact usage text to show percent while hovered, got %q", got)
	}
}

func TestMaxFloat(t *testing.T) {
	if got := maxFloat(1.5, 2.0); got != 2.0 {
		t.Fatalf("expected max float 2.0, got %f", got)
	}
	if got := maxFloat(3.0, 2.0); got != 3.0 {
		t.Fatalf("expected max float 3.0, got %f", got)
	}
}
