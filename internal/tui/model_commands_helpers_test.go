package tui

import (
	"strings"
	"testing"
	"time"

	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/mention"
	"bytemind/internal/provider"

	"github.com/charmbracelet/bubbles/textarea"
)

func TestOpenCommandPalettePrefillsSlashAndClosesMention(t *testing.T) {
	input := textarea.New()
	input.SetValue("draft message")

	m := model{
		input:         input,
		mentionOpen:   true,
		mentionCursor: 2,
		mentionResults: []mention.Candidate{
			{Path: "README.md"},
		},
	}

	m.openCommandPalette()

	if !m.commandOpen {
		t.Fatalf("expected command palette to open")
	}
	if m.commandCursor != 0 {
		t.Fatalf("expected command cursor reset, got %d", m.commandCursor)
	}
	if m.input.Value() != "/" {
		t.Fatalf("expected input to be prefilled with slash, got %q", m.input.Value())
	}
	if m.mentionOpen || len(m.mentionResults) != 0 {
		t.Fatalf("expected mention palette to be closed when opening command palette")
	}
}

func TestTrimPromptSearchQueryUpdatesMatches(t *testing.T) {
	m := model{
		promptSearchQuery: "fixx",
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "fix failing tests"},
			{Prompt: "write docs"},
		},
	}
	m.refreshPromptSearchMatches()
	if len(m.promptSearchMatches) != 0 {
		t.Fatalf("expected no matches for query fixx")
	}

	m.trimPromptSearchQuery()

	if m.promptSearchQuery != "fix" {
		t.Fatalf("expected query trim to remove one rune, got %q", m.promptSearchQuery)
	}
	if len(m.promptSearchMatches) != 1 {
		t.Fatalf("expected one match after trim, got %d", len(m.promptSearchMatches))
	}
	if !strings.Contains(m.promptSearchMatches[0].Prompt, "fix") {
		t.Fatalf("unexpected match after trim: %+v", m.promptSearchMatches[0])
	}
}

func TestVisiblePromptSearchEntriesPageUsesCursorPage(t *testing.T) {
	entries := make([]history.PromptEntry, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, history.PromptEntry{
			Prompt:    "prompt " + string(rune('a'+i)),
			SessionID: "sid",
			Timestamp: time.Unix(int64(i), 0).UTC(),
		})
	}

	m := model{
		promptSearchMatches: entries,
		promptSearchCursor:  6,
	}

	page := m.visiblePromptSearchEntriesPage()
	if len(page) != promptSearchPageSize {
		t.Fatalf("expected page size %d, got %d", promptSearchPageSize, len(page))
	}
	if page[0].Prompt != "prompt f" {
		t.Fatalf("expected page to start at prompt f, got %q", page[0].Prompt)
	}
}

func TestStartupProviderDefaults(t *testing.T) {
	if got := startupProviderDefaultBaseURL("anthropic"); got != "https://api.anthropic.com" {
		t.Fatalf("unexpected anthropic base_url: %q", got)
	}
	if got := startupProviderDefaultBaseURL("openai-compatible"); got != "https://api.openai.com/v1" {
		t.Fatalf("unexpected openai-compatible base_url: %q", got)
	}
	if got := startupProviderDefaultModel("anthropic"); got != "" {
		t.Fatalf("expected anthropic default model empty, got %q", got)
	}
	if got := startupProviderDefaultModel("openai-compatible"); got != "GPT-5.4" {
		t.Fatalf("unexpected openai-compatible default model: %q", got)
	}
}

