package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

const (
	defaultSessionLimit        = 8
	scrollStep                 = 3
	scrollbarWidth             = 1
	mouseZoneAutoProbeMaxDelta = 4
	commandPageSize            = 3
	mentionPageSize            = 5
	maxPendingBTW              = 5
	promptSearchPageSize       = 5
	promptSearchLoadLimit      = 50000
	promptSearchResultCap      = 200
	pasteSubmitGuard           = 400 * time.Millisecond
	mouseSelectionScrollTick   = 60 * time.Millisecond
	assistantLabel             = "Bytemind"
	thinkingLabel              = "Bytemind"
	chatTitleLabel             = "Bytemind Chat"
	tuiTitleLabel              = "Bytemind TUI"
	footerHintText             = "tab agents | / commands | drag select | Ctrl+C copy/quit | Ctrl+F history | Ctrl+L sessions"
	conversationViewportZoneID = "bytemind:conversation:viewport"
	inputEditorZoneID          = "bytemind:input:editor"
)

type footerShortcutHint struct {
	Key   string
	Label string
}

var footerShortcutHints = []footerShortcutHint{
	{Key: "tab", Label: "agents"},
	{Key: "/", Label: "commands"},
	{Key: "Ctrl+F", Label: "history"},
	{Key: "Ctrl+L", Label: "sessions"},
	{Key: "Ctrl+C", Label: "copy/quit"},
}

var promptSearchFilterHints = []footerShortcutHint{
	{Key: "ws:<kw>", Label: "workspace"},
	{Key: "sid:<kw>", Label: "session"},
}

var promptSearchActionHints = []footerShortcutHint{
	{Key: "PgUp/PgDn", Label: "page"},
	{Key: "Ctrl+F", Label: "next"},
	{Key: "Ctrl+S", Label: "prev"},
	{Key: "Enter", Label: "apply"},
	{Key: "Esc", Label: "close"},
}

var zoneInitOnce sync.Once

type screenKind string

const (
	screenLanding screenKind = "landing"
	screenChat    screenKind = "chat"
)

type agentMode string

const (
	modeBuild agentMode = "build"
	modePlan  agentMode = "plan"
)

type promptSearchMode string

const (
	promptSearchModeQuick promptSearchMode = "quick"
	promptSearchModePanel promptSearchMode = "panel"
)

const (
	startupFieldType    = "type"
	startupFieldBaseURL = "base_url"
	startupFieldModel   = "model"
	startupFieldAPIKey  = "api_key"
)

var startupFieldOrder = []string{
	startupFieldType,
	startupFieldBaseURL,
	startupFieldModel,
	startupFieldAPIKey,
}

type chatEntry struct {
	Kind   string
	Title  string
	Meta   string
	Body   string
	Status string
}

type viewportSelectionPoint struct {
	Col int
	Row int
}

type viewportTopLookupCache struct {
	left           int
	expectedTop    int
	viewportWidth  int
	viewportHeight int
	viewportOffset int
	top            int
	found          bool
	valid          bool
}

type commandItem struct {
	Name        string
	Usage       string
	Description string
	Group       string
	Kind        string
}

func (c commandItem) FilterValue() string {
	return strings.ToLower(strings.TrimPrefix(c.Usage, "/") + " " + c.Description)
}

type toolRun struct {
	Name    string
	Summary string
	Lines   []string
	Status  string
}

type approvalPrompt struct {
	Command string
	Reason  string
	Reply   chan approvalDecision
}

type approvalDecision struct {
	Approved bool
	Err      error
}

type agentEventMsg struct {
	Event agent.Event
}

type runFinishedMsg struct {
	RunID int
	Err   error
}

type runFinishReason string

const (
	runFinishReasonCompleted  runFinishReason = "completed"
	runFinishReasonFailed     runFinishReason = "failed"
	runFinishReasonCanceled   runFinishReason = "canceled"
	runFinishReasonBTWRestart runFinishReason = "btw_restart"
)

type approvalRequestMsg struct {
	Request tools.ApprovalRequest
	Reply   chan approvalDecision
}

type sessionsLoadedMsg struct {
	Summaries []session.Summary
	Err       error
}

type tokenUsagePulledMsg struct {
	Used    int
	Input   int
	Output  int
	Context int
	Err     error
}

type selectionToastExpiredMsg struct {
	ID int
}

type mouseSelectionScrollTickMsg struct {
	ID int
}

var commandItems = []commandItem{
	{Name: "/help", Usage: "/help", Description: "Show usage and supported commands.", Kind: "command"},
	{Name: "/session", Usage: "/session", Description: "Open the recent session list.", Kind: "command"},
	{Name: "/new", Usage: "/new", Description: "Start a fresh session in this workspace.", Kind: "command"},
	{Name: "/compact", Usage: "/compact", Description: "Compress long session history into a continuation summary.", Kind: "command"},
	{Name: "/btw", Usage: "/btw <message>", Description: "Interject while a run is in progress.", Kind: "command"},
	{Name: "/quit", Usage: "/quit", Description: "Exit the current TUI window.", Kind: "command"},
	{Name: "/skills", Usage: "/skills", Description: "List available skills and current active skill.", Kind: "command"},
	{Name: "/skill clear", Usage: "/skill clear", Description: "Clear active skill for this session.", Kind: "command"},
}

