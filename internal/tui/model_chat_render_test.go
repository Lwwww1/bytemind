package tui

import (
	"strings"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/llm"
	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

func TestAssistantChatBubbleUsesFullAvailableWidth(t *testing.T) {
	width := 80
	assistantWidth := chatBubbleWidth(chatEntry{Kind: "assistant"}, width)
	if assistantWidth != width {
		t.Fatalf("expected assistant bubble width %d, got %d", width, assistantWidth)
	}

	userWidth := chatBubbleWidth(chatEntry{Kind: "user"}, width)
	if userWidth != width {
		t.Fatalf("expected user bubble width %d, got %d", width, userWidth)
	}
}

func TestRenderChatRowFitsViewportWidth(t *testing.T) {
	row := renderChatRow(chatEntry{
		Kind:   "user",
		Title:  "You",
		Body:   "Please describe the relationship between tui, session, agent, and tools in several paragraphs so we can inspect wrapping behavior.",
		Status: "final",
	}, 80)

	if lipgloss.Width(row) > 80 {
		t.Fatalf("expected rendered row to fit viewport width, got %d", lipgloss.Width(row))
	}
	if !strings.Contains(row, "Please describe the relationship") {
		t.Fatalf("expected rendered row to contain the full user message")
	}
}

func TestRenderConversationPreservesFullUserText(t *testing.T) {
	m := model{
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 10)
			vp.Width = 40
			return vp
		}(),
		chatItems: []chatEntry{
			{
				Kind:   "user",
				Title:  "You",
				Body:   "Please describe the relationship between tui, session, agent, and tools in several detailed sections.",
				Status: "final",
			},
		},
	}

	got := m.renderConversation()
	flat := strings.Join(strings.Fields(got), "")
	for _, want := range []string{"Pleasedescribetherelationship", "session,agent,andtools", "severaldetailedsections"} {
		if !strings.Contains(flat, want) {
			t.Fatalf("expected conversation to preserve %q, got %q", want, got)
		}
	}
}

func TestRenderConversationIncludesToolEntries(t *testing.T) {
	m := model{
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(60, 10)
			vp.Width = 60
			return vp
		}(),
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "check repo", Status: "final"},
			{Kind: "tool", Title: "Tool Call | read_file", Body: "Read internal/tui/model.go lines 1-20", Status: "done"},
		},
	}

	got := m.renderConversation()
	if !strings.Contains(got, "Tool Call | read_file") {
		t.Fatalf("expected conversation to show tool entry, got %q", got)
	}
	if !strings.Contains(got, "Read internal/tui/model.go lines 1-20") {
		t.Fatalf("expected conversation to show tool summary, got %q", got)
	}
}

func TestRebuildSessionTimelineParsesUserToolResultParts(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			llm.NewUserTextMessage("please inspect"),
			{
				Role: llm.RoleAssistant,
				Parts: []llm.Part{{
					Type: llm.PartToolUse,
					ToolUse: &llm.ToolUsePart{
						ID:        "call-1",
						Name:      "read_file",
						Arguments: `{"path":"a.txt"}`,
					},
				}},
			},
			llm.NewToolResultMessage("call-1", `{"path":"a.txt","content":"ok"}`),
		},
	}

	items, runs := rebuildSessionTimeline(sess)
	if len(items) != 2 {
		t.Fatalf("expected user + tool items, got %#v", items)
	}
	if items[1].Kind != "tool" || !strings.Contains(items[1].Title, "Tool Call | read_file") {
		t.Fatalf("expected tool item from tool_result part, got %#v", items[1])
	}
	if len(runs) != 1 || runs[0].Name != "read_file" {
		t.Fatalf("expected tool run reconstructed, got %#v", runs)
	}
}

func TestRebuildSessionTimelineFallsBackToGenericToolNameForUnknownToolUseID(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			llm.NewToolResultMessage("missing-call-id", `{"ok":true}`),
		},
	}

	items, runs := rebuildSessionTimeline(sess)
	if len(items) != 1 {
		t.Fatalf("expected only one tool item, got %#v", items)
	}
	if items[0].Kind != "tool" || items[0].Title != "Tool Call | tool" {
		t.Fatalf("expected fallback tool title for unknown tool use id, got %#v", items[0])
	}
	if len(runs) != 1 || runs[0].Name != "tool" {
		t.Fatalf("expected fallback tool run name, got %#v", runs)
	}
}

