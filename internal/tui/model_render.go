package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
)

func (m *model) refreshViewport() {
	m.syncViewportSize()
	m.syncTokenUsageBounds()
	chatOffset := m.viewport.YOffset
	keepChatBottom := m.chatAutoFollow || m.viewport.AtBottom()
	conversationContent := m.renderConversation()
	m.viewportContentCache = conversationContent
	m.viewport.SetContent(conversationContent)
	m.copyView.SetContent(m.renderConversationCopy())
	if keepChatBottom {
		m.viewport.GotoBottom()
		m.copyView.GotoBottom()
		m.chatAutoFollow = true
	} else {
		m.viewport.SetYOffset(chatOffset)
		m.copyView.SetYOffset(chatOffset)
	}
	m.syncCopyViewOffset()

	planOffset := m.planView.YOffset
	m.planView.SetContent(m.planPanelContent(max(16, m.planView.Width)))
	m.planView.SetYOffset(planOffset)
}

func (m *model) syncTokenUsageBounds() {
	if m.screen != screenChat || m.width <= 0 || m.height <= 0 {
		m.tokenUsage.SetBounds(0, 0, 0, 0)
		return
	}
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTokenBadge(width))
	if badge == "" {
		m.tokenUsage.SetBounds(0, 0, 0, 0)
		return
	}
	badgeW := lipgloss.Width(badge)
	badgeH := lipgloss.Height(badge)
	x := panelStyle.GetHorizontalFrameSize()/2 + max(0, width-badgeW-1)
	y := panelStyle.GetVerticalFrameSize() / 2
	m.tokenUsage.SetBounds(x, y, badgeW, badgeH)
}

func (m *model) syncLayoutForCurrentScreen() {
	if m.width > 0 {
		if m.screen == screenLanding {
			m.input.SetWidth(m.landingInputContentWidth())
		} else {
			m.input.SetWidth(m.chatInputContentWidth())
		}
	}
	m.syncInputStyle()
	m.syncViewportSize()
}

func (m *model) resize() {
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}

func (m model) View() string {
	ensureZoneManager()
	if m.width > 0 {
		if m.screen == screenLanding {
			m.input.SetWidth(m.landingInputContentWidth())
			m.syncInputStyle()
		} else {
			m.input.SetWidth(m.chatInputContentWidth())
			m.syncInputStyle()
		}
	}
	base := m.renderLanding()
	if m.screen == screenChat {
		chatContent := lipgloss.JoinVertical(lipgloss.Left, m.renderMainPanel(), m.renderFooter())
		base = panelStyle.Width(m.chatPanelWidth()).Render(chatContent)
	}

	rendered := base
	switch {
	case m.helpOpen:
		rendered = renderModal(m.width, m.height, m.renderHelpModal())
	case m.sessionsOpen:
		rendered = renderModal(m.width, m.height, m.renderSessionsModal())
	}
	return zone.Scan(rendered)
}

func (m *model) SetUsage(used, total int) tea.Cmd {
	m.tokenHasOfficialUsage = true
	m.tokenUsage.SetUnavailable(false)
	return m.tokenUsage.SetUsage(used, 0)
}

