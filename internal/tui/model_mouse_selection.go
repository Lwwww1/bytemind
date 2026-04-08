package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ensureZoneManager()
	msg = m.normalizeMouseMsg(msg)

	if m.inputMouseSelecting {
		switch msg.Action {
		case tea.MouseActionMotion:
			if point, ok := m.inputPointFromMouse(msg.X, msg.Y, true); ok {
				m.inputSelectionEnd = point
			} else {
				m.clearInputSelection()
			}
			return m, nil
		case tea.MouseActionRelease:
			if point, ok := m.inputPointFromMouse(msg.X, msg.Y, true); ok && selectionHasRange(m.inputSelectionStart, point) {
				m.inputSelectionEnd = point
				m.inputSelectionActive = true
				m.statusNote = "Selection ready. Press Ctrl+C to copy."
			} else {
				m.clearInputSelection()
			}
			m.inputMouseSelecting = false
			return m, nil
		}
	}

	if m.mouseSelecting {
		m.mouseSelectionMouseX = msg.X
		m.mouseSelectionMouseY = msg.Y
		switch msg.Action {
		case tea.MouseActionMotion:
			if point, ok := m.viewportPointFromMouseWithAutoScroll(msg.X, msg.Y); ok {
				m.mouseSelectionEnd = point
			} else {
				m.clearMouseSelection()
			}
			return m, nil
		case tea.MouseActionRelease:
			if point, ok := m.viewportPointFromMouseWithAutoScroll(msg.X, msg.Y); ok && selectionHasRange(m.mouseSelectionStart, point) {
				m.mouseSelectionEnd = point
				m.mouseSelectionActive = true
				m.statusNote = "Selection ready. Press Ctrl+C to copy."
			} else {
				m.clearMouseSelection()
			}
			m.mouseSelecting = false
			m.stopMouseSelectionScrollTicker()
			m.draggingScrollbar = false
			return m, nil
		}
	}

	if msg.Action == tea.MouseActionRelease {
		m.draggingScrollbar = false
	}
	if m.helpOpen || m.commandOpen || m.mentionOpen || m.promptSearchOpen || m.approval != nil {
		return m, nil
	}
	if m.screen != screenChat && m.screen != screenLanding {
		return m, nil
	}
	if m.screen == screenChat && m.sessionsOpen {
		return m, nil
	}
	if cmd, consumed := m.tokenUsage.Update(msg); consumed {
		return m, cmd
	}
	if m.mouseOverInput(msg.Y) {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if m.mouseSelecting || m.mouseSelectionActive {
				m.clearMouseSelection()
			}
			if point, ok := m.inputPointFromMouse(msg.X, msg.Y, false); ok {
				m.inputMouseSelecting = true
				m.inputSelectionActive = false
				m.inputSelectionStart = point
				m.inputSelectionEnd = point
				return m, nil
			}
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollInput(-scrollStep)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.scrollInput(scrollStep)
			return m, nil
		default:
			return m, nil
		}
	}
	if m.screen == screenChat {
		if msg.Action == tea.MouseActionMotion && m.draggingScrollbar {
			m.dragScrollbarTo(msg.Y)
			m.chatAutoFollow = false
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			trackX, trackTop, trackBottom, ok := m.scrollbarTrackBounds()
			if ok && msg.X == trackX && msg.Y >= trackTop && msg.Y <= trackBottom {
				thumbTop, thumbHeight, _, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
				if visible && thumbHeight > 0 {
					absoluteThumbTop := trackTop + thumbTop
					absoluteThumbBottom := absoluteThumbTop + thumbHeight - 1
					if msg.Y >= absoluteThumbTop && msg.Y <= absoluteThumbBottom {
						m.scrollbarDragOffset = msg.Y - absoluteThumbTop
					} else {
						// Click on track should jump close to that point, then start drag.
						m.scrollbarDragOffset = thumbHeight / 2
						m.dragScrollbarTo(msg.Y)
					}
					m.draggingScrollbar = true
					m.chatAutoFollow = false
					return m, nil
				}
			}
			if m.mouseSelectionActive {
				m.clearMouseSelection()
			}
			if m.inputSelectionActive || m.inputMouseSelecting {
				m.clearInputSelection()
			}
			if point, ok := m.viewportPointFromMouse(msg.X, msg.Y); ok {
				m.mouseSelecting = true
				m.mouseSelectionActive = false
				m.mouseSelectionMouseX = msg.X
				m.mouseSelectionMouseY = msg.Y
				m.mouseSelectionStart = point
				m.mouseSelectionEnd = point
				return m, m.startMouseSelectionScrollTicker()
			}
		}
	}
	if m.screen == screenChat {
		if m.mouseOverPlan(msg.X, msg.Y) {
			m.ensurePlanMouse()
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.planView.LineUp(scrollStep)
				return m, nil
			case tea.MouseButtonWheelDown:
				m.planView.LineDown(scrollStep)
				return m, nil
			default:
				var cmd tea.Cmd
				m.planView, cmd = m.planView.Update(msg)
				return m, cmd
			}
		}
		m.ensureViewportMouse()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(scrollStep)
			m.syncCopyViewOffset()
			m.chatAutoFollow = false
			return m, nil
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(scrollStep)
			m.syncCopyViewOffset()
			m.chatAutoFollow = m.viewport.AtBottom()
			return m, nil
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.syncCopyViewOffset()
			m.chatAutoFollow = m.viewport.AtBottom()
			return m, cmd
		}
	}
	return m, nil
}