func TestResolveStartupFieldValue(t *testing.T) {
	t.Run("explicit input wins", func(t *testing.T) {
		m := model{}
		got, err := m.resolveStartupFieldValue(startupFieldModel, " gpt-5.4-mini ")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if got != "gpt-5.4-mini" {
			t.Fatalf("unexpected explicit value: %q", got)
		}
	})

	t.Run("current config value fallback", func(t *testing.T) {
		m := model{
			cfg: config.Config{
				Provider: config.ProviderConfig{
					Model: "deepseek-chat",
				},
			},
		}
		got, err := m.resolveStartupFieldValue(startupFieldModel, "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if got != "deepseek-chat" {
			t.Fatalf("expected current model fallback, got %q", got)
		}
	})

	t.Run("type defaults to openai-compatible", func(t *testing.T) {
		m := model{}
		got, err := m.resolveStartupFieldValue(startupFieldType, "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if got != "openai-compatible" {
			t.Fatalf("unexpected type default: %q", got)
		}
	})

	t.Run("base_url default follows provider type", func(t *testing.T) {
		m := model{
			cfg: config.Config{
				Provider: config.ProviderConfig{Type: "anthropic"},
			},
		}
		got, err := m.resolveStartupFieldValue(startupFieldBaseURL, "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if got != "https://api.anthropic.com" {
			t.Fatalf("unexpected anthropic base_url default: %q", got)
		}
	})

	t.Run("anthropic model without explicit value returns error", func(t *testing.T) {
		m := model{
			cfg: config.Config{
				Provider: config.ProviderConfig{Type: "anthropic"},
			},
		}
		_, err := m.resolveStartupFieldValue(startupFieldModel, "")
		if err == nil {
			t.Fatalf("expected error for missing anthropic model")
		}
		if !strings.Contains(err.Error(), "please enter model name") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestInitializeStartupGuideDefaultsToTypeField(t *testing.T) {
	m := model{
		input: textarea.New(),
		startupGuide: StartupGuide{
			CurrentField: "unknown",
		},
	}

	m.initializeStartupGuide()

	if m.startupGuide.CurrentField != startupFieldType {
		t.Fatalf("expected startup field to default to type, got %q", m.startupGuide.CurrentField)
	}
	if !strings.Contains(m.startupGuide.Status, "Step 1/4") {
		t.Fatalf("expected startup status to include step progress, got %q", m.startupGuide.Status)
	}
}

func TestStartupGuideIssueHintMappings(t *testing.T) {
	cases := []struct {
		name   string
		check  provider.Availability
		expect string
	}{
		{
			name:   "missing key",
			check:  provider.Availability{Reason: "missing api key"},
			expect: "No API key is configured yet.",
		},
		{
			name:   "unauthorized",
			check:  provider.Availability{Reason: "unauthorized"},
			expect: "The API key was rejected by the provider.",
		},
		{
			name:   "network",
			check:  provider.Availability{Reason: "failed to reach endpoint"},
			expect: "Cannot reach provider endpoint. Check proxy or network.",
		},
		{
			name:   "not found",
			check:  provider.Availability{Reason: "not found"},
			expect: "Provider endpoint path looks incorrect.",
		},
		{
			name:   "empty reason",
			check:  provider.Availability{},
			expect: "Provider check failed.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startupGuideIssueHint(tc.check); got != tc.expect {
				t.Fatalf("unexpected issue hint: got %q want %q", got, tc.expect)
			}
		})
	}
}

func TestRenderPromptSearchPaletteStates(t *testing.T) {
	t.Run("empty state shows no matches hint", func(t *testing.T) {
		m := model{
			width:            120,
			screen:           screenChat,
			promptSearchMode: promptSearchModePanel,
		}
		got := m.renderPromptSearchPalette()
		if !strings.Contains(got, "Prompt history panel") {
			t.Fatalf("expected panel header, got %q", got)
		}
		if !strings.Contains(got, "No matching prompts.") {
			t.Fatalf("expected empty state text, got %q", got)
		}
	})

	t.Run("non-empty state renders prompt rows", func(t *testing.T) {
		m := model{
			width:             120,
			screen:            screenChat,
			promptSearchMode:  promptSearchModeQuick,
			promptSearchQuery: "layout",
			promptSearchMatches: []history.PromptEntry{
				{
					Prompt:    "fix layout overflow in status bar",
					Workspace: "repo",
					SessionID: "abc123",
					Timestamp: time.Now().UTC(),
				},
			},
		}
		got := m.renderPromptSearchPalette()
		if !strings.Contains(got, "fix layout overflow") {
			t.Fatalf("expected prompt row to render, got %q", got)
		}
		if !strings.Contains(got, "query:layout") {
			t.Fatalf("expected query metadata in palette, got %q", got)
		}
	})
}
