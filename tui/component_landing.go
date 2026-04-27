package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

var landingShortcutHints = []footerShortcutHint{
	{Key: "/", Label: "commands"},
	{Key: "Ctrl+L", Label: "sessions"},
	{Key: "Ctrl+C", Label: "quit"},
}

func (m model) renderLandingHero() string {
	innerWidth := m.landingPromptHeroWidth()
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#1E2E3A"))
	headerHostStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64DF69"))
	promptSigilStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64DF69")).Bold(true)
	brandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E2F1FF")).Bold(true)
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5B7288"))
	dotMutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4A5F72"))
	dotActiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64DF69"))

	headerHost := headerHostStyle.Render("bytemind@localhost:~")
	dots := strings.Join([]string{
		dotMutedStyle.Render("o"),
		dotMutedStyle.Render("o"),
		dotActiveStyle.Render("o"),
	}, " ")
	headerGap := max(1, innerWidth-lipgloss.Width(headerHost)-lipgloss.Width(dots))
	headerRow := padLandingANSI(headerHost+strings.Repeat(" ", headerGap)+dots, innerWidth)

	promptRow := padLandingANSI("  "+promptSigilStyle.Render(">_ ")+brandStyle.Render("Bytemind"), innerWidth)
	blankRow := strings.Repeat(" ", innerWidth)
	cursorGlyph := " "
	if m.landingPromptCursorVisible() {
		cursorGlyph = "_"
	}
	cursorRow := padLandingANSI("  "+cursorStyle.Render(cursorGlyph), innerWidth)

	frame := strings.Join([]string{
		borderStyle.Render("+" + strings.Repeat("-", innerWidth) + "+"),
		borderStyle.Render("|") + headerRow + borderStyle.Render("|"),
		borderStyle.Render("+" + strings.Repeat("-", innerWidth) + "+"),
		borderStyle.Render("|") + promptRow + borderStyle.Render("|"),
		borderStyle.Render("|") + blankRow + borderStyle.Render("|"),
		borderStyle.Render("|") + cursorRow + borderStyle.Render("|"),
		borderStyle.Render("+" + strings.Repeat("-", innerWidth) + "+"),
	}, "\n")

	subtitle := landingSubtitleStyle.Render("Your AI assistant")
	return frame + "\n\n" + subtitle
}

func (m model) landingPromptHeroWidth() int {
	if m.width <= 0 {
		return 72
	}
	maxFit := max(24, m.width-14)
	preferred := min(90, max(60, (m.width*3)/4))
	return clamp(preferred, 48, maxFit)
}

func (m model) landingPromptCursorVisible() bool {
	const blinkHalfCycle = 8
	return (m.landingGlowStep/blinkHalfCycle)%2 == 0
}

func padLandingANSI(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if w := lipgloss.Width(text); w < width {
		return text + strings.Repeat(" ", width-w)
	}
	return xansi.Cut(text, 0, width)
}

func (m model) renderLandingOverlayPanel() string {
	switch {
	case m.startupGuide.Active:
		return m.renderStartupGuidePanel()
	case m.promptSearchOpen:
		return m.renderPromptSearchPalette()
	case m.mentionOpen:
		return m.renderMentionPalette()
	case m.commandOpen:
		return m.renderCommandPalette()
	default:
		return ""
	}
}

func (m model) renderLandingInputBox(markZone bool) string {
	editor := m.renderInputEditorView()
	editor = strings.TrimRight(editor, "\n")
	if editor == "" {
		editor = " "
	}
	editor = ensureMinRows(editor, 2)
	editor = landingInputEditorSurfaceStyle.Copy().
		Width(m.landingInputContentWidth()).
		Render(editor)
	if markZone {
		editor = zone.Mark(inputEditorZoneID, editor)
	}
	return landingInputStyle.Copy().
		BorderForeground(m.modeAccentColor()).
		Width(m.landingInputShellWidth()).
		Render(editor)
}