func (m model) renderConversation() string {
	if len(m.chatItems) == 0 {
		return mutedStyle.Render("No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\".")
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatRow(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}
		blocks = append(blocks, renderBytemindRunRow(m.chatItems[i:j], width))
		i = j
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

func (m model) renderConversationCopy() string {
	if len(m.chatItems) == 0 {
		return "No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\"."
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatCopySection(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}

		runParts := make([]string, 0, j-i)
		for _, runItem := range m.chatItems[i:j] {
			runParts = append(runParts, renderChatCopySection(runItem, width))
		}
		blocks = append(blocks, strings.Join(runParts, "\n\n"))
		i = j
	}
	return strings.Join(blocks, "\n\n")
}

func renderChatCopySection(item chatEntry, width int) string {
	title := strings.TrimSpace(item.Title)
	status := strings.TrimSpace(item.Status)
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "assistant":
		if strings.EqualFold(item.Status, "thinking") {
			title = "thinking"
			status = ""
		}
	case "user":
		if strings.TrimSpace(item.Meta) != "" {
			title = strings.TrimSpace(item.Meta)
		}
	}

	if title == "" {
		switch item.Kind {
		case "assistant":
			title = assistantLabel
		case "user":
			title = "You"
		case "tool":
			title = "Tool"
		default:
			title = "Message"
		}
	}
	if status != "" {
		title += "  " + status
	}

	body := strings.TrimRight(formatChatBody(item, width), "\n")
	if item.Kind == "tool" && strings.TrimSpace(body) == "" {
		return title
	}
	if strings.TrimSpace(body) == "" {
		return title
	}
	return title + "\n" + body
}

func (m *model) syncViewportSize() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerHeight := lipgloss.Height(m.renderFooter())
	bodyHeight := m.height - footerHeight
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	statusHeight := lipgloss.Height(m.renderStatusBar())
	panelInnerHeight := max(4, bodyHeight-panelStyle.GetVerticalFrameSize()-statusHeight-1)
	m.planView.Width = 0
	m.planView.Height = 0
	contentHeight := max(3, panelInnerHeight)
	m.viewport.Width = max(8, m.conversationPanelWidth()-scrollbarWidth)
	m.viewport.Height = contentHeight
	m.copyView.Width = m.viewport.Width
	m.copyView.Height = m.viewport.Height
	m.syncCopyViewOffset()
}

func (m *model) syncCopyViewOffset() {
	if m == nil {
		return
	}
	m.copyView.SetYOffset(m.viewport.YOffset)
}

func (m model) renderMainPanel() string {
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTopRightCluster(width))
	conversation := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderConversationViewport(),
		m.renderScrollbar(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset),
	)
	if badge == "" {
		return lipgloss.JoinVertical(lipgloss.Left, m.renderStatusBar(), "", conversation)
	}

	badgeW := lipgloss.Width(badge)
	statusW := max(12, width-badgeW-2)
	status := m.renderStatusBarWithWidth(statusW)
	header := lipgloss.JoinHorizontal(lipgloss.Top, status, "  ", badge)

	parts := []string{header}
	if popup := strings.TrimSpace(m.tokenUsage.PopupView()); popup != "" {
		parts = append(parts, lipgloss.PlaceHorizontal(width, lipgloss.Right, popup))
	}
	parts = append(parts, "", conversation)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderTopRightCluster(width int) string {
	parts := make([]string, 0, 2)
	if toast := strings.TrimSpace(m.selectionToast); toast != "" {
		parts = append(parts, selectionToastStyle.Render(toast))
	}
	if badge := strings.TrimSpace(m.renderTokenBadge(width)); badge != "" {
		parts = append(parts, badge)
	}
	return strings.Join(parts, "  ")
}

func (m model) renderTokenBadge(width int) string {
	return m.tokenUsage.View()
}

func (m model) renderScrollbar(viewHeight, contentHeight, currentOffset int) string {
	thumbTop, thumbHeight, _, visible := m.scrollbarLayout(viewHeight, contentHeight, currentOffset)
	if !visible {
		return ""
	}
	trackStyle := scrollbarTrackStyle.Copy().Background(lipgloss.Color("#1B1D22"))
	thumbStyle := scrollbarThumbIdleStyle.Copy().Background(lipgloss.Color("#C2C7CF"))
	if m.draggingScrollbar {
		thumbStyle = scrollbarThumbActiveStyle.Copy().Background(lipgloss.Color("#E5E7EB"))
	}
	lines := make([]string, 0, viewHeight)
	for row := 0; row < viewHeight; row++ {
		if row >= thumbTop && row < thumbTop+thumbHeight {
			lines = append(lines, thumbStyle.Render(" "))
			continue
		}
		lines = append(lines, trackStyle.Render(" "))
	}
	return strings.Join(lines, "\n")
}