func (m model) handleMouseSelectionScrollTick(msg mouseSelectionScrollTickMsg) (tea.Model, tea.Cmd) {
	if !m.mouseSelecting || msg.ID != m.mouseSelectionTickID {
		return m, nil
	}
	cmd := mouseSelectionScrollTickCmd(msg.ID)
	if !selectionHasRange(m.mouseSelectionStart, m.mouseSelectionEnd) {
		return m, cmd
	}
	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return m, cmd
	}
	if m.mouseSelectionMouseX < left-1 || m.mouseSelectionMouseX > right+1 {
		return m, cmd
	}

	targetY := 0
	switch {
	case m.mouseSelectionMouseY >= bottom:
		targetY = bottom + 1
	case m.mouseSelectionMouseY <= top:
		targetY = top - 1
	default:
		return m, cmd
	}
	if point, ok := m.viewportPointFromMouseWithAutoScroll(m.mouseSelectionMouseX, targetY); ok {
		m.mouseSelectionEnd = point
	}
	return m, cmd
}

func (m *model) startMouseSelectionScrollTicker() tea.Cmd {
	if m == nil {
		return nil
	}
	m.mouseSelectionTickID++
	return mouseSelectionScrollTickCmd(m.mouseSelectionTickID)
}

func (m *model) stopMouseSelectionScrollTicker() {
	if m == nil {
		return
	}
	m.mouseSelectionTickID++
}

func mouseSelectionScrollTickCmd(id int) tea.Cmd {
	return tea.Tick(mouseSelectionScrollTick, func(time.Time) tea.Msg {
		return mouseSelectionScrollTickMsg{ID: id}
	})
}

func (m *model) viewportPointFromMouseWithAutoScroll(x, y int) (viewportSelectionPoint, bool) {
	if m == nil {
		return viewportSelectionPoint{}, false
	}
	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if x < left-1 || x > right+1 {
		return viewportSelectionPoint{}, false
	}

	edgeX := clamp(x, left, right)
	switch {
	case y > bottom:
		steps := y - bottom
		for i := 0; i < steps; i++ {
			m.viewport.LineDown(1)
		}
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m.viewportPointFromMouse(edgeX, bottom)
	case y < top:
		steps := top - y
		for i := 0; i < steps; i++ {
			m.viewport.LineUp(1)
		}
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m.viewportPointFromMouse(edgeX, top)
	default:
		return m.viewportPointFromMouse(x, y)
	}
}