type model struct {
	runner     *agent.Runner
	store      *session.Store
	sess       *session.Session
	imageStore assets.ImageStore
	cfg        config.Config
	workspace  string

	width  int
	height int

	async    chan tea.Msg
	viewport viewport.Model
	copyView viewport.Model
	planView viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	viewportContentCache  string
	viewportTopCache      viewportTopLookupCache
	chatItems             []chatEntry
	toolRuns              []toolRun
	plan                  planpkg.State
	sessions              []session.Summary
	sessionLimit          int
	sessionCursor         int
	commandCursor         int
	mentionCursor         int
	screen                screenKind
	mode                  agentMode
	sessionsOpen          bool
	helpOpen              bool
	commandOpen           bool
	mentionOpen           bool
	promptSearchOpen      bool
	busy                  bool
	streamingIndex        int
	statusNote            string
	phase                 string
	llmConnected          bool
	approval              *approvalPrompt
	mentionQuery          string
	mentionToken          mention.Token
	mentionResults        []mention.Candidate
	mentionIndex          *mention.WorkspaceFileIndex
	mentionRecent         map[string]int
	mentionSeq            int
	lastPasteAt           time.Time
	lastInputAt           time.Time
	inputBurstSize        int
	chatAutoFollow        bool
	draggingScrollbar     bool
	scrollbarDragOffset   int
	mouseSelecting        bool
	mouseSelectionMouseX  int
	mouseSelectionMouseY  int
	mouseSelectionTickID  int
	mouseSelectionActive  bool
	mouseSelectionStart   viewportSelectionPoint
	mouseSelectionEnd     viewportSelectionPoint
	inputMouseSelecting   bool
	inputSelectionActive  bool
	inputSelectionStart   viewportSelectionPoint
	inputSelectionEnd     viewportSelectionPoint
	selectionToast        string
	selectionToastID      int
	tokenUsage            tokenUsageComponent
	tokenUsedTotal        int
	tokenBudget           int
	tokenInput            int
	tokenOutput           int
	tokenContext          int
	tokenHasOfficialUsage bool
	tempEstimatedOutput   int
	tokenEstimator        *realtimeTokenEstimator
	promptHistoryLoaded   bool
	promptHistoryEntries  []history.PromptEntry
	promptSearchMode      promptSearchMode
	promptSearchQuery     string
	promptSearchMatches   []history.PromptEntry
	promptSearchCursor    int
	promptSearchBaseInput string
	inputImageRefs        map[int]llm.AssetID
	inputImageMentions    map[string]llm.AssetID
	orphanedImages        map[llm.AssetID]time.Time
	nextImageID           int
	pastedContents        map[string]pastedContent
	pastedOrder           []string
	nextPasteID           int
	pastedStateLoaded     bool
	lastCompressedPasteAt time.Time
	clipboard             clipboardImageReader
	clipboardText         clipboardTextWriter
	runCancel             context.CancelFunc
	pendingBTW            []string
	interrupting          bool
	interruptSafe         bool
	runSeq                int
	activeRunID           int
	startupGuide          StartupGuide
	mouseYOffset          int
}

func newModel(opts Options) model {
	ensureZoneManager()
	async := make(chan tea.Msg, 128)

	input := textarea.New()
	input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	input.Focus()
	input.CharLimit = 0
	input.SetWidth(72)
	input.SetHeight(2)
	input.ShowLineNumbers = false
	input.Prompt = ""

	spin := spinner.New()
	spin.Spinner = spinner.MiniDot
	spin.Style = lipgloss.NewStyle().Foreground(colorAccent)

	vp := viewport.New(0, 0)
	vp.YPosition = 0
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = scrollStep

	copyVP := viewport.New(0, 0)
	copyVP.YPosition = 0

	planVP := viewport.New(0, 0)
	planVP.YPosition = 0
	planVP.MouseWheelEnabled = true
	planVP.MouseWheelDelta = scrollStep

	chatItems, toolRuns := rebuildSessionTimeline(opts.Session)

	opts.Runner.SetObserver(agent.ObserverFunc(func(event agent.Event) {
		async <- agentEventMsg{Event: event}
	}))
	opts.Runner.SetApprovalHandler(func(req tools.ApprovalRequest) (bool, error) {
		reply := make(chan approvalDecision, 1)
		async <- approvalRequestMsg{Request: req, Reply: reply}
		decision := <-reply
		return decision.Approved, decision.Err
	})

	m := model{
		runner:             opts.Runner,
		store:              opts.Store,
		sess:               opts.Session,
		imageStore:         opts.ImageStore,
		cfg:                opts.Config,
		workspace:          opts.Workspace,
		async:              async,
		viewport:           vp,
		copyView:           copyVP,
		planView:           planVP,
		input:              input,
		spinner:            spin,
		chatItems:          chatItems,
		toolRuns:           toolRuns,
		plan:               copyPlanState(opts.Session.Plan),
		sessions:           nil,
		sessionLimit:       defaultSessionLimit,
		screen:             initialScreen(opts.Session),
		mode:               toAgentMode(opts.Session.Mode),
		streamingIndex:     -1,
		statusNote:         "Ready.",
		phase:              "idle",
		llmConnected:       true,
		chatAutoFollow:     true,
		mentionIndex:       mention.NewWorkspaceFileIndex(opts.Workspace),
		tokenUsage:         newTokenUsageComponent(),
		tokenBudget:        max(1, opts.Config.TokenQuota),
		tokenEstimator:     newRealtimeTokenEstimator(opts.Config.Provider.Model),
		inputImageRefs:     make(map[int]llm.AssetID, 8),
		inputImageMentions: make(map[string]llm.AssetID, 8),
		orphanedImages:     make(map[llm.AssetID]time.Time, 8),
		nextImageID:        nextSessionImageID(opts.Session),
		pastedContents:     make(map[string]pastedContent, maxStoredPastedContents),
		pastedOrder:        make([]string, 0, maxStoredPastedContents),
		nextPasteID:        1,
		clipboard:          defaultClipboardImageReader{},
		clipboardText:      defaultClipboardTextWriter{},
		startupGuide:       opts.StartupGuide,
		mouseYOffset:       resolveMouseYOffset(),
	}
	if opts.StartupGuide.Active {
		m.statusNote = opts.StartupGuide.Status
		m.llmConnected = false
		m.phase = "error"
		m.initializeStartupGuide()
	}
	m.restoreTokenUsageFromSession(opts.Session)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.ensureSessionImageAssets()
	m.ensurePastedContentState()
	m.syncInputStyle()
	m.syncInputOverlays()
	if m.mentionIndex != nil {
		go m.mentionIndex.Prewarm()
	}
	return m
}