func (m model) scrollbarLayout(viewHeight, contentHeight, currentOffset int) (thumbTop, thumbHeight, maxOffset int, visible bool) {
	if viewHeight <= 0 {
		return 0, 0, 0, false
	}
	if contentHeight <= 0 {
		contentHeight = viewHeight
	}
	maxOffset = max(0, contentHeight-viewHeight)
	if maxOffset == 0 {
		return 0, viewHeight, 0, true
	}

	thumbHeight = roundedScaledDivision(viewHeight, viewHeight, contentHeight)
	thumbHeight = clamp(thumbHeight, 1, viewHeight)

	trackRange := max(0, viewHeight-thumbHeight)
	if trackRange == 0 {
		return 0, thumbHeight, maxOffset, true
	}
	offset := clamp(currentOffset, 0, maxOffset)
	thumbTop = roundedScaledDivision(offset, trackRange, maxOffset)
	thumbTop = clamp(thumbTop, 0, trackRange)
	return thumbTop, thumbHeight, maxOffset, true
}

func (m model) scrollbarTrackBounds() (x, top, bottom int, ok bool) {
	if m.screen != screenChat || m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, false
	}
	left, right, top, bottom, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		return 0, 0, 0, false
	}
	return right + 1, top, bottom, left >= 0
}

func (m *model) dragScrollbarTo(mouseY int) {
	_, trackTop, _, ok := m.scrollbarTrackBounds()
	if !ok {
		return
	}
	_, thumbHeight, maxOffset, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
	if !visible || maxOffset == 0 {
		return
	}
	trackRange := max(0, m.viewport.Height-thumbHeight)
	if trackRange == 0 {
		m.viewport.SetYOffset(0)
		m.syncCopyViewOffset()
		return
	}
	desiredTop := mouseY - trackTop - m.scrollbarDragOffset
	desiredTop = clamp(desiredTop, 0, trackRange)
	offset := roundedScaledDivision(desiredTop, maxOffset, trackRange)
	m.viewport.SetYOffset(clamp(offset, 0, maxOffset))
	m.syncCopyViewOffset()
}

func stripANSIText(value string) string {
	return xansi.Strip(value)
}