func (m *model) copyCurrentSelection() tea.Cmd {
	if m == nil {
		return nil
	}
	selected := m.inputSelectionText()
	if strings.TrimSpace(selected) == "" {
		selected = m.viewportSelectionText()
	}
	if strings.TrimSpace(selected) != "" {
		if m.clipboardText == nil {
			m.statusNote = "clipboard copy is unavailable in current environment"
		} else if err := m.clipboardText.WriteText(context.Background(), selected); err != nil {
			m.statusNote = err.Error()
		} else {
			m.statusNote = "Copied selection to clipboard."
			m.selectionToast = "Copied selection"
			m.selectionToastID++
			id := m.selectionToastID
			m.clearMouseSelection()
			m.clearInputSelection()
			return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
				return selectionToastExpiredMsg{ID: id}
			})
		}
		return nil
	}
	m.statusNote = "Selection is empty."
	m.clearMouseSelection()
	m.clearInputSelection()
	return nil
}

func (m model) normalizeMouseMsg(msg tea.MouseMsg) tea.MouseMsg {
	if m.mouseYOffset == 0 {
		return msg
	}
	if m.screen == screenLanding {
		// Landing input uses zone-based hit testing; applying an extra global
		// y-offset here introduces row drift in some Windows terminals.
		return msg
	}
	msg.Y += m.mouseYOffset
	return msg
}

func (m *model) clearMouseSelection() {
	if m == nil {
		return
	}
	m.stopMouseSelectionScrollTicker()
	m.mouseSelecting = false
	m.mouseSelectionActive = false
	m.mouseSelectionMouseX = 0
	m.mouseSelectionMouseY = 0
	m.mouseSelectionStart = viewportSelectionPoint{}
	m.mouseSelectionEnd = viewportSelectionPoint{}
}

func (m *model) clearInputSelection() {
	if m == nil {
		return
	}
	m.inputMouseSelecting = false
	m.inputSelectionActive = false
	m.inputSelectionStart = viewportSelectionPoint{}
	m.inputSelectionEnd = viewportSelectionPoint{}
}

func (m model) hasCopyableSelection() bool {
	return m.hasCopyableInputSelection() || m.hasCopyableViewportSelection()
}

func (m model) hasCopyableInputSelection() bool {
	return (m.inputMouseSelecting || m.inputSelectionActive) && selectionHasRange(m.inputSelectionStart, m.inputSelectionEnd)
}

func (m model) hasCopyableViewportSelection() bool {
	return (m.mouseSelecting || m.mouseSelectionActive) && selectionHasRange(m.mouseSelectionStart, m.mouseSelectionEnd)
}

func (m model) renderConversationViewport() string {
	content := ""
	if m.hasCopyableViewportSelection() {
		if preview := m.renderActiveSelectionPreview(); strings.TrimSpace(preview) != "" {
			content = preview
		}
	}
	if content == "" {
		content = m.viewport.View()
	}
	return zone.Mark(conversationViewportZoneID, content)
}

func (m model) renderInputEditorView() string {
	raw := m.input.View()
	if !m.hasCopyableInputSelection() {
		return raw
	}
	if preview := m.renderInputSelectionPreview(raw); preview != "" {
		return preview
	}
	return raw
}

func (m model) renderInputSelectionPreview(raw string) string {
	lines := m.inputSelectionSourceLines(raw)
	if len(lines) == 0 {
		return raw
	}
	start, end := normalizeViewportSelectionPoints(m.inputSelectionStart, m.inputSelectionEnd)
	maxRow := len(lines) - 1
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)
	rendered := make([]string, 0, len(lines))
	for row, line := range lines {
		lineWidth := max(1, xansi.StringWidth(line))
		rangeStart, rangeEnd, ok := selectionColumnsForRow(row, lineWidth, start, end, false)
		if !ok {
			rendered = append(rendered, line)
			continue
		}
		rendered = append(rendered, highlightVisibleLineByCells(line, rangeStart, rangeEnd))
	}
	return strings.Join(rendered, "\n")
}

func (m model) renderActiveSelectionPreview() string {
	view := strings.ReplaceAll(m.viewport.View(), "\r\n", "\n")
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return ""
	}

	start, end := normalizeViewportSelectionPoints(m.mouseSelectionStart, m.mouseSelectionEnd)
	sourceLines := m.selectionSourceLines()
	maxRow := len(sourceLines) - 1
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)

	rendered := make([]string, 0, len(lines))
	visibleStartRow := max(0, m.viewport.YOffset)
	for row, raw := range lines {
		lineWidth := max(xansi.StringWidth(raw), m.viewport.Width)
		absRow := visibleStartRow + row
		rangeStart, rangeEnd, ok := selectionColumnsForRow(absRow, lineWidth, start, end, false)
		if !ok {
			rendered = append(rendered, raw)
			continue
		}
		rendered = append(rendered, highlightVisibleLineByCells(raw, rangeStart, rangeEnd))
	}
	return strings.Join(rendered, "\n")
}