func ensureZoneManager() {
	zoneInitOnce.Do(func() {
		zone.NewGlobal()
	})
}

func resolveMouseYOffset() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MOUSE_Y_OFFSET"))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return clamp(value, -10, 10)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		waitForAsync(m.async),
		m.tokenUsage.tickCmd(),
		m.loadSessionsCmd(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.updateThinkingCard()
		m.refreshViewport()
		return m, cmd
	case agentEventMsg:
		m.handleAgentEvent(msg.Event)
		m.refreshViewport()
		return m, waitForAsync(m.async)
	case runFinishedMsg:
		if msg.RunID > 0 && msg.RunID != m.activeRunID {
			return m, waitForAsync(m.async)
		}
		m.busy = false
		m.runCancel = nil
		m.activeRunID = 0
		m.interruptSafe = false
		shouldResumeBTW := m.interrupting && len(m.pendingBTW) > 0
		m.interrupting = false
		finishReason := classifyRunFinish(msg.Err, shouldResumeBTW)
		if shouldResumeBTW {
			updateScope := formatBTWUpdateScope(len(m.pendingBTW))
			prompt := composeBTWPrompt(m.pendingBTW)
			m.pendingBTW = nil
			note := fmt.Sprintf("BTW accepted. Restarting with %s...", updateScope)
			if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
				note = fmt.Sprintf("Previous run ended early. Restarting with %s from BTW...", updateScope)
			}
			m.appendChat(chatEntry{
				Kind:   "system",
				Title:  "System",
				Body:   fmt.Sprintf("BTW interrupt accepted. Restarting with %s.", updateScope),
				Status: "final",
			})
			return m, m.beginRun(prompt, string(m.mode), note)
		}
		m.pendingBTW = nil
		switch finishReason {
		case runFinishReasonCompleted:
			if !m.shouldKeepStreamingIndexOnRunFinished() {
				m.streamingIndex = -1
			}
			m.statusNote = "Ready."
			m.phase = "idle"
		case runFinishReasonCanceled:
			m.streamingIndex = -1
			m.statusNote = "Run canceled."
			m.phase = "idle"
			m.llmConnected = true
		case runFinishReasonFailed:
			m.streamingIndex = -1
			m.statusNote = "Run failed: " + msg.Err.Error()
			m.phase = "error"
			m.llmConnected = false
			m.failLatestAssistant(msg.Err.Error())
		default:
			m.streamingIndex = -1
			m.statusNote = "Ready."
			m.phase = "idle"
		}
		m.refreshViewport()
		return m, tea.Batch(waitForAsync(m.async), m.loadSessionsCmd())
	case approvalRequestMsg:
		m.approval = &approvalPrompt{
			Command: msg.Request.Command,
			Reason:  msg.Request.Reason,
			Reply:   msg.Reply,
		}
		m.statusNote = "Approval required."
		m.phase = "approval"
		return m, waitForAsync(m.async)
	case sessionsLoadedMsg:
		if msg.Err == nil {
			m.sessions = msg.Summaries
			if m.sessionCursor >= len(m.sessions) && len(m.sessions) > 0 {
				m.sessionCursor = len(m.sessions) - 1
			}
		}
		return m, nil
	case tokenUsagePulledMsg:
		// Account-level usage is not session-accurate; ignore in session-only mode.
		return m, nil
	case selectionToastExpiredMsg:
		if msg.ID == m.selectionToastID {
			m.selectionToast = ""
		}
		return m, nil
	case mouseSelectionScrollTickMsg:
		return m.handleMouseSelectionScrollTick(msg)
	case tokenMonitorTickMsg:
		cmd, _ := m.tokenUsage.Update(msg)
		return m, cmd
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if !m.sessionsOpen && !m.helpOpen && !m.commandOpen && m.approval == nil {
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), "")
			m.syncInputOverlays()
		}
		return m, cmd
	}

	return m, nil
}

func (m model) shouldKeepStreamingIndexOnRunFinished() bool {
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return false
	}
	item := m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(item.Status))
	return status == "streaming" || status == "thinking" || status == "pending"
}

func (m *model) ensureViewportMouse() {
	m.viewport.MouseWheelEnabled = true
	if m.viewport.MouseWheelDelta <= 0 {
		m.viewport.MouseWheelDelta = scrollStep
	}
}

func (m *model) ensurePlanMouse() {
	m.planView.MouseWheelEnabled = true
	if m.planView.MouseWheelDelta <= 0 {
		m.planView.MouseWheelDelta = scrollStep
	}
}

func normalizeKeyName(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(key)
}

func isInputNewlineKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlJ || normalizeKeyName(msg.String()) == "ctrl+j" {
		return true
	}
	if msg.Type == tea.KeyEnter && msg.Alt {
		return true
	}
	key := normalizeKeyName(msg.String())
	return key == "shift+enter" || key == "shift+return"
}

func isCtrlVPasteKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlV {
		return true
	}
	if len(msg.Runes) == 1 && msg.Runes[0] == []rune(ctrlVMarkerRune)[0] {
		return true
	}
	return normalizeKeyName(msg.String()) == "ctrl+v"
}

func inputMutationSource(msg tea.KeyMsg) string {
	source := strings.TrimSpace(msg.String())
	if !msg.Paste {
		return source
	}
	if source == "" {
		return "paste"
	}
	return source + ":paste"
}

func isClipboardNoImageNote(note string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(note)), "clipboard has no image")
}

func isPageUpKey(msg tea.KeyMsg) bool {
	key := normalizeKeyName(msg.String())
	return msg.Type == tea.KeyPgUp || key == "pgup" || key == "pageup" || key == "prior"
}

