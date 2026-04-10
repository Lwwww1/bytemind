package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatChatBodyPreservesExplicitBlankLines(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "first paragraph\n\nsecond paragraph",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "first paragraph\n\nsecond paragraph") {
		t.Fatalf("expected explicit blank line to be preserved, got %q", got)
	}
}

func TestFormatChatBodyWrapsLongUserText(t *testing.T) {
	item := chatEntry{
		Kind: "user",
		Body: "Please describe the relationship between tui, session, agent, and tools so I can inspect how long user text wraps in the chat body.",
	}

	got := formatChatBody(item, 16)
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected long user text to wrap, got %q", got)
	}
	flat := strings.Join(strings.Fields(got), "")
	if flat != strings.Join(strings.Fields(item.Body), "") {
		t.Fatalf("expected wrapped user text to preserve all content, got %q", got)
	}
}

func TestFormatChatBodySeparatesParagraphAndList(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "Explanation\n- first\n- second",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "Explanation") {
		t.Fatalf("expected explanation text to remain, got %q", got)
	}
	if !strings.Contains(got, "- first") {
		t.Fatalf("expected markdown list marker to be normalized, got %q", got)
	}
}

func TestFormatChatBodyRendersMarkdownHeadingWithoutHashes(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "# Heading\nBody",
	}

	got := formatChatBody(item, 80)
	if strings.Contains(got, "# Heading") {
		t.Fatalf("expected heading marker to be stripped, got %q", got)
	}
	if !strings.Contains(got, "Heading") {
		t.Fatalf("expected heading text to remain, got %q", got)
	}
}

func TestFormatChatBodyRendersCodeBlockWithoutFences(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "```go\nfmt.Println(\"hi\")\n```",
	}

	got := formatChatBody(item, 80)
	if strings.Contains(got, "```") {
		t.Fatalf("expected code fences to be removed, got %q", got)
	}
	if !strings.Contains(got, "fmt.Println(\"hi\")") {
		t.Fatalf("expected code contents to remain, got %q", got)
	}
}

func TestFormatChatBodyStripsInlineMarkdownTokens(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "We are **ByteMind** project, support go test ./... and [docs](https://example.com/docs).",
	}

	got := formatChatBody(item, 120)
	for _, unwanted := range []string{"**", "`", "[", "]("} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("expected inline markdown token %q to be removed, got %q", unwanted, got)
		}
	}
	if !strings.Contains(got, "ByteMind") {
		t.Fatalf("expected bold content to remain after normalization, got %q", got)
	}
	if !strings.Contains(got, "go test ./...") {
		t.Fatalf("expected inline code content to remain after normalization, got %q", got)
	}
	if !strings.Contains(got, "docs (https://example.com/docs)") {
		t.Fatalf("expected markdown link to be normalized to plain text, got %q", got)
	}
}

func TestFinishAssistantMessageDoesNotAppendDuplicateCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{
				Kind:   "assistant",
				Title:  assistantLabel,
				Body:   "same answer",
				Status: "streaming",
			},
		},
		streamingIndex: -1,
	}

	m.finishAssistantMessage("same answer")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected no duplicate assistant card, got %d items", len(m.chatItems))
	}
	if m.chatItems[0].Status != "final" {
		t.Fatalf("expected assistant card to be marked final, got %q", m.chatItems[0].Status)
	}
}

func TestShouldKeepStreamingIndexOnRunFinishedBranches(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Status: "streaming"},
			{Kind: "assistant", Status: "thinking"},
			{Kind: "assistant", Status: "pending"},
			{Kind: "assistant", Status: "final"},
			{Kind: "tool", Status: "streaming"},
		},
	}

	for i, want := range []bool{true, true, true, false, false} {
		m.streamingIndex = i
		if got := m.shouldKeepStreamingIndexOnRunFinished(); got != want {
			t.Fatalf("unexpected keep-streaming result at index %d: got %v want %v", i, got, want)
		}
	}

	m.streamingIndex = -1
	if m.shouldKeepStreamingIndexOnRunFinished() {
		t.Fatalf("expected negative streaming index to return false")
	}
	m.streamingIndex = len(m.chatItems)
	if m.shouldKeepStreamingIndexOnRunFinished() {
		t.Fatalf("expected out-of-range streaming index to return false")
	}
}