func (m model) renderLanding() string {
	ensureZoneManager()
	logo := landingLogoStyle.Render(strings.Join([]string{
		"    ____        __                      _           __",
		"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
		"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
		" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
		"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
		"      /____/                                          ",
	}, "\n"))
	inputBox := landingInputStyle.Copy().
		BorderForeground(m.modeAccentColor()).
		Width(m.landingInputShellWidth()).
		Render(zone.Mark(inputEditorZoneID, m.renderInputEditorView()))
	parts := []string{logo, "", m.renderModeTabs(), ""}
	if m.startupGuide.Active {
		parts = append(parts, m.renderStartupGuidePanel(), "")
	} else if m.promptSearchOpen {
		parts = append(parts, m.renderPromptSearchPalette(), "")
	} else if m.mentionOpen {
		parts = append(parts, m.renderMentionPalette(), "")
	} else if m.commandOpen {
		parts = append(parts, m.renderCommandPalette(), "")
	}
	parts = append(parts, inputBox, "", renderFooterShortcutHints())
	content := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderFooter() string {
	ensureZoneManager()
	inputBorder := m.inputBorderStyle().
		Width(m.chatPanelInnerWidth()).
		Render(zone.Mark(inputEditorZoneID, m.renderInputEditorView()))
	parts := make([]string, 0, 4)
	if m.approval != nil {
		parts = append(parts, m.renderApprovalBanner())
	}
	if m.startupGuide.Active {
		parts = append(parts, m.renderStartupGuidePanel())
	} else if m.promptSearchOpen {
		parts = append(parts, m.renderPromptSearchPalette())
	} else if m.mentionOpen {
		parts = append(parts, m.renderMentionPalette())
	} else if m.commandOpen {
		parts = append(parts, m.renderCommandPalette())
	}
	if banner := m.renderActiveSkillBanner(); banner != "" {
		parts = append(parts, banner)
	}
	parts = append(parts, inputBorder, m.renderFooterInfoLine())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderModeTabs() string {
	buildStyle := modeTabStyle.Copy().Foreground(colorMuted)
	planStyle := modeTabStyle.Copy().Foreground(colorMuted)
	if m.mode == modeBuild {
		buildStyle = buildStyle.Copy().Foreground(colorAccent).Bold(true)
	} else {
		planStyle = planStyle.Copy().Foreground(colorThinking).Bold(true)
	}
	parts := []string{
		buildStyle.Render("Build"),
		planStyle.Render("Plan"),
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m model) renderFooterInfoLine() string {
	width := max(24, m.chatPanelInnerWidth())
	left := m.renderModeTabs()
	modelName := strings.TrimSpace(m.currentModelLabel())
	if modelName == "-" {
		modelName = ""
	}
	right := renderFooterInfoRight(modelName, 1<<30)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 2 {
		available := max(10, width-leftW-2)
		if available <= 10 {
			return lipgloss.NewStyle().Width(width).Render(renderFooterInfoRight(modelName, width))
		}
		compacted := renderFooterInfoRight(modelName, available)
		gap = width - leftW - lipgloss.Width(compacted)
		return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", max(2, gap)) + compacted)
	}

	return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", gap) + right)
}

func renderFooterInfoRight(modelName string, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return renderInlineShortcutHintsCompacted(footerShortcutHints, maxWidth)
	}
	modelText := compact(modelName, maxWidth)
	modelWidth := runewidth.StringWidth(modelText)
	if modelWidth >= maxWidth {
		return mutedStyle.Render(modelText)
	}
	dividerPlain := "  |  "
	dividerWidth := runewidth.StringWidth(dividerPlain)
	remaining := maxWidth - modelWidth - dividerWidth
	if remaining <= 0 {
		return mutedStyle.Render(modelText)
	}
	hints := renderInlineShortcutHintsCompacted(footerShortcutHints, remaining)
	if strings.TrimSpace(hints) == "" {
		return mutedStyle.Render(modelText)
	}
	return mutedStyle.Render(modelText) + footerHintDividerStyle.Render(dividerPlain) + hints
}

func renderFooterShortcutHints() string {
	return renderInlineShortcutHints(footerShortcutHints)
}

func renderInlineShortcutHints(hints []footerShortcutHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, footerHintKeyStyle.Render(hint.Key)+" "+footerHintLabelStyle.Render(hint.Label))
	}
	return strings.Join(parts, footerHintDividerStyle.Render("  |  "))
}

func renderInlineShortcutHintsCompacted(hints []footerShortcutHint, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	dividerPlain := "  |  "
	dividerWidth := runewidth.StringWidth(dividerPlain)

	used := 0
	parts := make([]string, 0, len(hints)*2)
	for _, hint := range hints {
		key := strings.TrimSpace(hint.Key)
		label := strings.TrimSpace(hint.Label)
		if key == "" {
			continue
		}
		segmentPlain := key
		segmentStyled := footerHintKeyStyle.Render(key)
		if label != "" {
			segmentPlain += " " + label
			segmentStyled += " " + footerHintLabelStyle.Render(label)
		}
		needDivider := len(parts) > 0
		prefixWidth := 0
		if needDivider {
			prefixWidth = dividerWidth
		}
		segmentWidth := runewidth.StringWidth(segmentPlain)
		if used+prefixWidth+segmentWidth <= maxWidth {
			if needDivider {
				parts = append(parts, footerHintDividerStyle.Render(dividerPlain))
				used += dividerWidth
			}
			parts = append(parts, segmentStyled)
			used += segmentWidth
			continue
		}

		remaining := maxWidth - used - prefixWidth
		if remaining <= 0 {
			break
		}
		if needDivider {
			parts = append(parts, footerHintDividerStyle.Render(dividerPlain))
			used += dividerWidth
		}

		keyWidth := runewidth.StringWidth(key)
		if keyWidth >= remaining {
			parts = append(parts, footerHintKeyStyle.Render(compact(key, remaining)))
			break
		}
		if label == "" {
			parts = append(parts, footerHintKeyStyle.Render(key))
			break
		}
		labelSpace := remaining - keyWidth - 1
		if labelSpace <= 0 {
			parts = append(parts, footerHintKeyStyle.Render(key))
			break
		}
		parts = append(parts, footerHintKeyStyle.Render(key)+" "+footerHintLabelStyle.Render(compact(label, labelSpace)))
		break
	}
	return strings.Join(parts, "")
}