func isPageDownKey(msg tea.KeyMsg) bool {
	key := normalizeKeyName(msg.String())
	return msg.Type == tea.KeyPgDown || key == "pgdn" || key == "pgdown" || key == "pagedown" || key == "next"
}

func (m *model) scrollInput(delta int) {
	switch {
	case delta < 0:
		for i := 0; i < -delta; i++ {
			m.input.CursorUp()
		}
	case delta > 0:
		for i := 0; i < delta; i++ {
			m.input.CursorDown()
		}
	}
}

func (m model) mouseOverInput(y int) bool {
	ensureZoneManager()
	switch m.screen {
	case screenLanding:
		return m.mouseOverLandingInput(y)
	case screenChat:
		return m.mouseOverChatInput(y)
	default:
		return false
	}
}

func (m model) mouseOverPlan(x, y int) bool {
	return false
}

func (m model) mouseOverChatInput(y int) bool {
	if m.width <= 0 {
		return false
	}
	footerTop := panelStyle.GetVerticalFrameSize()/2 + lipgloss.Height(m.renderMainPanel())
	inputHeight := lipgloss.Height(
		m.inputBorderStyle().
			Width(m.chatPanelInnerWidth()).
			Render(m.input.View()),
	)
	inputTop := footerTop
	if m.approval != nil {
		inputTop += lipgloss.Height(m.renderApprovalBanner())
	}
	if m.startupGuide.Active {
		inputTop += lipgloss.Height(m.renderStartupGuidePanel())
	} else if m.promptSearchOpen {
		inputTop += lipgloss.Height(m.renderPromptSearchPalette())
	} else if m.mentionOpen {
		inputTop += lipgloss.Height(m.renderMentionPalette())
	} else if m.commandOpen {
		inputTop += lipgloss.Height(m.renderCommandPalette())
	}
	inputBottom := inputTop + max(1, inputHeight) - 1
	return y >= inputTop && y <= inputBottom
}

