package tui

import tuichat "bytemind/internal/tui/chat"

func wrapPlainText(text string, width int) string {
	return tuichat.WrapPlainText(text, width)
}

func wrapLineSmart(line string, width int) []string {
	return tuichat.WrapLineSmart(line, width)
}

func tidyAssistantSpacing(text string) string {
	return tuichat.TidyAssistantSpacing(text)
}

func needsLeadingBlankLine(line string) bool {
	return tuichat.NeedsLeadingBlankLine(line)
}

func renderAssistantBody(text string, width int) string {
	return tuichat.RenderAssistantBody(text, width)
}

func normalizeAssistantMarkdownLine(line string) string {
	return tuichat.NormalizeAssistantMarkdownLine(line)
}

func splitOrderedListItem(line string) (marker string, rest string, ok bool) {
	return tuichat.SplitOrderedListItem(line)
}

func isMarkdownTableDivider(line string) bool {
	return tuichat.IsMarkdownTableDivider(line)
}

func stripMarkdownLinks(line string) string {
	return tuichat.StripMarkdownLinks(line)
}

func looksLikeMarkdownTable(line string) bool {
	return tuichat.LooksLikeMarkdownTable(line)
}
