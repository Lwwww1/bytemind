package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/mention"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAtOpensMentionPaletteWithPrefilledToken(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen: screenChat,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
		}, 0, false),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	updated := got.(model)

	if !updated.mentionOpen {
		t.Fatalf("expected @ to open mention palette")
	}
	if updated.input.Value() != "@" {
		t.Fatalf("expected main input to keep @ token, got %q", updated.input.Value())
	}
	if len(updated.mentionResults) == 0 {
		t.Fatalf("expected mention palette to return candidates")
	}
}

func TestMentionPaletteFiltersAsUserTypes(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		input: input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
			{Path: "README.md", BaseName: "README.md"},
		}, 0, false),
	}

	m.syncInputOverlays()

	if !m.mentionOpen {
		t.Fatalf("expected mention palette to stay open for @mod")
	}
	if len(m.mentionResults) != 1 || m.mentionResults[0].Path != "internal/tui/model.go" {
		t.Fatalf("expected @mod to only match internal/tui/model.go, got %+v", m.mentionResults)
	}
}

func TestMentionPaletteEnterInsertsMentionInsteadOfSubmitting(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		screen: screenLanding,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
			{Path: "README.md", BaseName: "README.md"},
		}, 0, false),
	}
	m.syncInputOverlays()

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd != nil {
		t.Fatalf("expected Enter on mention selection to avoid submit command")
	}
	if updated.input.Value() != "@internal/tui/model.go " {
		t.Fatalf("expected mention selection to rewrite input, got %q", updated.input.Value())
	}
	if updated.mentionOpen {
		t.Fatalf("expected mention palette to close after inserting a file")
	}
	if len(updated.chatItems) != 0 {
		t.Fatalf("expected mention insertion to avoid sending message")
	}
	if updated.mentionRecent["internal/tui/model.go"] <= 0 {
		t.Fatalf("expected selected mention to be recorded as recent")
	}
}

func TestMentionPaletteEscClosesWithoutResettingInput(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		screen: screenChat,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
		}, 0, false),
	}
	m.syncInputOverlays()

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := got.(model)

	if updated.mentionOpen {
		t.Fatalf("expected Esc to close mention palette")
	}
	if updated.input.Value() != "@mod" {
		t.Fatalf("expected Esc to keep typed mention token, got %q", updated.input.Value())
	}
}

func TestMentionPaletteEnterWithoutCandidatesFallsBackToSubmit(t *testing.T) {
	input := textarea.New()
	input.SetValue("@unknown")
	m := model{
		screen: screenLanding,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "README.md", BaseName: "README.md"},
		}, 0, false),
	}
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette to open for unmatched query")
	}
	if len(m.mentionResults) != 0 {
		t.Fatalf("expected no candidates for @unknown, got %+v", m.mentionResults)
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected Enter with no mention candidates to submit prompt")
	}
	if updated.screen != screenChat {
		t.Fatalf("expected fallback Enter flow to switch to chat screen")
	}
	if updated.mentionOpen {
		t.Fatalf("expected mention palette to close during fallback submit")
	}
}

func TestMentionPaletteTabInsertsMentionWithoutTogglingMode(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		screen: screenChat,
		mode:   modeBuild,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go", TypeTag: "go"},
		}, 0, false),
	}
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette to open")
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	updated := got.(model)

	if updated.mode != modeBuild {
		t.Fatalf("expected tab in mention palette not to toggle mode, got %q", updated.mode)
	}
	if updated.input.Value() != "@internal/tui/model.go " {
		t.Fatalf("expected Tab to insert mention, got %q", updated.input.Value())
	}
}

func TestMentionPaletteEnterImageCandidateKeepsMentionTextAndBindsAsset(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	if err := os.WriteFile(filepath.Join(m.workspace, "2.1.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	m.input.SetValue("@2.1")
	m.mentionIndex = mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
		{Path: "2.1.jpg", BaseName: "2.1.jpg", TypeTag: "jpg"},
	}, 0, false)
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette to open")
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if cmd != nil {
		t.Fatalf("expected image mention selection not to submit")
	}
	if updated.input.Value() != "@2.1.jpg " {
		t.Fatalf("expected image mention to keep @path text, got %q", updated.input.Value())
	}
	if updated.mentionOpen {
		t.Fatalf("expected mention palette to close after image selection")
	}
	if !strings.Contains(updated.statusNote, "Attached image") {
		t.Fatalf("expected attached image note, got %q", updated.statusNote)
	}
	if len(updated.sess.Conversation.Assets.Images) != 1 {
		t.Fatalf("expected image metadata to be stored, got %d", len(updated.sess.Conversation.Assets.Images))
	}
	key := normalizeImageMentionPath("2.1.jpg")
	if strings.TrimSpace(string(updated.inputImageMentions[key])) == "" {
		t.Fatalf("expected mention image binding for key %q", key)
	}
}

func TestMentionPaletteRecentSelectionRanksFirstOnEmptyQuery(t *testing.T) {
	input := textarea.New()
	input.SetValue("@")
	m := model{
		screen: screenChat,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "alpha.go", BaseName: "alpha.go", TypeTag: "go"},
			{Path: "beta.go", BaseName: "beta.go", TypeTag: "go"},
		}, 0, false),
		mentionRecent: map[string]int{"beta.go": 99},
	}
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette for empty query")
	}
	if len(m.mentionResults) < 2 {
		t.Fatalf("expected at least two mention results")
	}
	if m.mentionResults[0].Path != "beta.go" {
		t.Fatalf("expected recent file beta.go first, got %q", m.mentionResults[0].Path)
	}
}

func TestRenderMentionPaletteShowsTruncatedMeta(t *testing.T) {
	index := mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
		{Path: "a.go", BaseName: "a.go", TypeTag: "go"},
		{Path: "b.go", BaseName: "b.go", TypeTag: "go"},
	}, 2, true)

	m := model{
		screen:      screenChat,
		width:       100,
		mentionOpen: true,
		mentionResults: []mention.Candidate{
			{Path: "a.go", BaseName: "a.go", TypeTag: "go"},
			{Path: "b.go", BaseName: "b.go", TypeTag: "go"},
		},
		mentionIndex: index,
	}

	view := m.renderMentionPalette()
	if !strings.Contains(view, "indexed first 2 files") {
		t.Fatalf("expected mention palette to show truncation hint, got %q", view)
	}
}

func TestCommandPaletteAllowsTypingJKWhenOpen(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/")
	m := model{
		screen:        screenChat,
		commandOpen:   true,
		commandCursor: 1,
		input:         input,
	}

	afterK, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	kModel := afterK.(model)
	if kModel.input.Value() != "/k" {
		t.Fatalf("expected k to be inserted into slash input, got %q", kModel.input.Value())
	}

	afterJ, _ := kModel.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	jModel := afterJ.(model)
	if jModel.input.Value() != "/kj" {
		t.Fatalf("expected j to be inserted into slash input, got %q", jModel.input.Value())
	}
}

func TestRenderCommandPaletteDoesNotCorruptChineseDescriptions(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{
		screen:      screenChat,
		width:       80,
		input:       input,
		commandOpen: true,
	}
	m.syncCommandPalette()

	got := m.renderCommandPalette()
	if strings.Contains(got, string('\uFFFD')) {
		t.Fatalf("expected command palette not to contain replacement glyphs, got %q", got)
	}
	for _, want := range []string{"/help", "/session", "/new"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected command palette to contain %q, got %q", want, got)
		}
	}
}