func TestScrollbarTrackBoundsAndDragScrollbarTo(t *testing.T) {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(3)

	m := model{
		screen:     screenChat,
		width:      120,
		height:     32,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "assistant", Body: strings.Repeat("line\n", 260), Status: "final"},
		},
	}
	m.refreshViewport()

	x, top, bottom, ok := m.scrollbarTrackBounds()
	if !ok {
		t.Fatalf("expected scrollbar track bounds to be available")
	}
	if x < 0 || bottom < top {
		t.Fatalf("unexpected scrollbar bounds: x=%d top=%d bottom=%d", x, top, bottom)
	}

	m.scrollbarDragOffset = 0
	m.dragScrollbarTo(bottom)
	if m.viewport.YOffset == 0 {
		t.Fatalf("expected dragging to scrollbar bottom to increase viewport offset")
	}
	afterBottom := m.viewport.YOffset
	m.dragScrollbarTo(top)
	if m.viewport.YOffset >= afterBottom {
		t.Fatalf("expected dragging to top to reduce offset, got %d -> %d", afterBottom, m.viewport.YOffset)
	}

	// Guard branch: track bounds unavailable.
	before := m.viewport.YOffset
	m.screen = screenLanding
	m.dragScrollbarTo(bottom)
	if m.viewport.YOffset != before {
		t.Fatalf("expected drag to no-op when track bounds are unavailable")
	}

	// Guard branch: no scrollable range (maxOffset == 0).
	m.screen = screenChat
	m.chatItems = []chatEntry{{Kind: "assistant", Body: "single line", Status: "final"}}
	m.refreshViewport()
	before = m.viewport.YOffset
	m.dragScrollbarTo(top)
	if m.viewport.YOffset != before {
		t.Fatalf("expected drag to no-op when content has no scrollable range")
	}
}

func TestHandleMouseScrollbarDragLifecycle(t *testing.T) {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(3)

	m := model{
		screen:         screenChat,
		width:          120,
		height:         32,
		input:          input,
		viewport:       viewport.New(60, 10),
		tokenUsage:     newTokenUsageComponent(),
		chatAutoFollow: true,
		chatItems: []chatEntry{
			{Kind: "assistant", Body: strings.Repeat("row\n", 280), Status: "final"},
		},
	}
	m.refreshViewport()

	x, top, bottom, ok := m.scrollbarTrackBounds()
	if !ok {
		t.Fatalf("expected scrollbar bounds for drag test")
	}

	// Click near track bottom so we exercise "track click jump + start drag" branch.
	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      bottom,
	})
	pressed := got.(model)
	if !pressed.draggingScrollbar {
		t.Fatalf("expected dragging mode after pressing scrollbar track")
	}
	if pressed.chatAutoFollow {
		t.Fatalf("expected auto-follow to be disabled once dragging starts")
	}

	beforeOffset := pressed.viewport.YOffset
	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      x,
		Y:      top,
	})
	moved := got.(model)
	if moved.viewport.YOffset == beforeOffset {
		t.Fatalf("expected motion while dragging to update viewport offset")
	}

	got, _ = moved.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      top,
	})
	released := got.(model)
	if released.draggingScrollbar {
		t.Fatalf("expected release to end scrollbar dragging")
	}
}

func TestHandleMouseGuardBranchesAndThumbPress(t *testing.T) {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(3)

	// Release should clear dragging even when another overlay short-circuits later logic.
	m := model{
		screen:            screenChat,
		width:             120,
		height:            28,
		input:             input,
		viewport:          viewport.New(60, 10),
		tokenUsage:        newTokenUsageComponent(),
		draggingScrollbar: true,
		helpOpen:          true,
	}
	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	updated := got.(model)
	if updated.draggingScrollbar {
		t.Fatalf("expected release to clear dragging even when help modal is open")
	}

	// Unsupported screen should return without changes.
	m = model{screen: screenKind("other"), viewport: viewport.New(20, 4)}
	before := m.viewport.YOffset
	got, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	updated = got.(model)
	if updated.viewport.YOffset != before {
		t.Fatalf("expected unsupported screen to ignore mouse event")
	}

	// Sessions modal open on chat screen should block viewport scrolling.
	m = model{
		screen:       screenChat,
		sessionsOpen: true,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Repeat("line\n", 60))
			return vp
		}(),
	}
	before = m.viewport.YOffset
	got, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	updated = got.(model)
	if updated.viewport.YOffset != before {
		t.Fatalf("expected sessions-open state to ignore mouse wheel scrolling")
	}

	// Clicking directly on thumb should use the direct-offset branch.
	m = model{
		screen:     screenChat,
		width:      120,
		height:     32,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "assistant", Body: strings.Repeat("thumb\n", 220), Status: "final"},
		},
	}
	m.refreshViewport()
	x, trackTop, _, ok := m.scrollbarTrackBounds()
	if !ok {
		t.Fatalf("expected scrollbar bounds for thumb click")
	}
	thumbTop, thumbHeight, _, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
	if !visible || thumbHeight <= 0 {
		t.Fatalf("expected visible thumb for thumb-click branch")
	}
	insideThumbY := trackTop + thumbTop
	got, _ = m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      insideThumbY,
	})
	updated = got.(model)
	if !updated.draggingScrollbar {
		t.Fatalf("expected thumb press to start dragging")
	}
	if updated.scrollbarDragOffset != 0 {
		t.Fatalf("expected thumb-top press to use zero drag offset, got %d", updated.scrollbarDragOffset)
	}
}

