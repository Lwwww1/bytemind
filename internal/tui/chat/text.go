package chat

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

var assistantInlineTokenReplacer = strings.NewReplacer(
	"**", "",
	"__", "",
	"~~", "",
	"`", "",
)

func WrapPlainText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		for _, part := range WrapLineSmart(line, width) {
			wrapped = append(wrapped, strings.TrimRight(part, " "))
		}
	}
	return strings.Join(wrapped, "\n")
}

func WrapLineSmart(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}

	out := make([]string, 0, 4)
	start := 0
	for start < len(runes) {
		curWidth := 0
		end := start
		lastSpaceEnd := -1

		for i := start; i < len(runes); i++ {
			rw := runewidth.RuneWidth(runes[i])
			if rw < 0 {
				rw = 0
			}
			if curWidth+rw > width {
				break
			}
			curWidth += rw
			end = i + 1
			if unicode.IsSpace(runes[i]) {
				lastSpaceEnd = i + 1
			}
		}

		if end == start {
			end = start + 1
		} else if lastSpaceEnd > start && end < len(runes) {
			end = lastSpaceEnd
		}

		segment := strings.TrimRightFunc(string(runes[start:end]), unicode.IsSpace)
		if segment == "" {
			segment = string(runes[start:end])
		}
		out = append(out, segment)
		start = end
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}

	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func TidyAssistantSpacing(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines)+4)
	inCodeBlock := false
	prevBlank := true

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !prevBlank && len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, line)
			inCodeBlock = !inCodeBlock
			prevBlank = false
			continue
		}

		if inCodeBlock {
			out = append(out, line)
			prevBlank = trimmed == ""
			continue
		}

		if trimmed == "" {
			if !prevBlank && len(out) > 0 {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}

		if NeedsLeadingBlankLine(trimmed) && !prevBlank && len(out) > 0 {
			out = append(out, "")
		}

		out = append(out, line)
		prevBlank = false
	}

	return strings.Join(out, "\n")
}

func NeedsLeadingBlankLine(line string) bool {
	if strings.HasPrefix(line, "#") {
		return true
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "> ") {
		return true
	}
	if len(line) >= 3 && line[1] == '.' && line[2] == ' ' && line[0] >= '0' && line[0] <= '9' {
		return true
	}
	return false
}

func RenderAssistantBody(text string, width int) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inCodeBlock := false
	prevBlank := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}

		plainLine := line
		if !inCodeBlock {
			plainLine = NormalizeAssistantMarkdownLine(line)
		}
		if strings.TrimSpace(plainLine) == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}
		out = append(out, WrapPlainText(plainLine, width))
		prevBlank = false
	}

	return strings.Join(out, "\n")
}

func NormalizeAssistantMarkdownLine(line string) string {
	indentWidth := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := line[:indentWidth]
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	for strings.HasPrefix(trimmed, ">") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	}
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level > 0 && (level == len(trimmed) || trimmed[level] == ' ') {
			trimmed = strings.TrimSpace(trimmed[level:])
		}
	}

	if IsMarkdownTableDivider(trimmed) {
		return ""
	}

	prefix := ""
	switch {
	case strings.HasPrefix(trimmed, "- [ ] "):
		prefix = "- [ ] "
		trimmed = strings.TrimSpace(trimmed[len("- [ ] "):])
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		prefix = "- [x] "
		trimmed = strings.TrimSpace(trimmed[len("- [x] "):])
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "+ "):
		prefix = "- "
		trimmed = strings.TrimSpace(trimmed[2:])
	default:
		if marker, rest, ok := SplitOrderedListItem(trimmed); ok {
			prefix = marker + " "
			trimmed = rest
		}
	}

	if LooksLikeMarkdownTable(trimmed) {
		parts := make([]string, 0, 8)
		for _, cell := range strings.Split(trimmed, "|") {
			cell = strings.TrimSpace(cell)
			if cell == "" {
				continue
			}
			parts = append(parts, cell)
		}
		trimmed = strings.Join(parts, " | ")
	}

	trimmed = StripMarkdownLinks(trimmed)
	trimmed = assistantInlineTokenReplacer.Replace(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	return indent + prefix + trimmed
}

func SplitOrderedListItem(line string) (marker string, rest string, ok bool) {
	if len(line) < 3 {
		return "", "", false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == 0 || len(line) <= index+1 || line[index] != '.' || line[index+1] != ' ' {
		return "", "", false
	}
	return line[:index+1], strings.TrimSpace(line[index+2:]), true
}

func IsMarkdownTableDivider(line string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if compact == "" || strings.Count(compact, "|") < 1 {
		return false
	}
	for _, ch := range compact {
		switch ch {
		case '|', '-', ':':
		default:
			return false
		}
	}
	return true
}

func StripMarkdownLinks(line string) string {
	if line == "" {
		return line
	}

	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		start := -1
		isImage := false
		switch {
		case i+1 < len(line) && line[i] == '!' && line[i+1] == '[':
			start = i + 2
			isImage = true
		case line[i] == '[':
			start = i + 1
		}

		if start < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}

		mid := strings.Index(line[start:], "](")
		if mid < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		textEnd := start + mid
		urlStart := textEnd + 2
		urlEndRel := strings.IndexByte(line[urlStart:], ')')
		if urlEndRel < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		urlEnd := urlStart + urlEndRel
		label := strings.TrimSpace(line[start:textEnd])
		url := strings.TrimSpace(line[urlStart:urlEnd])
		if label != "" {
			b.WriteString(label)
		}
		if url != "" && !isImage {
			b.WriteString(" (")
			b.WriteString(url)
			b.WriteString(")")
		}
		i = urlEnd + 1
	}
	return b.String()
}

func LooksLikeMarkdownTable(line string) bool {
	return strings.Count(line, "|") >= 2
}