func (m model) viewportPointFromMouse(x, y int) (viewportSelectionPoint, bool) {
	ensureZoneManager()
	if z := zone.Get(conversationViewportZoneID); z != nil {
		if point, ok := m.viewportPointFromZone(z, x, y); ok {
			return point, true
		}
		// Keep zone-first behavior robust for terminals that occasionally
		// report mouse rows with small absolute drift near viewport edges.
		if x >= z.StartX-1 && x <= z.EndX+1 {
			for delta := 1; delta <= mouseZoneAutoProbeMaxDelta; delta++ {
				if point, ok := m.viewportPointFromZone(z, x, y-delta); ok {
					return point, true
				}
				if point, ok := m.viewportPointFromZone(z, x, y+delta); ok {
					return point, true
				}
			}
		}
	}

	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if renderedTop, found := m.conversationViewportTopFromRenderedView(left, top); found {
		top = renderedTop
		bottom = top + m.viewport.Height - 1
	}
	// Keep drag-select usable for terminals that report 0-based or 1-based mouse coords.
	if x < left-1 || x > right+1 || y < top-1 || y > bottom+1 {
		return viewportSelectionPoint{}, false
	}
	col := clamp(x-left, 0, max(0, m.viewport.Width-1))
	row := clamp(y-top, 0, max(0, m.viewport.Height-1))
	return viewportSelectionPoint{
		Col: col,
		Row: max(0, m.viewport.YOffset) + row,
	}, true
}

func (m model) viewportPointFromZone(z *zone.ZoneInfo, x, y int) (viewportSelectionPoint, bool) {
	col, row := z.Pos(tea.MouseMsg{X: x, Y: y})
	if col < 0 || row < 0 {
		return viewportSelectionPoint{}, false
	}
	return viewportSelectionPoint{
		Col: clamp(col, 0, max(0, m.viewport.Width-1)),
		Row: max(0, m.viewport.YOffset) + clamp(row, 0, max(0, m.viewport.Height-1)),
	}, true
}

func (m model) viewportSelectionText() string {
	start, end := normalizeViewportSelectionPoints(m.mouseSelectionStart, m.mouseSelectionEnd)
	if start.Row == end.Row && start.Col == end.Col {
		return ""
	}

	lines := m.selectionSourceLines()
	if len(lines) == 0 {
		return ""
	}
	maxRow := len(lines) - 1
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)
	if start.Row > end.Row {
		start, end = end, start
	}

	parts := make([]string, 0, end.Row-start.Row+1)
	for row := start.Row; row <= end.Row; row++ {
		raw := lines[row]
		lineWidth := max(xansi.StringWidth(raw), m.viewport.Width)
		rangeStart, rangeEnd, ok := selectionColumnsForRow(row, lineWidth, start, end, false)
		if !ok {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, sliceViewportLineByCells(raw, rangeStart, rangeEnd))
	}
	return strings.Join(parts, "\n")
}

func (m model) inputSelectionText() string {
	if strings.TrimSpace(m.input.Value()) == "" {
		return ""
	}
	start, end := normalizeViewportSelectionPoints(m.inputSelectionStart, m.inputSelectionEnd)
	if start.Row == end.Row && start.Col == end.Col {
		return ""
	}
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return ""
	}
	maxRow := len(lines) - 1
	start.Row = clamp(start.Row, 0, maxRow)
	end.Row = clamp(end.Row, 0, maxRow)
	if start.Row > end.Row {
		start, end = end, start
	}

	parts := make([]string, 0, end.Row-start.Row+1)
	for row := start.Row; row <= end.Row; row++ {
		raw := lines[row]
		lineWidth := max(1, xansi.StringWidth(raw))
		rangeStart, rangeEnd, ok := selectionColumnsForRow(row, lineWidth, start, end, false)
		if !ok {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, sliceViewportLineByCells(raw, rangeStart, rangeEnd))
	}
	return strings.Join(parts, "\n")
}