func TestRebuildSessionTimelineParsesLegacyToolRoleMessage(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			llm.NewAssistantTextMessage("analysis complete"),
			{
				Role:       llm.Role("tool"),
				ToolCallID: "missing-call-id",
				Content:    `{"path":"a.txt","content":"ok"}`,
			},
		},
	}

	items, runs := rebuildSessionTimeline(sess)
	if len(items) != 2 {
		t.Fatalf("expected assistant + tool items, got %#v", items)
	}
	if items[0].Kind != "assistant" || !strings.Contains(items[0].Body, "analysis complete") {
		t.Fatalf("expected assistant text item from legacy message, got %#v", items[0])
	}
	if items[1].Kind != "tool" || items[1].Title != "Tool Call | tool" {
		t.Fatalf("expected fallback tool title for legacy tool message, got %#v", items[1])
	}
	if len(runs) != 1 || runs[0].Name != "tool" {
		t.Fatalf("expected tool run reconstructed from legacy tool message, got %#v", runs)
	}
}

func TestHandleAgentEventShowsToolProgressInChat(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "what project is this", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking", Status: "thinking"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "read_file",
		ToolArguments: `{"path":"internal/tui/model.go"}`,
	})
	if len(m.chatItems) != 3 {
		t.Fatalf("expected tool start to keep assistant step then append tool call, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Kind != "assistant" || m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" || strings.TrimSpace(m.chatItems[1].Body) == "" {
		t.Fatalf("expected assistant step before tool call, got %+v", m.chatItems[1])
	}
	if m.chatItems[2].Kind != "tool" || m.chatItems[2].Status != "running" || !strings.Contains(m.chatItems[2].Title, "Tool Call | read_file") {
		t.Fatalf("expected running tool call chat item, got %+v", m.chatItems[2])
	}
	if strings.TrimSpace(m.chatItems[2].Body) != "" {
		t.Fatalf("expected tool call body to hide params, got %q", m.chatItems[2].Body)
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":20}`,
	})
	if len(m.chatItems) != 3 {
		t.Fatalf("expected completed tool to update existing tool call, got %d", len(m.chatItems))
	}
	if m.chatItems[2].Kind != "tool" || !strings.Contains(m.chatItems[2].Title, "Tool Call | read_file") {
		t.Fatalf("expected tool call entry after completion, got %+v", m.chatItems[2])
	}
	if m.chatItems[2].Status != "done" {
		t.Fatalf("expected completed tool call status to be done, got %q", m.chatItems[2].Status)
	}
	if !strings.Contains(m.chatItems[2].Body, "Read internal/tui/model.go lines 1-20") {
		t.Fatalf("expected completed tool summary in tool call item, got %q", m.chatItems[2].Body)
	}
}

func TestHandleAgentEventTracksRunLifecyclePhases(t *testing.T) {
	m := model{
		busy:         true,
		llmConnected: true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "inspect tui", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking", Status: "thinking"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "Inspecting the TUI flow...",
	})
	if m.phase != "responding" || m.statusNote != "LLM is responding..." {
		t.Fatalf("expected assistant delta to move UI into responding phase, got phase=%q note=%q", m.phase, m.statusNote)
	}
	if m.chatItems[1].Status != "streaming" || !strings.Contains(m.chatItems[1].Body, "Inspecting the TUI flow") {
		t.Fatalf("expected streaming assistant card after delta, got %+v", m.chatItems[1])
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "read_file",
		ToolArguments: `{"path":"internal/tui/model.go","start_line":1,"end_line":5}`,
	})
	if m.phase != "tool" || m.statusNote != "Running tool: read_file" {
		t.Fatalf("expected tool start to move UI into tool phase, got phase=%q note=%q", m.phase, m.statusNote)
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":5}`,
	})
	if m.phase != "thinking" {
		t.Fatalf("expected completed tool to return UI to thinking phase, got %q", m.phase)
	}
	if !strings.Contains(m.statusNote, "Read internal/tui/model.go lines 1-5") {
		t.Fatalf("expected tool result summary in status note, got %q", m.statusNote)
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventRunFinished,
		Content: "Done.",
	})
	if m.phase != "idle" || m.statusNote != "Run finished." {
		t.Fatalf("expected run finished event to return UI to idle, got phase=%q note=%q", m.phase, m.statusNote)
	}
}