func (m model) mouseOverLandingInput(y int) bool {
	if m.height <= 0 {
		return false
	}
	logoHeight := lipgloss.Height(landingLogoStyle.Render(strings.Join([]string{
		"    ____        __                      _           __",
		"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
		"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
		" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
		"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
		"      /____/                                          ",
	}, "\n")))
	modeTabsHeight := lipgloss.Height(m.renderModeTabs())
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
	inputHeight := lipgloss.Height(
		landingInputStyle.Copy().
			BorderForeground(m.modeAccentColor()).
			Width(m.landingInputShellWidth()).
			Render(m.input.View()),
	)
	hintHeight := lipgloss.Height(renderFooterShortcutHints())
	contentHeight := logoHeight + 1 + modeTabsHeight + 1 + overlayHeight + inputHeight + 1 + hintHeight
	contentTop := max(0, (m.height-contentHeight)/2)
	inputTop := contentTop + logoHeight + 1 + modeTabsHeight + 1 + overlayHeight
	inputBottom := inputTop + max(1, inputHeight) - 1
	return y >= inputTop && y <= inputBottom
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.hasCopyableSelection() {
			return m, m.copyCurrentSelection()
		}
		if m.approval != nil {
			m.approval.Reply <- approvalDecision{Approved: false}
		}
		if m.runCancel != nil {
			m.runCancel()
		}
		return m, tea.Quit
	}

	if m.promptSearchOpen {
		return m.handlePromptSearchKey(msg)
	}

	switch msg.String() {
	case "esc":
		if m.hasCopyableSelection() {
			m.clearMouseSelection()
			m.clearInputSelection()
			m.statusNote = "Selection cleared."
			return m, nil
		}
	case "tab":
		if m.commandOpen || m.mentionOpen || m.sessionsOpen || m.helpOpen || m.approval != nil {
			break
		}
		m.toggleMode()
		return m, nil
	case "ctrl+g":
		if m.approval == nil {
			m.helpOpen = !m.helpOpen
		}
		return m, nil
	case "ctrl+f":
		if m.approval != nil || m.helpOpen || m.sessionsOpen || m.commandOpen || m.mentionOpen {
			return m, nil
		}
		m.openPromptSearch(promptSearchModeQuick)
		return m, nil
	}

	if m.approval != nil {
		switch msg.String() {
		case "y", "Y", "enter":
			m.approval.Reply <- approvalDecision{Approved: true}
			m.statusNote = "Shell command approved."
			m.phase = "tool"
			m.approval = nil
		case "n", "N", "esc":
			m.approval.Reply <- approvalDecision{Approved: false}
			m.statusNote = "Shell command rejected."
			m.phase = "thinking"
			m.approval = nil
		}
		return m, nil
	}

	if m.helpOpen {
		if msg.String() == "esc" || msg.String() == "ctrl+g" {
			m.helpOpen = false
		}
		return m, nil
	}

	if m.commandOpen {
		return m.handleCommandPaletteKey(msg)
	}

	if m.mentionOpen {
		return m.handleMentionPaletteKey(msg)
	}

	if m.sessionsOpen {
		switch msg.String() {
		case "esc":
			m.sessionsOpen = false
		case "up", "k":
			if m.sessionCursor > 0 {
				m.sessionCursor--
			}
		case "down", "j":
			if m.sessionCursor < len(m.sessions)-1 {
				m.sessionCursor++
			}
		case "enter":
			if m.busy || len(m.sessions) == 0 {
				return m, nil
			}
			if err := m.resumeSession(m.sessions[m.sessionCursor].ID); err != nil {
				m.statusNote = err.Error()
			} else {
				m.sessionsOpen = false
			}
		}
		return m, nil
	}

	if isInputNewlineKey(msg) {
		before := m.input.Value()
		var cmd tea.Cmd
		// Preserve multiline input shortcuts without triggering submit.
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), inputMutationSource(msg))
			m.syncInputOverlays()
		}
		return m, cmd
	}

	ctrlVPasteDetected := isCtrlVPasteKey(msg)
	// Prefer Ctrl+V image paste first. If clipboard has no image, fall through
	// so regular terminal paste behavior can continue.
	if ctrlVPasteDetected {
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
			if strings.Contains(note, "Attached image from clipboard") {
				m.syncInputOverlays()
				return m, nil
			}
			if !isClipboardNoImageNote(note) {
				m.syncInputOverlays()
				return m, nil
			}
		}
	}

	switch msg.String() {
	case "ctrl+l":
		if !m.busy {
			m.sessionsOpen = true
		}
		return m, m.loadSessionsCmd()
	case "alt+v":
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
		m.syncInputOverlays()
		return m, nil
	case "ctrl+n":
		if !m.busy && m.screen == screenChat {
			if err := m.newSession(); err != nil {
				m.statusNote = err.Error()
			}
		}
		return m, m.loadSessionsCmd()
	case "home":
		m.viewport.GotoTop()
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		m.syncCopyViewOffset()
		m.chatAutoFollow = true
		return m, nil
	}

	if msg.String() == "enter" && !msg.Paste {
		if m.shouldSuppressEnterAfterPaste() {
			if m.busy {
				return m, nil
			}
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.input.Value() != before {
				m.handleInputMutation(before, m.input.Value(), "paste-enter")
				m.syncInputOverlays()
			}
			return m, cmd
		}
		rawValue := m.input.Value()
		if markerChain, ok := extractLeadingCompressedMarker(rawValue); ok {
			tail := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawValue), markerChain))
			if tail != "" {
				if m.shouldCompressPastedText(tail, "paste-enter") {
					marker, content, err := m.compressPastedText(tail)
					if err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
					combined := strings.TrimSpace(markerChain) + marker
					m.setInputValue(combined)
					m.syncInputOverlays()
					m.statusNote = fmt.Sprintf("Detected another pasted block and compressed it as %s (%d lines). Press Enter again to send.", marker, content.Lines)
					return m, nil
				}
				if len(tail) >= 24 || strings.Contains(tail, "\n") {
					m.setInputValue(strings.TrimSpace(markerChain))
					m.syncInputOverlays()
					m.statusNote = "Detected continued paste chunk after compressed marker. Kept compressed markers only; press Enter again to send."
					return m, nil
				}
			}
		}
		// Check whether the input has already been compressed.
		isAlreadyCompressed := strings.Contains(rawValue, "[Paste #") || strings.Contains(rawValue, "[Pasted #")

		// Compress long pasted content before sending.
		if !isAlreadyCompressed && m.shouldCompressPastedText(rawValue, inputMutationSource(msg)) {
			marker, content, err := m.compressPastedText(rawValue)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			m.setInputValue(marker)
			m.syncInputOverlays()
			m.statusNote = fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Press Enter again to send. Use [Paste #%s] or [Paste #%s line10~line20] later.", content.Lines, marker, content.ID, content.ID)
			return m, nil
		}
		value := strings.TrimSpace(rawValue)
		if m.startupGuide.Active && !strings.HasPrefix(value, "/") {
			if err := m.handleStartupGuideSubmission(rawValue); err != nil {
				m.statusNote = err.Error()
			}
			m.screen = screenLanding
			return m, nil
		}
		if value == "" {
			return m, nil
		}
		if isBTWCommand(value) {
			btw, err := extractBTWText(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			if m.busy {
				return m.submitBTW(btw)
			}
			return m.submitPrompt(btw)
		}
		if value == "/quit" {
			return m, tea.Quit
		}
		if m.busy {
			if strings.HasPrefix(value, "/") {
				m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message> or plain text."
				return m, nil
			}
			return m.submitBTW(value)
		}
		if isContinueExecutionInput(value) && planpkg.HasStructuredPlan(m.plan) {
			state, err := preparePlanForContinuation(m.plan)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			m.plan = state
			m.mode = modeBuild
			if m.sess != nil {
				m.sess.Mode = planpkg.ModeBuild
				m.sess.Plan = copyPlanState(state)
				if m.store != nil {
					if err := m.store.Save(m.sess); err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
				}
			}
		}
		if strings.HasPrefix(value, "/") {
			m.input.Reset()
			next, cmd, err := m.executeCommand(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		return m.submitPrompt(value)
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	after := m.input.Value()
	mutationSource := inputMutationSource(msg)
	if after != before {
		m.handleInputMutation(before, after, mutationSource)
		after = m.input.Value()
	}
	triggerClipboardImagePaste := shouldTriggerClipboardImagePaste(before, after, mutationSource)
	if ctrlVPasteDetected {
		triggerClipboardImagePaste = false
	}
	if !triggerClipboardImagePaste && msg.Paste {
		_, inserted, _ := insertionDiff(before, after)
		cleanInserted := strings.TrimSpace(strings.ReplaceAll(inserted, ctrlVMarkerRune, ""))
		if cleanInserted == "" {
			triggerClipboardImagePaste = true
		}
	}
	if triggerClipboardImagePaste {
		if cleaned, changed := stripCtrlVMarker(m.input.Value()); changed {
			m.setInputValue(cleaned)
		}
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}
	m.syncInputOverlays()
	return m, cmd
}

func (m model) shouldSuppressEnterAfterPaste() bool {
	if m.lastPasteAt.IsZero() {
		return false
	}
	if time.Since(m.lastPasteAt) > pasteSubmitGuard {
		return false
	}
	if strings.Contains(m.input.Value(), "\n") {
		return true
	}
	return time.Since(m.lastInputAt) <= 120*time.Millisecond
}

func (m *model) toggleMode() {
	if m.mode == modeBuild {
		m.mode = modePlan
		if m.plan.Phase == planpkg.PhaseNone {
			m.plan.Phase = planpkg.PhaseDrafting
		}
		m.statusNote = "Switched to Plan mode. Draft the plan before executing."
	} else {
		m.mode = modeBuild
		m.statusNote = "Switched to Build mode. Execution still requires confirmation."
	}
	if m.sess != nil {
		m.sess.Mode = planpkg.NormalizeMode(string(m.mode))
		m.sess.Plan = copyPlanState(m.plan)
		if m.store != nil {
			_ = m.store.Save(m.sess)
		}
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}

func (m *model) noteInputMutation(before, after, source string) {
	now := time.Now()
	delta := len(after) - len(before)
	if delta < 0 {
		delta = 0
	}

	if now.Sub(m.lastInputAt) <= 80*time.Millisecond {
		m.inputBurstSize += max(1, delta)
	} else {
		m.inputBurstSize = max(1, delta)
	}
	m.lastInputAt = now

	if source == "paste-enter" ||
		source == "ctrl+v" ||
		delta > 1 ||
		strings.Contains(after[lenCommonPrefix(before, after):], "\n") ||
		m.inputBurstSize >= 4 {
		m.lastPasteAt = now
	}
}

func (m *model) handleInputMutation(before, after, source string) {
	m.noteInputMutation(before, after, source)

	updated, note := m.applyInputImagePipeline(before, after, source)
	if updated == after {
		fallbackUpdated, fallbackNote := m.applyWholeInputImagePathFallback(after, source)
		if fallbackUpdated != after {
			updated = fallbackUpdated
		}
		if strings.TrimSpace(note) == "" {
			note = fallbackNote
		}
	}

	pasteUpdated, pasteNote := m.applyLongPastedTextPipeline(before, updated, source)
	if pasteUpdated != updated {
		updated = pasteUpdated
	}
	if strings.TrimSpace(note) == "" {
		note = pasteNote
	}

	if updated != after {
		m.setInputValue(updated)
	}
	if strings.TrimSpace(note) != "" {
		m.statusNote = note
	}
}

func lenCommonPrefix(a, b string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func (m *model) beginRun(prompt, mode, note string) tea.Cmd {
	return m.beginRunWithInput(agent.RunPromptInput{
		UserMessage: llm.NewUserTextMessage(prompt),
		DisplayText: prompt,
	}, mode, note)
}

func (m *model) beginRunWithInput(promptInput agent.RunPromptInput, mode, note string) tea.Cmd {
	runCtx, cancel := context.WithCancel(context.Background())
	m.runSeq++
	runID := m.runSeq
	m.activeRunID = runID
	m.runCancel = cancel
	m.streamingIndex = -1
	if strings.TrimSpace(note) == "" {
		note = "Request sent to LLM. Waiting for response..."
	}
	m.statusNote = note
	m.phase = "thinking"
	m.llmConnected = true
	m.busy = true
	m.chatAutoFollow = true
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return tea.Batch(m.startRunCmd(runCtx, runID, promptInput, mode), m.spinner.Tick, waitForAsync(m.async))
}

func (m model) submitPrompt(value string) (tea.Model, tea.Cmd) {
	promptInput, displayText, err := m.buildPromptInput(value)
	if err != nil {
		m.statusNote = err.Error()
		return m, nil
	}

	m.input.Reset()
	m.screen = screenChat
	if m.promptHistoryLoaded {
		entry := history.PromptEntry{
			Timestamp: time.Now().UTC(),
			Workspace: strings.TrimSpace(m.workspace),
			Prompt:    strings.TrimSpace(displayText),
		}
		if m.sess != nil {
			entry.SessionID = m.sess.ID
		}
		if entry.Prompt != "" {
			m.promptHistoryEntries = append(m.promptHistoryEntries, entry)
			if len(m.promptHistoryEntries) > promptSearchLoadLimit {
				m.promptHistoryEntries = m.promptHistoryEntries[len(m.promptHistoryEntries)-promptSearchLoadLimit:]
			}
		}
	}
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   displayText,
		Status: "final",
	})
	return m, m.beginRunWithInput(promptInput, string(m.mode), "Request sent to LLM. Waiting for response...")
}

func (m model) submitBTW(value string) (tea.Model, tea.Cmd) {
	value = strings.TrimSpace(value)
	if value == "" {
		return m, nil
	}

	m.input.Reset()
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()) + " | btw",
		Body:   value,
		Status: "final",
	})
	var dropped int
	m.pendingBTW, dropped = queueBTWUpdate(m.pendingBTW, value)
	m.chatAutoFollow = true

	if m.interrupting {
		if dropped > 0 {
			m.statusNote = fmt.Sprintf("Queued BTW update (%d pending, dropped %d older). Waiting for current run to stop...", len(m.pendingBTW), dropped)
		} else {
			m.statusNote = fmt.Sprintf("Queued BTW update (%d pending). Waiting for current run to stop...", len(m.pendingBTW))
		}
		m.phase = "interrupting"
		if m.width > 0 && m.height > 0 {
			m.syncLayoutForCurrentScreen()
			m.refreshViewport()
		}
		return m, nil
	}

	wasToolPhase := m.phase == "tool"
	m.interrupting = true
	m.phase = "interrupting"
	if m.runCancel != nil {
		if wasToolPhase {
			m.interruptSafe = true
			m.statusNote = "BTW queued. Waiting for current tool step to finish..."
		} else {
			m.interruptSafe = false
			m.statusNote = "BTW received. Stopping current run..."
			m.runCancel()
		}
	} else {
		prompt := composeBTWPrompt(m.pendingBTW)
		m.pendingBTW = nil
		m.interrupting = false
		m.interruptSafe = false
		return m, m.beginRun(prompt, string(m.mode), "BTW accepted. Restarting with your update...")
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return m, nil
}

func (m *model) handleAgentEvent(event agent.Event) {
	switch event.Type {
	case agent.EventRunStarted:
		m.tempEstimatedOutput = 0
	case agent.EventAssistantDelta:
		m.phase = "responding"
		m.statusNote = "LLM is responding..."
		m.llmConnected = true
		m.appendAssistantDelta(event.Content)
	case agent.EventAssistantMessage:
		m.llmConnected = true
		m.finishAssistantMessage(event.Content)
	case agent.EventToolCallStarted:
		m.phase = "tool"
		m.llmConnected = true
		m.finalizeAssistantTurnForTool(event.ToolName)
		m.appendChat(chatEntry{
			Kind:   "tool",
			Title:  "Tool Call | " + event.ToolName,
			Body:   "",
			Status: "running",
		})
		m.toolRuns = append(m.toolRuns, toolRun{
			Name:    event.ToolName,
			Summary: "Tool call started.",
			Status:  "running",
		})
		m.statusNote = "Running tool: " + event.ToolName
	case agent.EventToolCallCompleted:
		summary, lines, status := summarizeTool(event.ToolName, event.ToolResult)
		m.finishLatestToolCall(event.ToolName, joinSummary(summary, lines), status)
		if len(m.toolRuns) > 0 {
			index := len(m.toolRuns) - 1
			m.toolRuns[index].Summary = summary
			m.toolRuns[index].Lines = lines
			m.toolRuns[index].Status = status
		}
		m.statusNote = summary
		m.phase = "thinking"
		if m.interruptSafe && m.interrupting && len(m.pendingBTW) > 0 && m.runCancel != nil {
			m.interruptSafe = false
			m.phase = "interrupting"
			m.statusNote = "BTW received. Stopping current run..."
			m.runCancel()
		}
	case agent.EventPlanUpdated:
		m.plan = copyPlanState(event.Plan)
		m.phase = string(planpkg.NormalizePhase(string(m.plan.Phase)))
		if m.phase == "none" {
			m.phase = "plan"
		}
		m.statusNote = fmt.Sprintf("Plan updated with %d step(s).", len(m.plan.Steps))
	case agent.EventUsageUpdated:
		m.applyUsage(event.Usage)
	case agent.EventRunFinished:
		if strings.TrimSpace(event.Content) != "" {
			m.statusNote = "Run finished."
		}
		m.phase = "idle"
	}
}

func (m *model) applyUsage(usage llm.Usage) {
	m.tokenHasOfficialUsage = true
	input := max(0, usage.InputTokens)
	output := max(0, usage.OutputTokens)
	context := max(0, usage.ContextTokens)
	used := usage.TotalTokens
	if used == 0 {
		used = input + output + context
	}
	used = max(0, used)
	if used == 0 && input == 0 && output == 0 && context == 0 {
		return
	}

	// Replace provisional stream estimate with provider-confirmed usage.
	if m.tempEstimatedOutput > 0 {
		m.tokenUsedTotal = max(0, m.tokenUsedTotal-m.tempEstimatedOutput)
		m.tokenOutput = max(0, m.tokenOutput-m.tempEstimatedOutput)
	}
	m.tempEstimatedOutput = 0

	m.tokenUsedTotal += used
	m.tokenInput += input
	m.tokenOutput += output
	m.tokenContext += context
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(false)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := m.chatItems[m.streamingIndex].Body
		if m.chatItems[m.streamingIndex].Status == "pending" ||
			m.chatItems[m.streamingIndex].Status == "thinking" ||
			current == m.thinkingText() {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasPrefix(delta, current) {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasSuffix(current, delta) {
			// Some providers may repeat the latest chunk; ignore it.
		} else {
			m.chatItems[m.streamingIndex].Body += delta
		}
		m.applyAssistantDeltaPresentation(&m.chatItems[m.streamingIndex])
		return
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   delta,
		Status: "streaming",
	})
	m.streamingIndex = len(m.chatItems) - 1
	m.applyAssistantDeltaPresentation(&m.chatItems[m.streamingIndex])
}

func (m *model) applyAssistantDeltaPresentation(item *chatEntry) {
	if item == nil || item.Kind != "assistant" {
		return
	}
	if shouldRenderThinkingFromDelta(item.Body) {
		item.Title = thinkingLabel
		item.Status = "thinking"
		return
	}
	item.Title = assistantLabel
	item.Status = "streaming"
}

func (m *model) finishAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := &m.chatItems[m.streamingIndex]
		if current.Status == "thinking" &&
			strings.TrimSpace(current.Body) != "" &&
			current.Body != m.thinkingText() {
			current.Title = thinkingLabel
			current.Status = "thinking"
			m.streamingIndex = -1
		} else {
			current.Title = assistantLabel
			current.Body = content
			current.Status = "final"
			m.streamingIndex = -1
			return
		}
	}
	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && last.Title == assistantLabel && strings.TrimSpace(last.Body) == content {
			last.Status = "final"
			return
		}
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   content,
		Status: "final",
	})
}

