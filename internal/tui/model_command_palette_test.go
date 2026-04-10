package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCommandPaletteListsQuitCommand(t *testing.T) {
	found := false
	for _, item := range commandItems {
		if item.Name == "/quit" && item.Kind == "command" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected command palette to include /quit")
	}
}

func TestCommandPaletteDoesNotListExitAlias(t *testing.T) {
	for _, item := range commandItems {
		if item.Name == "/exit" {
			t.Fatalf("did not expect command palette to include /exit")
		}
	}
}

func TestCommandPaletteDoesNotListPlanCommands(t *testing.T) {
	for _, item := range commandItems {
		if strings.HasPrefix(item.Name, "/plan") || item.Group == "plan" {
			t.Fatalf("did not expect command palette to include plan item %+v", item)
		}
	}
}

func TestSlashOpensCommandPaletteWithPrefilledSlash(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen: screenChat,
		input:  input,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated := got.(model)

	if !updated.commandOpen {
		t.Fatalf("expected slash to open command palette")
	}
	if updated.input.Value() != "/" {
		t.Fatalf("expected main input to start with '/', got %q", updated.input.Value())
	}
}

func TestFilteredCommandsShowsRootSelectorGroups(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{input: input}

	items := m.filteredCommands()
	usages := make([]string, 0, len(items))
	for _, item := range items {
		usages = append(usages, item.Usage)
	}

	for _, want := range []string{"/help", "/session", "/new", "/compact", "/quit"} {
		if !containsString(usages, want) {
			t.Fatalf("expected root selector to contain %q, got %v", want, usages)
		}
	}
	for _, unwanted := range []string{"/sessions [limit]", "/resume <id>", "/plan", "/plan add <step>"} {
		if containsString(usages, unwanted) {
			t.Fatalf("did not expect root selector to contain %q", unwanted)
		}
	}
}

func TestHandleSlashCompactCompactsSession(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("first ask"),
		llm.NewAssistantTextMessage(strings.Repeat("history details ", 30)),
		llm.NewUserTextMessage("second ask"),
		llm.NewAssistantTextMessage(strings.Repeat("more details ", 30)),
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &compactCommandTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: keep building\nDone: reviewed history\nPending: continue coding"},
		},
	}
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
			Stream:   false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		screen:    screenChat,
	}
	if err := m.handleSlashCommand("/compact"); err != nil {
		t.Fatalf("expected /compact to succeed, got %v", err)
	}
	if m.statusNote != "Conversation compacted." {
		t.Fatalf("expected compacted status note, got %q", m.statusNote)
	}
	if len(sess.Messages) != 1 || sess.Messages[0].Role != llm.RoleAssistant {
		t.Fatalf("expected compacted session summary message, got %#v", sess.Messages)
	}
	if !strings.Contains(sess.Messages[0].Text(), "Goal: keep building") {
		t.Fatalf("expected persisted summary content, got %q", sess.Messages[0].Text())
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one compaction LLM request, got %d", len(client.requests))
	}
}

func TestHandleSlashSessionOpensSessionsModal(t *testing.T) {
	m := model{}

	if err := m.handleSlashCommand("/session"); err != nil {
		t.Fatalf("expected /session to succeed, got %v", err)
	}
	if !m.sessionsOpen {
		t.Fatalf("expected /session to open sessions modal")
	}
}

func TestHandleSlashSkillsListsDiscoveredSkills(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"review skill"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		screen:    screenChat,
	}
	if err := m.handleSlashCommand("/skills"); err != nil {
		t.Fatalf("expected /skills to succeed, got %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected /skills command exchange in chat, got %#v", m.chatItems)
	}
	if !strings.Contains(m.chatItems[len(m.chatItems)-1].Body, "review") {
		t.Fatalf("expected skills output to contain review, got %q", m.chatItems[len(m.chatItems)-1].Body)
	}
}

