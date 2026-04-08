package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

func (m model) conversationViewportBounds() (left, right, top, bottom int, ok bool) {
	if m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	return m.conversationViewportBoundsByLayout()
}

func (m model) conversationViewportTopFromRenderedView(left, expectedTop int) (int, bool) {
	if m.viewport.Height <= 0 {
		return 0, false
	}
	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	viewportLines := strings.Split(strings.ReplaceAll(m.renderConversationViewport(), "\r\n", "\n"), "\n")
	if len(fullLines) == 0 || len(viewportLines) == 0 || len(fullLines) < len(viewportLines) {
		return 0, false
	}
	maxOrigin := len(fullLines) - len(viewportLines)
	if maxOrigin < 0 {
		return 0, false
	}

	bestOrigin := -1
	bestScore := -1
	bestDistance := 0
	fullMatchOrigin := -1
	fullMatchDistance := 0

	for candidate := 0; candidate <= maxOrigin; candidate++ {
		score := 0
		for row := 0; row < len(viewportLines); row++ {
			if m.screenLineMatchesViewportLine(fullLines[candidate+row], left, viewportLines[row]) {
				score++
			}
		}
		distance := intAbs(candidate - expectedTop)
		if score == len(viewportLines) {
			if fullMatchOrigin < 0 || distance < fullMatchDistance {
				fullMatchOrigin = candidate
				fullMatchDistance = distance
			}
			continue
		}
		if score > bestScore || (score == bestScore && (bestOrigin < 0 || distance < bestDistance)) {
			bestOrigin = candidate
			bestScore = score
			bestDistance = distance
		}
	}

	if fullMatchOrigin >= 0 {
		return fullMatchOrigin, true
	}
	requiredScore := max(1, (len(viewportLines)*8)/10)
	if bestOrigin < 0 || bestScore < requiredScore {
		return 0, false
	}
	return bestOrigin, true
}

func (m model) screenLineMatchesViewportLine(screenLine string, left int, viewportLine string) bool {
	segment := xansi.Cut(screenLine, left, left+m.viewport.Width)
	screenText := strings.TrimRight(xansi.Strip(segment), " ")
	viewportText := strings.TrimRight(xansi.Strip(viewportLine), " ")
	return screenText == viewportText
}

func intAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (m model) conversationViewportBoundsByLayout() (left, right, top, bottom int, ok bool) {
	if m.screen != screenChat || m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	panelTop := panelStyle.GetVerticalFrameSize() / 2
	panelLeft := panelStyle.GetHorizontalFrameSize() / 2
	viewportTop := panelTop + m.conversationViewportOffsetInMainPanel()
	viewportBottom := viewportTop + m.viewport.Height - 1
	viewportLeft := panelLeft
	viewportRight := viewportLeft + m.viewport.Width - 1
	return viewportLeft, viewportRight, viewportTop, viewportBottom, true
}

func (m model) conversationViewportOffsetInMainPanel() int {
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTopRightCluster(width))
	if badge == "" {
		return lipgloss.Height(m.renderStatusBar()) + 1
	}
	badgeW := lipgloss.Width(badge)
	statusW := max(12, width-badgeW-2)
	status := m.renderStatusBarWithWidth(statusW)
	header := lipgloss.JoinHorizontal(lipgloss.Top, status, "  ", badge)
	offset := lipgloss.Height(header)
	if popup := strings.TrimSpace(m.tokenUsage.PopupView()); popup != "" {
		offset += lipgloss.Height(lipgloss.PlaceHorizontal(width, lipgloss.Right, popup))
	}
	return offset + 1
}