func (m *model) appendChat(item chatEntry) {
	m.chatItems = append(m.chatItems, item)
}

func (m *model) finalizeAssistantTurnForTool(toolName string) {
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		item := &m.chatItems[m.streamingIndex]
		if item.Kind == "assistant" {
			if !isMeaningfulThinking(item.Body, toolName) {
				m.removeStreamingAssistantPlaceholder()
				return
			}
			item.Title = thinkingLabel
			item.Status = "thinking"
			m.streamingIndex = -1
			return
		}
	}
}

func (m *model) removeStreamingAssistantPlaceholder() {
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		m.streamingIndex = -1
		return
	}
	if m.chatItems[m.streamingIndex].Kind == "assistant" {
		m.chatItems = append(m.chatItems[:m.streamingIndex], m.chatItems[m.streamingIndex+1:]...)
	}
	m.streamingIndex = -1
}

func (m *model) appendAssistantToolFollowUp(toolName, summary, status string) {
	step := assistantToolFollowUp(toolName, summary, status)
	if step == "" {
		return
	}
	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && strings.TrimSpace(last.Body) == step {
			last.Title = thinkingLabel
			last.Status = "thinking"
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   step,
		Status: "thinking",
	})
}

func (m *model) finishLatestToolCall(name, body, status string) {
	title := "Tool Call | " + name
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		if m.chatItems[i].Kind != "tool" {
			continue
		}
		if m.chatItems[i].Title != title && strings.TrimSpace(name) != "" {
			continue
		}
		m.chatItems[i].Title = title
		m.chatItems[i].Body = body
		m.chatItems[i].Status = status
		return
	}
	m.appendChat(chatEntry{
		Kind:   "tool",
		Title:  title,
		Body:   body,
		Status: status,
	})
}