func TestHandleSlashSkillActivateAndClear(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "bug-investigation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "bug-investigation", "skill.json"), []byte(`{
  "name":"bug-investigation",
  "description":"bug skill",
  "entry":{"slash":"/bug-investigation"},
  "tools":{"policy":"allowlist","items":["read_file"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"review skill",
  "tools":{"policy":"allowlist","items":["read_file"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		screen:    screenChat,
	}
	if err := m.handleSlashCommand("/bug-investigation"); err != nil {
		t.Fatalf("expected /bug-investigation to succeed, got %v", err)
	}
	if m.sess.ActiveSkill == nil || m.sess.ActiveSkill.Name != "bug-investigation" {
		t.Fatalf("expected bug-investigation active before switch, got %#v", m.sess.ActiveSkill)
	}
	if err := m.handleSlashCommand("/review severity=high"); err != nil {
		t.Fatalf("expected /review to succeed, got %v", err)
	}
	if m.sess.ActiveSkill == nil || m.sess.ActiveSkill.Name != "review" {
		t.Fatalf("expected active skill to be set, got %#v", m.sess.ActiveSkill)
	}
	if got := m.sess.ActiveSkill.Args["severity"]; got != "high" {
		t.Fatalf("expected skill arg severity=high, got %q", got)
	}
	if err := m.handleSlashCommand("/skill clear"); err != nil {
		t.Fatalf("expected /skill clear to succeed, got %v", err)
	}
	if m.sess.ActiveSkill != nil {
		t.Fatalf("expected active skill to be cleared, got %#v", m.sess.ActiveSkill)
	}
}

func TestFilteredCommandsIncludeSkillSlashCommands(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"review skill",
  "entry":{"slash":"/review"}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	input := textarea.New()
	input.SetValue("/re")
	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		input:     input,
	}

	items := m.filteredCommands()
	found := false
	for _, item := range items {
		if item.Name == "/review" && item.Kind == "skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /review skill command in filtered commands, got %+v", items)
	}
}

func TestCommandPaletteFiltersAsUserTypes(t *testing.T) {
	input := textarea.New()
	input.SetValue("/h")
	m := model{input: input}

	items := m.filteredCommands()
	if len(items) != 1 || items[0].Name != "/help" {
		t.Fatalf("expected /h to only show /help, got %+v", items)
	}
}

func TestEscapeClosesCommandPalette(t *testing.T) {
	input := textarea.New()
	input.SetValue("/h")
	m := model{
		screen:      screenChat,
		commandOpen: true,
		input:       input,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := got.(model)

	if updated.commandOpen {
		t.Fatalf("expected esc to close command palette")
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected main input to reset after esc, got %q", updated.input.Value())
	}
}

func TestCommandPaletteEnterOnQuitReturnsQuitCmd(t *testing.T) {
	input := textarea.New()
	input.SetValue("/quit")
	m := model{
		screen:      screenChat,
		commandOpen: true,
		input:       input,
	}
	m.syncCommandPalette()

	_, cmd := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected /quit from command palette to return a quit command")
	}
}

func TestCommandPaletteBusyPlainTextQueuesBTW(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("focus only on unit tests")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:      screenChat,
		commandOpen: true,
		busy:        true,
		input:       input,
		runCancel:   func() { canceled = true },
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
	}

	got, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if !canceled {
		t.Fatalf("expected command palette busy submit to cancel active run")
	}
	if updated.commandOpen {
		t.Fatalf("expected command palette to close after busy plain-text submit")
	}
	if len(updated.pendingBTW) != 1 || updated.pendingBTW[0] != "focus only on unit tests" {
		t.Fatalf("expected plain text to queue as btw, got %#v", updated.pendingBTW)
	}
	if !updated.interrupting {
		t.Fatalf("expected busy plain-text submit to enter interrupting state")
	}
}

func TestViewRendersCommandPaletteAsOverlaySection(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{
		screen:      screenChat,
		width:       100,
		height:      30,
		input:       input,
		commandOpen: true,
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
		cfg: config.Config{
			Provider:       config.ProviderConfig{Type: "openai-compatible", Model: "deepseek-chat"},
			ApprovalPolicy: "on-request",
			MaxIterations:  32,
		},
	}
	m.syncCommandPalette()

	view := m.View()
	if !strings.Contains(view, "/help") {
		t.Fatalf("expected slash command overlay to render, got %q", view)
	}
	if strings.Contains(view, "Conversation") {
		t.Fatalf("did not expect redundant conversation header in chat view")
	}
}

func TestLandingViewRendersCommandPaletteAboveInput(t *testing.T) {
	input := textarea.New()
	input.SetValue("/h")
	m := model{
		screen:      screenLanding,
		width:       100,
		height:      30,
		input:       input,
		commandOpen: true,
	}
	m.syncCommandPalette()

	view := m.View()
	if !strings.Contains(view, "Build") || !strings.Contains(view, "Plan") {
		t.Fatalf("expected landing view to remain visible, got %q", view)
	}
	if !strings.Contains(view, "/help") {
		t.Fatalf("expected landing slash menu to render, got %q", view)
	}
}

func TestCommandPaletteUsesCompactThreeRowList(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{
		screen:      screenChat,
		width:       100,
		height:      30,
		input:       input,
		commandOpen: true,
	}

	m.syncCommandPalette()

	if len(m.visibleCommandItemsPage()) != 3 {
		t.Fatalf("expected command palette list height 3, got %d", len(m.visibleCommandItemsPage()))
	}
}

func TestCommandPaletteSupportsPageNavigation(t *testing.T) {
	original := commandItems
	commandItems = []commandItem{
		{Name: "/a", Usage: "/a", Description: "a"},
		{Name: "/b", Usage: "/b", Description: "b"},
		{Name: "/c", Usage: "/c", Description: "c"},
		{Name: "/d", Usage: "/d", Description: "d"},
		{Name: "/e", Usage: "/e", Description: "e"},
	}
	defer func() { commandItems = original }()

	m := model{
		commandOpen: true,
		input: func() textarea.Model {
			input := textarea.New()
			input.SetValue("/")
			return input
		}(),
	}
	m.syncCommandPalette()

	afterDown, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyPgDown})
	downModel := afterDown.(model)
	if downModel.commandCursor != 3 {
		t.Fatalf("expected pgdown to move to next command page, got cursor %d", downModel.commandCursor)
	}
	page := downModel.visibleCommandItemsPage()
	if len(page) == 0 || page[0].Name != "/d" {
		t.Fatalf("expected second page to start with /d, got %+v", page)
	}

	afterUp, _ := downModel.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyPgUp})
	upModel := afterUp.(model)
	if upModel.commandCursor != 0 {
		t.Fatalf("expected pgup to move back to first command page, got cursor %d", upModel.commandCursor)
	}
}
