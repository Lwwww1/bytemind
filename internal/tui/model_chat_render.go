package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func rebuildSessionTimeline(sess *session.Session) ([]chatEntry, []toolRun) {
	items := make([]chatEntry, 0, len(sess.Messages))
	runs := make([]toolRun, 0, 8)
	callNames := map[string]string{}

	for _, message := range sess.Messages {
		message.Normalize()
		switch message.Role {
		case "user":
			userTextParts := make([]string, 0, len(message.Parts))
			for _, part := range message.Parts {
				if part.Text != nil {
					userTextParts = append(userTextParts, part.Text.Value)
				}
				if part.ToolResult == nil {
					continue
				}
				name := callNames[part.ToolResult.ToolUseID]
				if name == "" {
					name = "tool"
				}
				summary, lines, status := summarizeTool(name, part.ToolResult.Content)
				items = append(items, chatEntry{
					Kind:   "tool",
					Title:  "Tool Call | " + name,
					Body:   joinSummary(summary, lines),
					Status: status,
				})
				runs = append(runs, toolRun{Name: name, Summary: summary, Lines: lines, Status: status})
			}
			userText := strings.Join(userTextParts, "")
			if strings.TrimSpace(userText) != "" {
				items = append(items, chatEntry{Kind: "user", Title: "You", Body: userText, Status: "final"})
			}
		case "assistant":
			for _, call := range message.ToolCalls {
				callNames[call.ID] = call.Function.Name
			}
			if strings.TrimSpace(message.Text()) != "" {
				items = append(items, chatEntry{Kind: "assistant", Title: assistantLabel, Body: message.Text(), Status: "final"})
			}
		case "tool":
			name := callNames[message.ToolCallID]
			if name == "" {
				name = "tool"
			}
			summary, lines, status := summarizeTool(name, message.Content)
			items = append(items, chatEntry{
				Kind:   "tool",
				Title:  "Tool Call | " + name,
				Body:   joinSummary(summary, lines),
				Status: status,
			})
			runs = append(runs, toolRun{Name: name, Summary: summary, Lines: lines, Status: status})
		}
	}
	return items, runs
}

func renderChatCard(item chatEntry, width int) string {
	border := chatAssistantStyle
	switch item.Kind {
	case "user":
		border = chatUserStyle
	case "tool":
		border = chatAssistantStyle
	case "system":
		border = chatSystemStyle
	default:
		if item.Status == "thinking" {
			border = chatThinkingStyle
		}
	}
	contentWidth := max(8, width-border.GetHorizontalFrameSize())
	rendered := border.Width(contentWidth).Render(renderChatSection(item, contentWidth))
	if item.Kind != "tool" {
		return rendered
	}

	sep := lipgloss.NewStyle().Foreground(colorTool).Render("|")
	lines := strings.Split(rendered, "\n")
	for i := range lines {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = "  " + lines[i]
			continue
		}
		lines[i] = sep + " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func renderChatSection(item chatEntry, width int) string {
	title := cardTitleStyle.Foreground(colorAccent)
	bodyStyle := chatBodyStyle
	toolCallTitle := cardTitleStyle.Foreground(lipgloss.Color("#E5B567")).Bold(true)
	toolResultTitle := cardTitleStyle.Foreground(lipgloss.Color("#7AC7FF")).Bold(true)
	status := item.Status
	displayTitle := item.Title
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "user":
		title = cardTitleStyle.Foreground(colorUser)
	case "tool":
		if strings.HasPrefix(displayTitle, "Tool Result | ") {
			title = toolResultTitle
		} else {
			title = toolCallTitle
		}
		bodyStyle = toolBodyStyle
		status = ""
	case "system":
		title = cardTitleStyle.Foreground(colorMuted)
	default:
		if item.Status == "thinking" {
			title = cardTitleStyle.Foreground(colorMuted).Faint(true)
			bodyStyle = thinkingBodyStyle
			displayTitle = "thinking"
			status = ""
		}
	}
	headContent := title.Render(displayTitle)
	if item.Kind == "user" && strings.TrimSpace(item.Meta) != "" {
		headContent = mutedStyle.Copy().Faint(true).Render(item.Meta)
	}
	if status != "" {
		headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, mutedStyle.Render("  "+status))
	}
	head := lipgloss.NewStyle().
		Width(width).
		Render(headContent)
	if item.Kind == "tool" && strings.TrimSpace(item.Body) == "" {
		return head
	}
	body := bodyStyle.Width(width).Render(formatChatBody(item, width))
	return lipgloss.JoinVertical(lipgloss.Left, head, body)
}