func ensureMinRows(text string, rows int) string {
	if rows <= 1 {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) renderLandingInputActions() string {
	actions := landingActionKeyStyle.Render("Enter") + " " + landingActionLabelStyle.Render("send") +
		landingActionDividerStyle.Render(" | ") +
		landingActionKeyStyle.Render("Shift+Enter") + " " + landingActionLabelStyle.Render("newline")
	return landingHintStyle.Render(actions)
}

func (m model) renderLandingModeTabs() string {
	buildStyle := landingModeInactiveStyle
	planStyle := landingModeInactiveStyle
	if m.mode == modeBuild {
		buildStyle = landingModeBuildActiveStyle
	} else {
		planStyle = landingModePlanActiveStyle
	}
	sep := landingModeInactiveStyle.Render("   ")
	return buildStyle.Render("Build") +
		sep +
		planStyle.Render("Plan")
}

func renderLandingShortcutHints() string {
	parts := make([]string, 0, len(landingShortcutHints))
	for _, hint := range landingShortcutHints {
		parts = append(parts, landingShortcutKeyStyle.Render(hint.Key)+" "+landingShortcutLabelStyle.Render(hint.Label))
	}
	return strings.Join(parts, landingShortcutDividerStyle.Render("  |  "))
}

func (m model) renderLandingContent(markInputZone bool) string {
	parts := []string{
		m.renderLandingHero(),
		"",
		m.renderLandingModeTabs(),
	}
	if overlay := strings.TrimSpace(m.renderLandingOverlayPanel()); overlay != "" {
		parts = append(parts, "", overlay)
	}
	parts = append(
		parts,
		"",
		m.renderLandingInputBox(markInputZone),
		m.renderLandingInputActions(),
		"",
		renderLandingShortcutHints(),
	)
	return strings.Join(parts, "\n")
}

func (m model) landingContentHeight() int {
	return lipgloss.Height(m.renderLandingContent(false))
}

func (m model) landingContentTop(contentHeight int) int {
	return max(0, (m.height-contentHeight)/2+1)
}

func (m model) landingInputTop(contentTop int) int {
	top := contentTop + lipgloss.Height(m.renderLandingHero()) + 1 + lipgloss.Height(m.renderLandingModeTabs())
	if overlay := strings.TrimSpace(m.renderLandingOverlayPanel()); overlay != "" {
		top += 1 + lipgloss.Height(overlay)
	}
	return top + 1
}

func (m model) renderLandingCanvas(content string) string {
	if m.width <= 0 || m.height <= 0 {
		return landingCanvasStyle.Render(content)
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	rows := make([]string, 0, m.height)
	topPad := m.landingContentTop(len(lines))
	for i := 0; i < topPad && len(rows) < m.height; i++ {
		rows = append(rows, m.renderLandingCanvasRow("", len(rows)))
	}
	for _, line := range lines {
		if len(rows) >= m.height {
			break
		}
		rows = append(rows, m.renderLandingCanvasRow(line, len(rows)))
	}
	for len(rows) < m.height {
		rows = append(rows, m.renderLandingCanvasRow("", len(rows)))
	}
	return strings.Join(rows, "\n")
}

func (m model) renderLandingCanvasRow(line string, row int) string {
	rowStyle := lipgloss.NewStyle().Background(m.landingGradientColor(row))
	if strings.TrimSpace(line) == "" {
		return rowStyle.Width(m.width).Render("")
	}
	lineWidth := lipgloss.Width(line)
	if lineWidth >= m.width {
		return xansi.Cut(line, 0, m.width)
	}
	left := max(0, (m.width-lineWidth)/2)
	right := max(0, m.width-left-lineWidth)
	return rowStyle.Width(left).Render("") + rowStyle.Render(line) + rowStyle.Width(right).Render("")
}

func (m model) landingGradientColor(row int) lipgloss.Color {
	return colorLandingPanel
}