func (m *model) updateThinkingCard() {
	if !m.busy || m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return
	}
	item := &m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" || (item.Status != "pending" && item.Status != "thinking") {
		return
	}
	item.Title = thinkingLabel
	item.Status = "thinking"
	item.Body = m.thinkingText()
}

func (m *model) failLatestAssistant(errText string) {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		errText = "Unknown provider error"
	}
	if len(m.chatItems) == 0 {
		m.appendChat(chatEntry{
			Kind:   "assistant",
			Title:  assistantLabel,
			Body:   "Request failed: " + errText,
			Status: "error",
		})
		return
	}
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		if m.chatItems[i].Kind == "assistant" {
			m.chatItems[i].Body = "Request failed: " + errText
			m.chatItems[i].Status = "error"
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   "Request failed: " + errText,
		Status: "error",
	})
}

func (m *model) newSession() error {
	next := session.New(m.workspace)
	if err := m.store.Save(next); err != nil {
		return err
	}
	m.sess = next
	m.screen = screenLanding
	m.plan = planpkg.State{}
	m.mode = modeBuild
	m.chatItems = nil
	m.toolRuns = nil
	m.streamingIndex = -1
	m.statusNote = "Started a new session."
	m.chatAutoFollow = true
	m.restoreTokenUsageFromSession(next)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.inputImageRefs = make(map[int]llm.AssetID, 8)
	m.inputImageMentions = make(map[string]llm.AssetID, 8)
	m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	m.nextImageID = nextSessionImageID(next)
	m.ensureSessionImageAssets()
	m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	m.nextPasteID = 1
	m.pastedStateLoaded = false
	m.lastCompressedPasteAt = time.Time{}
	m.ensurePastedContentState()
	m.pendingBTW = nil
	m.interrupting = false
	m.interruptSafe = false
	m.runCancel = nil
	m.activeRunID = 0
	m.input.Reset()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m *model) resumeSession(prefix string) error {
	id, err := resolveSessionID(m.sessions, prefix)
	if err != nil {
		return err
	}
	next, err := m.store.Load(id)
	if err != nil {
		return err
	}
	if !sameWorkspace(m.workspace, next.Workspace) {
		return fmt.Errorf("session %s belongs to workspace %s", next.ID, next.Workspace)
	}
	m.sess = next
	m.screen = screenChat
	m.plan = copyPlanState(next.Plan)
	m.mode = toAgentMode(next.Mode)
	m.chatItems, m.toolRuns = rebuildSessionTimeline(next)
	m.streamingIndex = -1
	m.statusNote = "Resumed session " + shortID(next.ID)
	m.chatAutoFollow = true
	m.restoreTokenUsageFromSession(next)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.inputImageRefs = make(map[int]llm.AssetID, 8)
	m.inputImageMentions = make(map[string]llm.AssetID, 8)
	m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	m.nextImageID = nextSessionImageID(next)
	m.ensureSessionImageAssets()
	m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	m.nextPasteID = 1
	m.pastedStateLoaded = false
	m.lastCompressedPasteAt = time.Time{}
	m.ensurePastedContentState()
	m.syncInputImageRefs("")
	m.pendingBTW = nil
	m.interrupting = false
	m.interruptSafe = false
	m.runCancel = nil
	m.activeRunID = 0
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m model) startRunCmd(runCtx context.Context, runID int, prompt agent.RunPromptInput, mode string) tea.Cmd {
	return func() tea.Msg {
		go func() {
			_, err := m.runner.RunPromptWithInput(runCtx, m.sess, prompt, mode, io.Discard)
			m.async <- runFinishedMsg{RunID: runID, Err: err}
		}()
		return nil
	}
}