func TestRenderTokenBadgeAndScrollbarHelpers(t *testing.T) {
	m := model{
		screen:     screenChat,
		width:      110,
		height:     30,
		input:      textarea.New(),
		viewport:   viewport.New(50, 8),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Repeat("line\n", 120))
	m.tokenUsage.displayUsed = 2345
	_ = m.tokenUsage.SetUsage(2345, 5000)
	m.refreshViewport()

	compact := m.renderTokenBadge(79)
	if strings.Contains(compact, "/") {
		t.Fatalf("expected compact badge under width threshold, got %q", compact)
	}
	full := m.renderTokenBadge(80)
	if !strings.Contains(full, "token: 2,345") {
		t.Fatalf("expected full badge to show token count, got %q", full)
	}

	if got := m.renderScrollbar(0, 10, 0); got != "" {
		t.Fatalf("expected empty scrollbar when view height is zero, got %q", got)
	}
	bar := m.renderScrollbar(8, 120, 5)
	if lines := strings.Count(bar, "\n") + 1; lines != 8 {
		t.Fatalf("expected scrollbar to have 8 visual rows, got %d", lines)
	}

	thumbTop, thumbHeight, maxOffset, visible := m.scrollbarLayout(8, 200, 9999)
	if !visible || thumbHeight <= 0 || maxOffset <= 0 {
		t.Fatalf("expected visible scrollbar layout with valid dimensions")
	}
	if thumbTop < 0 || thumbTop > 8-thumbHeight {
		t.Fatalf("expected clamped thumb top, got top=%d height=%d", thumbTop, thumbHeight)
	}

	thumbTop, thumbHeight, maxOffset, visible = m.scrollbarLayout(8, 0, 0)
	if !visible || thumbTop != 0 || thumbHeight != 8 || maxOffset != 0 {
		t.Fatalf("expected zero-content layout fallback, got top=%d height=%d max=%d visible=%v", thumbTop, thumbHeight, maxOffset, visible)
	}
}

func TestWrapLineSmartBranchCoverage(t *testing.T) {
	if got := wrapLineSmart("abc", 0); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("expected width<=0 to return original line, got %#v", got)
	}
	if got := wrapLineSmart("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("expected empty line to remain empty, got %#v", got)
	}

	wideRune := wrapLineSmart("\u4f60\u597d", 1)
	if len(wideRune) < 2 || wideRune[0] != "\u4f60" {
		t.Fatalf("expected wide-rune fallback split, got %#v", wideRune)
	}

	words := wrapLineSmart("hello world", 6)
	if len(words) < 2 || words[0] != "hello" {
		t.Fatalf("expected split at word boundary, got %#v", words)
	}
}

func TestMarkdownNormalizationHelpers(t *testing.T) {
	if got := normalizeAssistantMarkdownLine(""); got != "" {
		t.Fatalf("expected empty line to normalize to empty, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("> ## Heading"); got != "Heading" {
		t.Fatalf("expected quote heading to normalize, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine(" - [x] done **item** "); got != " - [x] done item" {
		t.Fatalf("expected checkbox normalization, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("1. [Doc](https://example.com)"); got != "1. Doc (https://example.com)" {
		t.Fatalf("expected ordered list with markdown link normalization, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("| --- | :---: |"); got != "" {
		t.Fatalf("expected table divider to be stripped, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("| a | b |"); got != "a | b" {
		t.Fatalf("expected table row to normalize, got %q", got)
	}

	if marker, rest, ok := splitOrderedListItem("12. step one"); !ok || marker != "12." || rest != "step one" {
		t.Fatalf("expected ordered list split, got marker=%q rest=%q ok=%v", marker, rest, ok)
	}
	if _, _, ok := splitOrderedListItem("a. not-ordered"); ok {
		t.Fatalf("expected invalid ordered-list marker to fail split")
	}

	if !isMarkdownTableDivider("| --- | :---: |") {
		t.Fatalf("expected markdown table divider to be detected")
	}
	if isMarkdownTableDivider("| a | b |") {
		t.Fatalf("expected non-divider row not to be treated as divider")
	}

	normalizedLinks := stripMarkdownLinks("see [Doc](https://x.test) and ![img](https://img.test)")
	if !strings.Contains(normalizedLinks, "Doc (https://x.test)") {
		t.Fatalf("expected standard link to preserve URL in text, got %q", normalizedLinks)
	}
	if !strings.Contains(normalizedLinks, "img") || strings.Contains(normalizedLinks, "img.test") {
		t.Fatalf("expected image link to keep label only, got %q", normalizedLinks)
	}

	broken := "[Doc](https://example.com"
	if got := stripMarkdownLinks(broken); got != broken {
		t.Fatalf("expected malformed markdown link to remain unchanged, got %q", got)
	}
}

func TestThinkingFilters(t *testing.T) {
	if isMeaningfulThinking("I will call read_file first.", "read_file") {
		t.Fatalf("expected generic tool-intent phrase not to be treated as meaningful thinking")
	}
	if !isMeaningfulThinking("I will first inspect the code and then patch tests.", "") {
		t.Fatalf("expected concrete planning thought to be meaningful")
	}
	if shouldRenderThinkingFromDelta("I will call read_file now.") {
		t.Fatalf("expected generic call text not to render as thinking delta")
	}
	if !shouldRenderThinkingFromDelta("First, I will inspect the failing branch and then patch tests.") {
		t.Fatalf("expected structured reasoning marker to trigger thinking rendering")
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