func (m model) renderSessionsModal() string {
	lines := []string{modalTitleStyle.Render("Recent Sessions"), mutedStyle.Render("Up/Down to select, Enter to resume, Esc to close"), ""}
	if len(m.sessions) == 0 {
		lines = append(lines, "No sessions available.")
	} else {
		for i, summary := range m.sessions {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.sessionCursor {
				prefix = "> "
				style = style.Foreground(colorAccent).Bold(true)
			}
			line := fmt.Sprintf("%s%s  %s  %d msgs", prefix, shortID(summary.ID), summary.UpdatedAt.Local().Format("2006-01-02 15:04"), summary.MessageCount)
			lines = append(lines, style.Render(line))
			lines = append(lines, mutedStyle.Render("   "+summary.Workspace))
			if summary.LastUserMessage != "" {
				lines = append(lines, mutedStyle.Render("   "+summary.LastUserMessage))
			}
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
}

func (m model) renderHelpModal() string {
	return modalBoxStyle.Width(min(88, max(54, m.width-16))).Render(
		lipgloss.JoinVertical(lipgloss.Left, modalTitleStyle.Render("Help"), m.helpText()),
	)
}

func (m model) renderApprovalBanner() string {
	width := max(24, m.chatPanelInnerWidth())
	commandWidth := max(20, width-4)
	lines := []string{
		accentStyle.Render("Approval required"),
		mutedStyle.Render("Reason: " + trimPreview(m.approval.Reason, commandWidth)),
		codeStyle.Width(commandWidth).Render(m.approval.Command),
		mutedStyle.Render("Y / Enter approve    N / Esc reject"),
	}
	return approvalBannerStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderActiveSkillBanner() string {
	if m.sess == nil || m.sess.ActiveSkill == nil {
		return ""
	}
	name := strings.TrimSpace(m.sess.ActiveSkill.Name)
	if name == "" {
		return ""
	}

	line := "Active skill: " + name
	if len(m.sess.ActiveSkill.Args) > 0 {
		keys := make([]string, 0, len(m.sess.ActiveSkill.Args))
		for key := range m.sess.ActiveSkill.Args {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, m.sess.ActiveSkill.Args[key]))
		}
		line += " | args: " + strings.Join(pairs, ", ")
	}

	width := max(24, m.chatPanelInnerWidth())
	return activeSkillBannerStyle.Width(width).Render(accentStyle.Render(line))
}

func (m model) renderStatusBar() string {
	return m.renderStatusBarWithWidth(max(24, m.chatPanelInnerWidth()))
}

func (m model) renderStatusBarWithWidth(width int) string {
	stepTitle := currentOrNextStepTitle(m.plan)
	if stepTitle == "" {
		stepTitle = "-"
	}
	left := strings.Join([]string{
		"Mode: " + strings.ToUpper(string(m.mode)),
		"Phase: " + m.currentPhaseLabel(),
		"Step: " + stepTitle,
		"Skill: " + m.currentSkillLabel(),
	}, "  |  ")
	right := strings.Join([]string{
		fmt.Sprintf("%d msgs", len(m.chatItems)),
		"Session: " + m.currentSessionLabel(),
		"Follow: " + m.autoFollowLabel(),
		"Model: " + m.currentModelLabel(),
	}, "  |  ")

	line := m.renderTopInfoLine(left, right, width)
	return statusBarStyle.Width(width).Render(line)
}

func (m model) renderTopInfoLine(left, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if width <= 0 {
		return strings.TrimSpace(left + " | " + right)
	}

	leftW := runewidth.StringWidth(left)
	rightW := runewidth.StringWidth(right)
	if leftW+rightW+2 > width {
		return compact(left+"  |  "+right, width)
	}
	gap := width - leftW - rightW
	return left + strings.Repeat(" ", max(2, gap)) + right
}