func TestToolStartKeepsStreamedAssistantReasoning(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "what project is this", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "let me inspect the repo structure first", Status: "streaming"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "list_files",
		ToolArguments: `{"path":"."}`,
	})

	if len(m.chatItems) != 3 {
		t.Fatalf("expected tool start to append only tool call after streamed assistant turn, got %d items", len(m.chatItems))
	}
	if !strings.Contains(m.chatItems[1].Body, "inspect the repo structure first") || m.chatItems[1].Status != "thinking" || m.chatItems[1].Title != thinkingLabel {
		t.Fatalf("expected streamed assistant turn to preserve reasoning content, got %+v", m.chatItems[1])
	}
	if !strings.Contains(m.chatItems[2].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry, got %+v", m.chatItems[2])
	}
}

func TestToolStartWithoutAssistantDeltaDoesNotInjectThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "list files", Status: "final"},
		},
		streamingIndex: -1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "list_files",
		ToolArguments: `{"path":"."}`,
	})

	if len(m.chatItems) != 2 {
		t.Fatalf("expected only tool call entry to be appended, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Kind != "tool" || !strings.Contains(m.chatItems[1].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry, got %+v", m.chatItems[1])
	}
	if strings.TrimSpace(m.chatItems[1].Body) != "" {
		t.Fatalf("expected tool call entry to omit params body, got %q", m.chatItems[1].Body)
	}
}

func TestToolStartWithGenericToolIntentDoesNotShowThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "list files", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "I will call `list_files` to inspect the relevant context first.", Status: "streaming"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "list_files",
		ToolArguments: `{"path":"."}`,
	})

	if len(m.chatItems) != 2 {
		t.Fatalf("expected generic tool-intent placeholder to be removed, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Kind != "tool" || !strings.Contains(m.chatItems[1].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry after removing placeholder, got %+v", m.chatItems[1])
	}
	if strings.TrimSpace(m.chatItems[1].Body) != "" {
		t.Fatalf("expected tool call entry to omit params body, got %q", m.chatItems[1].Body)
	}
}

func TestRenderChatSectionToolHeaderOmitsStatusWords(t *testing.T) {
	got := renderChatSection(chatEntry{
		Kind:   "tool",
		Title:  "Tool Call | list_files",
		Body:   "",
		Status: "running",
	}, 64)

	if strings.Contains(got, "running") || strings.Contains(got, "done") || strings.Contains(got, "pending") {
		t.Fatalf("expected tool header to omit status words, got %q", got)
	}
	if strings.Contains(got, "params:") || strings.Contains(got, "{\"") {
		t.Fatalf("expected tool section to hide params content, got %q", got)
	}
}

func TestAssistantDeltaPlanningTextRendersAsThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "please inspect this project", Status: "final"},
		},
		streamingIndex: -1,
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "I will first inspect structure and config, then code organization and dependencies, and finally verify with build and tests.",
	})

	if len(m.chatItems) != 2 {
		t.Fatalf("expected assistant delta to append one assistant item, got %d", len(m.chatItems))
	}
	if m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" {
		t.Fatalf("expected planning delta to render as thinking, got %+v", m.chatItems[1])
	}
}

func TestFinishAssistantMessageAppendsFinalCardAfterThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "what project is this", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "let me inspect the repo structure first", Status: "thinking"},
		},
		streamingIndex: 1,
	}

	m.finishAssistantMessage("This is a Go TUI project.")

	if len(m.chatItems) != 3 {
		t.Fatalf("expected final answer to be appended after thinking, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" {
		t.Fatalf("expected thinking card to remain visible, got %+v", m.chatItems[1])
	}
	if m.chatItems[2].Title != assistantLabel || m.chatItems[2].Status != "final" || m.chatItems[2].Body != "This is a Go TUI project." {
		t.Fatalf("expected final assistant card after thinking, got %+v", m.chatItems[2])
	}
}