func (m model) selectionSourceLines() []string {
	content := m.viewportContentCache
	if content == "" {
		content = m.viewport.View()
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(content, "\n")
}

func (m model) inputSelectionSourceLines(raw string) []string {
	if raw == "" {
		raw = m.input.View()
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	return strings.Split(raw, "\n")
}

func (m model) inputPointFromMouse(x, y int, clampToBounds bool) (viewportSelectionPoint, bool) {
	ensureZoneManager()
	if z := zone.Get(inputEditorZoneID); z != nil {
		if point, ok := m.inputPointFromZone(z, x, y, clampToBounds); ok {
			return point, true
		}
		// Keep input selection robust for terminals that occasionally
		// report mouse rows with small absolute drift.
		if x >= z.StartX-1 && x <= z.EndX+1 {
			for delta := 1; delta <= mouseZoneAutoProbeMaxDelta; delta++ {
				if point, ok := m.inputPointFromZone(z, x, y-delta, clampToBounds); ok {
					return point, true
				}
				if point, ok := m.inputPointFromZone(z, x, y+delta, clampToBounds); ok {
					return point, true
				}
			}
		}
	}

	left, right, top, bottom, innerLeft, innerTop, ok := m.inputInnerBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if !clampToBounds && (x < left || x > right || y < top || y > bottom) {
		return viewportSelectionPoint{}, false
	}
	x = clamp(x, left, right)
	y = clamp(y, top, bottom)
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return viewportSelectionPoint{}, false
	}
	row := clamp(y-innerTop, 0, len(lines)-1)
	lineWidth := xansi.StringWidth(lines[row])
	if lineWidth <= 0 {
		return viewportSelectionPoint{Row: row, Col: 0}, true
	}
	col := clamp(x-innerLeft, 0, lineWidth-1)
	return viewportSelectionPoint{Row: row, Col: col}, true
}

func (m model) inputPointFromZone(z *zone.ZoneInfo, x, y int, clampToBounds bool) (viewportSelectionPoint, bool) {
	col, row := z.Pos(tea.MouseMsg{X: x, Y: y})
	if (col < 0 || row < 0) && clampToBounds {
		clampedX := clamp(x, z.StartX, z.EndX)
		clampedY := clamp(y, z.StartY, z.EndY)
		col, row = z.Pos(tea.MouseMsg{X: clampedX, Y: clampedY})
	}
	if col < 0 || row < 0 {
		return viewportSelectionPoint{}, false
	}
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return viewportSelectionPoint{}, false
	}
	row = clamp(row, 0, len(lines)-1)
	lineWidth := xansi.StringWidth(lines[row])
	if lineWidth <= 0 {
		return viewportSelectionPoint{Row: row, Col: 0}, true
	}
	col = clamp(col, 0, lineWidth-1)
	return viewportSelectionPoint{Row: row, Col: col}, true
}