func renderChatRow(item chatEntry, width int) string {
	bubbleWidth := chatBubbleWidth(item, width)
	card := renderChatCard(item, bubbleWidth)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunRow(items []chatEntry, width int) string {
	if len(items) == 0 {
		return ""
	}
	card := renderBytemindRunCard(items, width)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunCard(items []chatEntry, width int) string {
	outer := chatAssistantStyle
	contentWidth := max(8, width-outer.GetHorizontalFrameSize())
	sections := make([]string, 0, len(items))
	for _, item := range items {
		sections = append(sections, renderChatSection(item, contentWidth))
	}
	return outer.Width(contentWidth).Render(strings.Join(sections, "\n"))
}

func renderModal(width, height int, modal string) string {
	if width == 0 || height == 0 {
		return modal
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func formatChatBody(item chatEntry, width int) string {
	text := strings.ReplaceAll(item.Body, "\r\n", "\n")
	if item.Kind != "assistant" {
		return strings.TrimRight(wrapPlainText(text, width), "\n")
	}
	return strings.TrimRight(renderAssistantBody(text, width), "\n")
}

func isMarkdownHeading(line string) bool {
	return strings.HasPrefix(line, "# ") ||
		strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ")
}

func renderMarkdownHeading(line string, width int) string {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	text := strings.TrimSpace(line[level:])
	style := assistantHeading3Style
	switch level {
	case 1:
		style = assistantHeading1Style
	case 2:
		style = assistantHeading2Style
	}
	wrapped := strings.Split(wrapPlainText(text, width), "\n")
	rendered := make([]string, 0, len(wrapped))
	for _, part := range wrapped {
		rendered = append(rendered, style.Render(part))
	}
	return strings.Join(rendered, "\n")
}

func isMarkdownListItem(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	return isOrderedListItem(line)
}

func isOrderedListItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	return index > 0 && len(line) > index+1 && line[index] == '.' && line[index+1] == ' '
}

func renderMarkdownListItem(line string, width int) string {
	indentWidth := len(line) - len(strings.TrimLeft(line, " "))
	indent := strings.Repeat(" ", indentWidth)
	trimmed := strings.TrimSpace(line)
	marker := ""
	content := ""

	switch {
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		marker = trimmed[:1]
		content = strings.TrimSpace(trimmed[2:])
	default:
		for i := 0; i < len(trimmed); i++ {
			if trimmed[i] == '.' && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
				marker = trimmed[:i+1]
				content = strings.TrimSpace(trimmed[i+2:])
				break
			}
		}
	}

	if content == "" {
		content = trimmed
	}

	prefix := indent + marker + " "
	contentWidth := max(8, width-runewidth.StringWidth(prefix))
	wrapped := strings.Split(wrapPlainText(content, contentWidth), "\n")
	lines := make([]string, 0, len(wrapped))
	for i, part := range wrapped {
		if i == 0 {
			lines = append(lines, indent+listMarkerStyle.Render(marker)+" "+part)
			continue
		}
		lines = append(lines, indent+strings.Repeat(" ", runewidth.StringWidth(marker))+" "+part)
	}
	return strings.Join(lines, "\n")
}

func renderMarkdownQuote(line string, width int) string {
	content := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	wrapped := strings.Split(wrapPlainText(content, max(8, width-2)), "\n")
	rendered := make([]string, 0, len(wrapped))
	for _, part := range wrapped {
		rendered = append(rendered, quoteLineStyle.Render(part))
	}
	return strings.Join(rendered, "\n")
}

func chatBubbleWidth(item chatEntry, width int) int {
	if width <= 28 {
		return width
	}
	return width
}

func summarizeTool(name, payload string) (string, []string, string) {
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != "" {
		return envelope.Error, nil, "error"
	}

	switch name {
	case "list_files":
		var result struct {
			Root  string `json:"root"`
			Items []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(3, len(result.Items)))
			for i := 0; i < min(3, len(result.Items)); i++ {
				lines = append(lines, result.Items[i].Type+" "+result.Items[i].Path)
			}
			return fmt.Sprintf("Listed %d items under %s", len(result.Items), emptyDot(result.Root)), lines, "done"
		}
	case "read_file":
		var result struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			return fmt.Sprintf("Read %s lines %d-%d", result.Path, result.StartLine, result.EndLine), nil, "done"
		}
	case "search_text":
		var result struct {
			Query   string `json:"query"`
			Matches []struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Text string `json:"text"`
			} `json:"matches"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(3, len(result.Matches)))
			for i := 0; i < min(3, len(result.Matches)); i++ {
				match := result.Matches[i]
				lines = append(lines, fmt.Sprintf("%s:%d %s", match.Path, match.Line, compact(match.Text, 72)))
			}
			return fmt.Sprintf("Found %d match(es) for %q", len(result.Matches), result.Query), lines, "done"
		}
	case "web_search":
		var result struct {
			Query   string `json:"query"`
			Results []struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			} `json:"results"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(3, len(result.Results)))
			for i := 0; i < min(3, len(result.Results)); i++ {
				item := result.Results[i]
				title := compact(item.Title, 56)
				if strings.TrimSpace(title) == "" {
					title = item.URL
				}
				lines = append(lines, title+" - "+item.URL)
			}
			return fmt.Sprintf("Searched web for %q (%d result(s))", result.Query, len(result.Results)), lines, "done"
		}
	case "web_fetch":
		var result struct {
			URL        string `json:"url"`
			StatusCode int    `json:"status_code"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			Truncated  bool   `json:"truncated"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, 2)
			if strings.TrimSpace(result.Title) != "" {
				lines = append(lines, "title: "+compact(result.Title, 72))
			}
			if strings.TrimSpace(result.Content) != "" {
				lines = append(lines, "preview: "+compact(result.Content, 72))
			}
			if result.Truncated {
				lines = append(lines, "content truncated")
			}
			return fmt.Sprintf("Fetched %s (HTTP %d)", result.URL, result.StatusCode), lines, "done"
		}
	case "write_file":
		var result struct {
			Path         string `json:"path"`
			BytesWritten int    `json:"bytes_written"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			return fmt.Sprintf("Wrote %s (%d bytes)", result.Path, result.BytesWritten), nil, "done"
		}
	case "replace_in_file":
		var result struct {
			Path     string `json:"path"`
			Replaced int    `json:"replaced"`
			OldCount int    `json:"old_count"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			return fmt.Sprintf("Updated %s (%d/%d)", result.Path, result.Replaced, result.OldCount), nil, "done"
		}
	case "apply_patch":
		var result struct {
			Operations []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"operations"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(4, len(result.Operations)))
			for i := 0; i < min(4, len(result.Operations)); i++ {
				lines = append(lines, result.Operations[i].Type+" "+result.Operations[i].Path)
			}
			return fmt.Sprintf("Patched %d file operation(s)", len(result.Operations)), lines, "done"
		}
	case "update_plan":
		var result struct {
			Plan planpkg.State `json:"plan"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(4, len(result.Plan.Steps)))
			for i := 0; i < min(4, len(result.Plan.Steps)); i++ {
				step := result.Plan.Steps[i]
				lines = append(lines, fmt.Sprintf("[%s] %s", step.Status, step.Title))
			}
			return fmt.Sprintf("Updated plan with %d step(s)", len(result.Plan.Steps)), lines, "done"
		}
	case "run_shell":
		var result struct {
			OK       bool   `json:"ok"`
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, 2)
			if text := strings.TrimSpace(result.Stdout); text != "" {
				lines = append(lines, "stdout: "+compact(strings.Split(text, "\n")[0], 72))
			}
			if text := strings.TrimSpace(result.Stderr); text != "" {
				lines = append(lines, "stderr: "+compact(strings.Split(text, "\n")[0], 72))
			}
			status := "done"
			if !result.OK {
				status = "warn"
			}
			return fmt.Sprintf("Shell exited with code %d", result.ExitCode), lines, status
		}
	}

	return compact(payload, 96), nil, "done"
}

func joinSummary(summary string, lines []string) string {
	if len(lines) == 0 {
		return summary
	}
	return summary + "\n" + strings.Join(lines, "\n")
}

func summarizeArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return "default arguments"
	}
	return compact(raw, 88)
}

func legacyAssistantToolIntro(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return "I will check the relevant context first."
	}
	return fmt.Sprintf("I will call `%s` to inspect the relevant context first.", toolName)
}

func legacyAssistantToolFollowUp(toolName, summary, status string) string {
	if strings.TrimSpace(summary) == "" {
		return "I have the tool result. Let me organize the next step."
	}
	switch status {
	case "error", "warn":
		return fmt.Sprintf("`%s` returned a result. I will continue from that signal.", toolName)
	default:
		return fmt.Sprintf("`%s` finished successfully. I will keep using the result.", toolName)
	}
}

func assistantToolIntro(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return "I will check the relevant context first."
	}
	return fmt.Sprintf("I will call `%s` to inspect the relevant context first.", toolName)
}

func assistantToolFollowUp(toolName, summary, status string) string {
	if strings.TrimSpace(summary) == "" {
		return "I have the tool result. Let me organize the next step."
	}
	switch status {
	case "error", "warn":
		return fmt.Sprintf("`%s` returned a result. I will continue from that signal.", toolName)
	default:
		return fmt.Sprintf("`%s` finished successfully. I will keep using the result.", toolName)
	}
}

func isMeaningfulThinking(body, toolName string) bool {
	raw := strings.TrimSpace(body)
	if raw == "" {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(raw, "`", ""))
	toolName = strings.ToLower(strings.TrimSpace(toolName))

	genericPrefixes := []string{
		"i will call ",
		"i'll call ",
		"let me call ",
		"i am going to call ",
		"i'm going to call ",
		"i will use ",
		"i'll use ",
		"let me use ",
		"i will run ",
		"let me run ",
		"i will check the relevant context first",
		"i have the tool result. let me organize the next step.",
	}
	for _, prefix := range genericPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return false
		}
	}

	if toolName != "" {
		toolIntentPhrases := []string{
			fmt.Sprintf("call %s", toolName),
			fmt.Sprintf("use %s", toolName),
			fmt.Sprintf("run %s", toolName),
		}
		for _, phrase := range toolIntentPhrases {
			if strings.Contains(normalized, phrase) && strings.Contains(normalized, "inspect") {
				return false
			}
		}
	}

	cnPrefixes := []string{
		"i will call",
		"i will use",
		"let me call",
		"let me use",
		"let me run",
		"tool result",
	}
	for _, prefix := range cnPrefixes {
		if strings.HasPrefix(raw, prefix) {
			return false
		}
	}

	return true
}

func shouldRenderThinkingFromDelta(body string) bool {
	text := strings.TrimSpace(body)
	if text == "" {
		return false
	}
	if !isMeaningfulThinking(text, "") {
		return false
	}
	lower := strings.ToLower(text)
	reasoningMarkers := []string{
		"i will first",
		"first,",
		"then",
		"finally",
		"approach",
		"systematically",
		"through build and test",
		"\u6211\u4f1a\u5148",
		"\u5148\u4e86\u89e3",
		"\u7136\u540e",
		"\u6700\u540e",
		"\u901a\u8fc7\u6784\u5efa\u548c\u6d4b\u8bd5",
		"\u7cfb\u7edf\u6027",
	}
	for _, marker := range reasoningMarkers {
		if strings.Contains(lower, marker) || strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (m model) thinkingText() string {
	return fmt.Sprintf("%s Thinking... request already sent to the LLM, waiting for response.", m.spinner.View())
}