func (m model) loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return sessionsLoadedMsg{}
		}
		summaries, _, err := m.store.List(m.sessionLimit)
		return sessionsLoadedMsg{Summaries: summaries, Err: err}
	}
}

func (m model) fetchRemoteTokenUsageCmd() tea.Cmd {
	return func() tea.Msg {
		usage, err := fetchCurrentMonthUsage(m.cfg)
		if err != nil {
			return tokenUsagePulledMsg{Err: err}
		}
		return tokenUsagePulledMsg{
			Used:    usage.Used,
			Input:   usage.Input,
			Output:  usage.Output,
			Context: usage.Context,
		}
	}
}

func (m *model) restoreTokenUsageFromSession(sess *session.Session) {
	m.tempEstimatedOutput = 0
	m.tokenHasOfficialUsage = false
	m.tokenUsedTotal = 0
	m.tokenInput = 0
	m.tokenOutput = 0
	m.tokenContext = 0

	if sess != nil {
		m.accumulateTokenUsage(sess.Messages)
	}
}

func (m *model) accumulateTokenUsage(messages []llm.Message) {
	for _, msg := range messages {
		if msg.Usage == nil {
			continue
		}
		m.tokenHasOfficialUsage = true
		used := msg.Usage.TotalTokens
		if used <= 0 {
			used = msg.Usage.InputTokens + msg.Usage.OutputTokens + msg.Usage.ContextTokens
		}
		m.tokenUsedTotal += max(0, used)
		m.tokenInput += max(0, msg.Usage.InputTokens)
		m.tokenOutput += max(0, msg.Usage.OutputTokens)
		m.tokenContext += max(0, msg.Usage.ContextTokens)
	}
}

func waitForAsync(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