func (m model) inputInnerBounds() (left, right, top, bottom, innerLeft, innerTop int, ok bool) {
	switch m.screen {
	case screenChat:
		if m.width <= 0 {
			return 0, 0, 0, 0, 0, 0, false
		}
		inputRender := m.inputBorderStyle().Width(m.chatPanelInnerWidth()).Render(m.input.View())
		left = panelStyle.GetHorizontalFrameSize() / 2
		top = panelStyle.GetVerticalFrameSize()/2 + lipgloss.Height(m.renderMainPanel())
		if m.approval != nil {
			top += lipgloss.Height(m.renderApprovalBanner())
		}
		if m.startupGuide.Active {
			top += lipgloss.Height(m.renderStartupGuidePanel())
		} else if m.promptSearchOpen {
			top += lipgloss.Height(m.renderPromptSearchPalette())
		} else if m.mentionOpen {
			top += lipgloss.Height(m.renderMentionPalette())
		} else if m.commandOpen {
			top += lipgloss.Height(m.renderCommandPalette())
		}
		right = left + max(1, lipgloss.Width(inputRender)) - 1
		bottom = top + max(1, lipgloss.Height(inputRender)) - 1
		innerLeft = left + m.inputBorderStyle().GetHorizontalFrameSize()/2
		innerTop = top + m.inputBorderStyle().GetVerticalFrameSize()/2
		return left, right, top, bottom, innerLeft, innerTop, true
	case screenLanding:
		if m.height <= 0 || m.width <= 0 {
			return 0, 0, 0, 0, 0, 0, false
		}
		box := landingInputStyle.Copy().
			BorderForeground(m.modeAccentColor()).
			Width(m.landingInputShellWidth()).
			Render(m.input.View())
		boxW := max(1, lipgloss.Width(box))
		boxH := max(1, lipgloss.Height(box))
		logoHeight := lipgloss.Height(landingLogoStyle.Render(strings.Join([]string{
			"    ____        __                      _           __",
			"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
			"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
			" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
			"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
			"      /____/                                          ",
		}, "\n")))
		overlayHeight := 0
		if m.startupGuide.Active {
			overlayHeight = lipgloss.Height(m.renderStartupGuidePanel()) + 1
		} else if m.promptSearchOpen {
			overlayHeight = lipgloss.Height(m.renderPromptSearchPalette()) + 1
		} else if m.mentionOpen {
			overlayHeight = lipgloss.Height(m.renderMentionPalette()) + 1
		} else if m.commandOpen {
			overlayHeight = lipgloss.Height(m.renderCommandPalette()) + 1
		}
		modeTabsHeight := lipgloss.Height(m.renderModeTabs())
		hintHeight := lipgloss.Height(mutedStyle.Render(footerHintText))
		contentHeight := logoHeight + 1 + modeTabsHeight + 1 + overlayHeight + boxH + 1 + hintHeight
		contentTop := max(0, (m.height-contentHeight)/2)
		top = contentTop + logoHeight + 1 + modeTabsHeight + 1 + overlayHeight
		left = max(0, (m.width-boxW)/2)
		right = left + boxW - 1
		bottom = top + boxH - 1
		innerLeft = left + landingInputStyle.GetHorizontalFrameSize()/2
		innerTop = top + landingInputStyle.GetVerticalFrameSize()/2
		return left, right, top, bottom, innerLeft, innerTop, true
	default:
		return 0, 0, 0, 0, 0, 0, false
	}
}

func sliceViewportLineByCells(line string, startCol, endCol int) string {
	width := xansi.StringWidth(line)
	if width == 0 {
		return ""
	}
	if endCol < startCol {
		return ""
	}
	start := clamp(startCol, 0, width-1)
	end := clamp(endCol+1, start+1, width)
	return strings.TrimRight(xansi.Strip(xansi.Cut(line, start, end)), " ")
}

func normalizeViewportSelectionPoints(start, end viewportSelectionPoint) (viewportSelectionPoint, viewportSelectionPoint) {
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		return end, start
	}
	return start, end
}

func selectionColumnsForRow(row, width int, start, end viewportSelectionPoint, includePoint bool) (int, int, bool) {
	if width <= 0 || row < start.Row || row > end.Row {
		return 0, 0, false
	}
	if start.Row == end.Row {
		if start.Col == end.Col {
			if includePoint && row == start.Row {
				col := clamp(start.Col, 0, width-1)
				return col, col, true
			}
			return 0, 0, false
		}
		return clamp(start.Col, 0, width-1), clamp(end.Col, 0, width-1), true
	}
	switch row {
	case start.Row:
		return clamp(start.Col, 0, width-1), width - 1, true
	case end.Row:
		return 0, clamp(end.Col, 0, width-1), true
	default:
		return 0, width - 1, true
	}
}

func highlightVisibleLineByCells(line string, startCol, endCol int) string {
	width := xansi.StringWidth(line)
	if width == 0 {
		return ""
	}
	if endCol < startCol {
		return line
	}
	start := clamp(startCol, 0, width-1)
	end := clamp(endCol+1, start+1, width)
	left := xansi.Cut(line, 0, start)
	middle := selectionHighlightStyle.Render(xansi.Strip(xansi.Cut(line, start, end)))
	right := xansi.Cut(line, end, width)
	return left + middle + right
}

func selectionHasRange(start, end viewportSelectionPoint) bool {
	return start.Row != end.Row || start.Col != end.Col
}
