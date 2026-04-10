package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	"bytemind/internal/provider"
	"bytemind/internal/session"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) handleCommandPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.filteredCommands()
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-commandPageSize)
		}
		return m, nil
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+commandPageSize)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closeCommandPalette()
		return m, nil
	case "up":
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-1)
		}
		return m, nil
	case "down":
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+1)
		}
		return m, nil
	case "enter":
		selected, ok := m.selectedCommandItem()
		if !ok {
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				return m, nil
			}
			if value == "/quit" {
				m.closeCommandPalette()
				return m, tea.Quit
			}
			if m.busy {
				if isBTWCommand(value) {
					btw, err := extractBTWText(value)
					if err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
					m.closeCommandPalette()
					return m.submitBTW(btw)
				}
				if strings.HasPrefix(value, "/") {
					m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message>."
					return m, nil
				}
				m.closeCommandPalette()
				return m.submitBTW(value)
			}
			m.closeCommandPalette()
			m.input.Reset()
			next, cmd, err := m.executeCommand(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		m.closeCommandPalette()
		if shouldExecuteFromPalette(selected) || selected.Name == "/continue" {
			if selected.Name == "/quit" {
				return m, tea.Quit
			}
			if m.busy {
				m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message>."
				return m, nil
			}
			m.input.Reset()
			next, cmd, err := m.executeCommand(selected.Name)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		m.setInputValue(selected.Usage)
		m.statusNote = selected.Description
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.handleInputMutation(before, m.input.Value(), msg.String())
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m model) handleMentionPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.mentionResults
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.mentionCursor = max(0, m.mentionCursor-mentionPageSize)
		}
		return m, nil
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.mentionCursor = min(len(items)-1, m.mentionCursor+mentionPageSize)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closeMentionPalette()
		return m, nil
	case "up", "k":
		if len(items) > 0 {
			m.mentionCursor = max(0, m.mentionCursor-1)
		}
		return m, nil
	case "down", "j":
		if len(items) > 0 {
			m.mentionCursor = min(len(items)-1, m.mentionCursor+1)
		}
		return m, nil
	case "tab":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			return m, nil
		}
		m.applyMentionSelection(selected)
		return m, nil
	case "enter":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			m.closeMentionPalette()
			return m.handleKey(msg)
		}
		m.applyMentionSelection(selected)
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.handleInputMutation(before, m.input.Value(), msg.String())
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m *model) applyMentionSelection(selected mention.Candidate) {
	m.recordRecentMention(selected.Path)

	if assetID, note, isImage := m.ingestMentionImageCandidate(selected.Path); isImage {
		if strings.TrimSpace(string(assetID)) != "" {
			m.bindMentionImageAsset(selected.Path, assetID)
			nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
			m.setInputValue(nextValue)
			if strings.TrimSpace(note) != "" {
				m.statusNote = note
			} else {
				m.statusNote = "Attached image: @" + filepath.ToSlash(strings.TrimSpace(selected.Path))
			}
			m.closeMentionPalette()
			m.syncInputOverlays()
			return
		}
		if strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}

	nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
	m.setInputValue(nextValue)
	m.statusNote = "Inserted mention: " + selected.Path
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) ingestMentionImageCandidate(path string) (assetID llm.AssetID, note string, isImage bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", false
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.workspace, resolved)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	if _, ok := mediaTypeFromPath(resolved); !ok {
		return "", "", false
	}

	placeholder, note, ok := m.ingestImageFromPath(resolved)
	if !ok {
		return "", note, true
	}
	imageID, ok := imageIDFromPlaceholder(placeholder)
	if !ok {
		return "", "image ingest failed: invalid placeholder id", true
	}
	assetID, _, ok = m.findAssetByImageID(imageID)
	if !ok {
		return "", "image ingest failed: asset metadata missing", true
	}
	return assetID, note, true
}

func (m *model) bindMentionImageAsset(path string, assetID llm.AssetID) {
	if m == nil {
		return
	}
	key := normalizeImageMentionPath(path)
	if key == "" || strings.TrimSpace(string(assetID)) == "" {
		return
	}
	if m.inputImageMentions == nil {
		m.inputImageMentions = make(map[string]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	if prev, ok := m.inputImageMentions[key]; ok && prev != assetID {
		m.orphanedImages[prev] = time.Now().UTC()
	}
	m.inputImageMentions[key] = assetID
	delete(m.orphanedImages, assetID)
}

func (m *model) openCommandPalette() {
	m.commandOpen = true
	m.commandCursor = 0
	m.setInputValue("/")
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) openPromptSearch(mode promptSearchMode) {
	m.ensurePromptHistoryLoaded()
	m.promptSearchMode = mode
	m.promptSearchBaseInput = m.input.Value()
	m.promptSearchQuery = ""
	m.promptSearchCursor = 0
	m.promptSearchOpen = true
	m.commandOpen = false
	m.closeMentionPalette()
	m.refreshPromptSearchMatches()
	if len(m.promptSearchMatches) == 0 {
		if mode == promptSearchModePanel {
			m.statusNote = "History panel opened. No matching prompts."
		} else {
			m.statusNote = "No matching prompts."
		}
	} else {
		if mode == promptSearchModePanel {
			m.statusNote = fmt.Sprintf("History panel ready (%d matches).", len(m.promptSearchMatches))
		} else {
			m.statusNote = fmt.Sprintf("Prompt history ready (%d matches).", len(m.promptSearchMatches))
		}
	}
}

func (m *model) closePromptSearch(restoreInput bool) {
	if restoreInput {
		m.setInputValue(m.promptSearchBaseInput)
	}
	m.promptSearchOpen = false
	m.promptSearchMode = ""
	m.promptSearchQuery = ""
	m.promptSearchMatches = nil
	m.promptSearchCursor = 0
	m.promptSearchBaseInput = ""
	m.syncInputOverlays()
}

func (m *model) ensurePromptHistoryLoaded() {
	if m.promptHistoryLoaded {
		return
	}
	entries, err := history.LoadRecentPrompts(promptSearchLoadLimit)
	if err != nil {
		m.promptHistoryEntries = nil
		m.promptHistoryLoaded = true
		m.statusNote = "Prompt history unavailable: " + compact(err.Error(), 72)
		return
	}
	m.promptHistoryEntries = entries
	m.promptHistoryLoaded = true
}

func (m *model) refreshPromptSearchMatches() {
	tokens, workspaceFilter, sessionFilter := parsePromptSearchQuery(m.promptSearchQuery)
	limit := promptSearchResultCap
	if m.promptSearchMode == promptSearchModePanel {
		limit = promptSearchLoadLimit
	}
	matches := make([]history.PromptEntry, 0, min(len(m.promptHistoryEntries), limit))
	for i := len(m.promptHistoryEntries) - 1; i >= 0; i-- {
		entry := m.promptHistoryEntries[i]
		prompt := strings.TrimSpace(entry.Prompt)
		if prompt == "" {
			continue
		}
		workspaceValue := strings.ToLower(strings.TrimSpace(entry.Workspace))
		if workspaceFilter != "" && !strings.Contains(workspaceValue, workspaceFilter) {
			continue
		}
		sessionValue := strings.ToLower(strings.TrimSpace(entry.SessionID))
		if sessionFilter != "" && !strings.Contains(sessionValue, sessionFilter) {
			continue
		}
		promptLower := strings.ToLower(prompt)
		if !matchAllTokens(promptLower, tokens) {
			continue
		}
		matches = append(matches, entry)
		if len(matches) >= limit {
			break
		}
	}

	m.promptSearchMatches = matches
	if len(matches) == 0 {
		m.promptSearchCursor = 0
		return
	}
	m.promptSearchCursor = clamp(m.promptSearchCursor, 0, len(matches)-1)
}

func (m *model) stepPromptSearch(delta int) {
	if len(m.promptSearchMatches) == 0 {
		return
	}
	next := m.promptSearchCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.promptSearchMatches) {
		next = len(m.promptSearchMatches) - 1
	}
	m.promptSearchCursor = next
}

func (m *model) trimPromptSearchQuery() {
	if m.promptSearchQuery == "" {
		return
	}
	runes := []rune(m.promptSearchQuery)
	m.promptSearchQuery = string(runes[:len(runes)-1])
	m.refreshPromptSearchMatches()
}

func (m *model) closeCommandPalette() {
	m.commandOpen = false
	m.commandCursor = 0
	m.closeMentionPalette()
	m.input.Reset()
}

func (m model) selectedCommandItem() (commandItem, bool) {
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandItem{}, false
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	return items[index], true
}

func (m model) renderPromptSearchPalette() string {
	width := m.commandPaletteWidth()
	items := m.promptSearchMatches
	modeLabel := "search"
	if m.promptSearchMode == promptSearchModePanel {
		modeLabel = "panel"
	}
	if len(items) == 0 {
		query := strings.TrimSpace(m.promptSearchQuery)
		if query == "" {
			query = "(all)"
		}
		content := []string{
			commandPaletteMetaStyle.Render("Prompt history " + modeLabel),
			commandPaletteMetaStyle.Render("query: "+query+"  (filters: ") + renderInlineShortcutHints(promptSearchFilterHints) + commandPaletteMetaStyle.Render(")"),
			commandPaletteMetaStyle.Render("No matching prompts."),
			commandPaletteMetaStyle.Render("Type to filter  ") +
				renderInlineShortcutHints([]footerShortcutHint{
					{Key: "PgUp/PgDn", Label: "page"},
					{Key: "Enter", Label: "apply"},
					{Key: "Esc", Label: "close"},
				}),
		}
		return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, content...))
	}

	selected, _ := m.selectedPromptSearchEntry()
	rowWidth := max(1, width-commandPaletteStyle.GetHorizontalFrameSize())
	rows := make([]string, 0, promptSearchPageSize+3)
	for _, item := range m.visiblePromptSearchEntriesPage() {
		rowStyle := commandPaletteRowStyle
		textStyle := commandPaletteDescStyle
		if item.Timestamp.Equal(selected.Timestamp) && item.SessionID == selected.SessionID && item.Prompt == selected.Prompt {
			rowStyle = commandPaletteSelectedRowStyle
			textStyle = commandPaletteSelectedDescStyle
		}
		workspaceName := filepath.Base(strings.TrimSpace(item.Workspace))
		if workspaceName == "" || workspaceName == "." {
			workspaceName = strings.TrimSpace(item.Workspace)
		}
		if workspaceName == "" {
			workspaceName = "-"
		}
		meta := fmt.Sprintf("%s  ws:%s  sid:%s", item.Timestamp.Local().Format("01-02 15:04"), compact(workspaceName, 16), compact(item.SessionID, 12))
		rowText := compact(strings.TrimSpace(item.Prompt), max(12, rowWidth-2))
		rows = append(rows, rowStyle.Width(rowWidth).Render(textStyle.Render(rowText)))
		rows = append(rows, rowStyle.Width(rowWidth).Render(commandPaletteMetaStyle.Render(compact(meta, max(12, rowWidth-2)))))
	}
	for len(rows) < promptSearchPageSize*2 {
		rows = append(rows, commandPaletteRowStyle.Width(rowWidth).Render(""))
	}

	query := strings.TrimSpace(m.promptSearchQuery)
	if query == "" {
		query = "(all)"
	}
	meta := commandPaletteMetaStyle.Render(fmt.Sprintf("%s  query:%s", modeLabel, compact(query, 24))) +
		footerHintDividerStyle.Render("  |  ") +
		renderInlineShortcutHints(promptSearchFilterHints) +
		footerHintDividerStyle.Render("  |  ") +
		renderInlineShortcutHints(promptSearchActionHints)
	rows = append(rows, meta)
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m model) renderStartupGuidePanel() string {
	width := max(24, m.commandPaletteWidth())
	title := strings.TrimSpace(m.startupGuide.Title)
	if title == "" {
		title = "Provider setup required"
	}
	status := strings.TrimSpace(m.startupGuide.Status)
	if status == "" {
		status = "AI provider is not available."
	}

	innerWidth := max(20, width-commandPaletteStyle.GetHorizontalFrameSize())
	content := make([]string, 0, 2+len(m.startupGuide.Lines))
	content = append(content, accentStyle.Render(title))
	content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(status))
	for _, line := range m.startupGuide.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(line))
	}
	content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(startupGuideInputHint(m.startupGuide.CurrentField)))

	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, content...))
}

func (m *model) handleStartupGuideSubmission(rawInput string) error {
	rawInput = strings.TrimSpace(rawInput)

	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	if explicitField, explicitValue, ok := parseStartupConfigInput(rawInput); ok {
		field = explicitField
		rawInput = explicitValue
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		value, err := m.resolveStartupFieldValue(field, rawInput)
		if err != nil {
			return err
		}
		if err := m.applyStartupConfigField(field, value); err != nil {
			return err
		}
		next := startupNextField(field)
		if next == "" {
			next = startupFieldAPIKey
		}
		m.setStartupGuideStep(next, "")
		m.input.Reset()
		return nil
	case startupFieldAPIKey:
		return m.verifyAndFinalizeStartupAPIKey(rawInput)
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}
}

func (m *model) verifyAndFinalizeStartupAPIKey(rawInput string) error {
	apiKey := sanitizeAPIKeyInput(rawInput)
	if apiKey == "" {
		return fmt.Errorf("please paste a non-empty API key")
	}

	checkCfg := m.cfg.Provider
	checkCfg.APIKey = apiKey
	check := provider.CheckAvailability(context.Background(), checkCfg)
	if !check.Ready {
		m.llmConnected = false
		m.phase = "error"
		m.setStartupGuideStep(startupFieldAPIKey, startupGuideIssueHint(check))
		return nil
	}

	writtenPath, saveErr := config.UpsertProviderAPIKey(m.startupGuide.ConfigPath, apiKey)

	if envName := strings.TrimSpace(checkCfg.APIKeyEnv); envName != "" {
		if err := os.Setenv(envName, apiKey); err != nil {
			warnSetenv(envName, err)
		}
	} else {
		if err := os.Setenv("BYTEMIND_API_KEY", apiKey); err != nil {
			warnSetenv("BYTEMIND_API_KEY", err)
		}
	}

	client, err := provider.NewClient(checkCfg)
	if err != nil {
		return err
	}
	if m.runner != nil {
		m.runner.UpdateProvider(checkCfg, client)
	}
	m.cfg.Provider = checkCfg
	m.startupGuide.Active = false
	m.statusNote = "Provider configured and verified. You can start chatting."
	m.llmConnected = true
	m.phase = "idle"
	if saveErr != nil {
		m.statusNote = "Provider verified, but config save failed: " + compact(saveErr.Error(), 80)
	} else if strings.TrimSpace(writtenPath) != "" {
		m.statusNote = "Provider configured and verified. Saved to " + compact(writtenPath, 48)
	}
	m.syncInputStyle()
	m.input.Reset()
	return nil
}

func (m *model) applyStartupConfigField(field, value string) error {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	persistValue := value

	switch field {
	case "model":
		m.cfg.Provider.Model = value
	case "base_url":
		m.cfg.Provider.BaseURL = value
	case "type":
		normalized, ok := normalizeStartupProviderType(value)
		if !ok {
			return fmt.Errorf("provider must be openai-compatible or anthropic")
		}
		m.cfg.Provider.Type = normalized
		persistValue = normalized
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}

	writtenPath, err := config.UpsertProviderField(m.startupGuide.ConfigPath, field, persistValue)
	if err != nil {
		return err
	}
	if strings.TrimSpace(writtenPath) != "" {
		m.startupGuide.ConfigPath = writtenPath
	}
	return nil
}

func isStartupGuideField(field string) bool {
	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel, startupFieldAPIKey:
		return true
	default:
		return false
	}
}

func startupNextField(current string) string {
	for i, field := range startupFieldOrder {
		if field == current {
			if i+1 >= len(startupFieldOrder) {
				return ""
			}
			return startupFieldOrder[i+1]
		}
	}
	return startupFieldType
}

func startupFieldStep(field string) (int, int) {
	for i, item := range startupFieldOrder {
		if item == field {
			return i + 1, len(startupFieldOrder)
		}
	}
	return 1, len(startupFieldOrder)
}

func startupFieldName(field string) string {
	switch field {
	case startupFieldType:
		return "provider"
	case startupFieldBaseURL:
		return "base_url"
	case startupFieldModel:
		return "model"
	case startupFieldAPIKey:
		return "api_key"
	default:
		return field
	}
}

func startupProviderDefaultBaseURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return "https://api.openai.com/v1"
	}
}

func startupProviderDefaultModel(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return ""
	default:
		return "GPT-5.4"
	}
}

func (m model) startupCurrentValue(field string) string {
	switch field {
	case startupFieldType:
		return strings.TrimSpace(m.cfg.Provider.Type)
	case startupFieldBaseURL:
		return strings.TrimSpace(m.cfg.Provider.BaseURL)
	case startupFieldModel:
		return strings.TrimSpace(m.cfg.Provider.Model)
	default:
		return ""
	}
}

func (m *model) resolveStartupFieldValue(field, rawInput string) (string, error) {
	value := strings.TrimSpace(rawInput)
	if value != "" {
		return value, nil
	}

	current := m.startupCurrentValue(field)
	if current != "" {
		return current, nil
	}

	switch field {
	case startupFieldType:
		return "openai-compatible", nil
	case startupFieldBaseURL:
		return startupProviderDefaultBaseURL(m.cfg.Provider.Type), nil
	case startupFieldModel:
		if fallback := startupProviderDefaultModel(m.cfg.Provider.Type); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("please enter model name for provider %s", strings.TrimSpace(m.cfg.Provider.Type))
	default:
		return "", fmt.Errorf("%s cannot be empty", startupFieldName(field))
	}
}

func (m *model) initializeStartupGuide() {
	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	m.setStartupGuideStep(field, "")
}

func (m *model) setStartupGuideStep(field, issue string) {
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	step, total := startupFieldStep(field)
	fieldName := startupFieldName(field)
	if strings.TrimSpace(issue) == "" {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s.", step, total, fieldName)
	} else {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s. %s", step, total, fieldName, issue)
	}
	m.statusNote = m.startupGuide.Status
	m.startupGuide.CurrentField = field
	m.startupGuide.Lines = startupGuideStepLines(field, m.cfg, m.startupGuide.ConfigPath, issue)
	m.syncInputStyle()
}

func startupGuideStepLines(field string, cfg config.Config, configPath, issue string) []string {
	lines := make([]string, 0, 8)
	switch field {
	case startupFieldType:
		lines = append(lines, "Enter provider: openai-compatible or anthropic.")
	case startupFieldBaseURL:
		lines = append(lines, "Enter provider base_url.")
		lines = append(lines, "Example: https://api.deepseek.com")
	case startupFieldModel:
		lines = append(lines, "Enter model name.")
		lines = append(lines, "Example: deepseek-chat or GPT-5.4")
	case startupFieldAPIKey:
		lines = append(lines, "Paste API key and press Enter.")
		lines = append(lines, "Bytemind will verify it automatically.")
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		current := ""
		switch field {
		case startupFieldType:
			current = strings.TrimSpace(cfg.Provider.Type)
		case startupFieldBaseURL:
			current = strings.TrimSpace(cfg.Provider.BaseURL)
		case startupFieldModel:
			current = strings.TrimSpace(cfg.Provider.Model)
		}
		if current == "" {
			lines = append(lines, "Press Enter to use default.")
		} else {
			lines = append(lines, "Press Enter to keep current: "+current)
		}
	}
	if strings.TrimSpace(issue) != "" {
		lines = append(lines, "Issue: "+issue)
	}
	if strings.TrimSpace(configPath) != "" {
		lines = append(lines, "Config file: "+configPath)
	}
	return lines
}

func startupGuideIssueHint(check provider.Availability) string {
	reason := strings.ToLower(strings.TrimSpace(check.Reason))
	switch {
	case strings.Contains(reason, "missing api key"):
		return "No API key is configured yet."
	case strings.Contains(reason, "unauthorized"):
		return "The API key was rejected by the provider."
	case strings.Contains(reason, "failed to reach"):
		return "Cannot reach provider endpoint. Check proxy or network."
	case strings.Contains(reason, "not found"):
		return "Provider endpoint path looks incorrect."
	default:
		if strings.TrimSpace(check.Reason) == "" {
			return "Provider check failed."
		}
		return compact(strings.TrimSpace(check.Reason), 90)
	}
}

func (m model) renderCommandPalette() string {
	width := m.commandPaletteWidth()
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandPaletteStyle.Width(width).Render(
			commandPaletteMetaStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render("No matching commands."),
		)
	}

	selected, _ := m.selectedCommandItem()
	nameWidth := min(26, max(14, width/4))
	descWidth := max(12, width-commandPaletteStyle.GetHorizontalFrameSize()-nameWidth-4)
	rows := make([]string, 0, commandPageSize+1)
	for _, item := range m.visibleCommandItemsPage() {
		rowStyle := commandPaletteRowStyle
		nameStyle := commandPaletteNameStyle
		descStyle := commandPaletteDescStyle
		if item.Name == selected.Name {
			rowStyle = commandPaletteSelectedRowStyle
			nameStyle = commandPaletteSelectedNameStyle
			descStyle = commandPaletteSelectedDescStyle
		}

		name := nameStyle.Width(nameWidth).Render(item.Usage)
		desc := descStyle.Width(descWidth).Render(compact(item.Description, max(12, descWidth)))
		rows = append(rows, rowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, name, "  ", desc),
		))
	}
	for len(rows) < commandPageSize {
		rows = append(rows, commandPaletteRowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(""))
	}
	rows = append(rows, commandPaletteMetaStyle.Render("Up/Down move  PgUp/PgDn page  Enter run  Esc close"))
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m model) renderMentionPalette() string {
	width := m.commandPaletteWidth()
	items := m.mentionResults
	if len(items) == 0 {
		return commandPaletteStyle.Width(width).Render(
			commandPaletteMetaStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render("No matching files in workspace."),
		)
	}

	selected, _ := m.selectedMentionCandidate()
	nameWidth := min(26, max(12, width/4))
	descWidth := max(12, width-commandPaletteStyle.GetHorizontalFrameSize()-nameWidth-4)
	rows := make([]string, 0, mentionPageSize+1)
	for _, item := range m.visibleMentionItemsPage() {
		rowStyle := commandPaletteRowStyle
		nameStyle := commandPaletteNameStyle
		descStyle := commandPaletteDescStyle
		if item.Path == selected.Path {
			rowStyle = commandPaletteSelectedRowStyle
			nameStyle = commandPaletteSelectedNameStyle
			descStyle = commandPaletteSelectedDescStyle
		}

		nameText := item.BaseName
		if tag := strings.TrimSpace(item.TypeTag); tag != "" {
			nameText = "[" + tag + "] " + nameText
		}
		if m.hasRecentMention(item.Path) {
			nameText = "* " + nameText
		} else {
			nameText = "  " + nameText
		}

		name := nameStyle.Width(nameWidth).Render(compact(nameText, max(12, nameWidth)))
		desc := descStyle.Width(descWidth).Render(compact(item.Path, max(12, descWidth)))
		rows = append(rows, rowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, name, "  ", desc),
		))
	}
	for len(rows) < mentionPageSize {
		rows = append(rows, commandPaletteRowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(""))
	}
	metaText := "* recent  Type @query to search  Up/Down move  Enter/Tab insert  Esc close"
	if m.mentionIndex != nil {
		stats := m.mentionIndex.Stats()
		if stats.Truncated && stats.MaxFiles > 0 {
			metaText = fmt.Sprintf("* recent  indexed first %d files  Enter/Tab insert  Esc close", stats.MaxFiles)
		}
	}
	rows = append(rows, commandPaletteMetaStyle.Render(metaText))
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m *model) handleSlashCommand(input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "/help":
		m.screen = screenChat
		m.appendChat(chatEntry{
			Kind:   "user",
			Title:  "You",
			Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
			Body:   input,
			Status: "final",
		})
		m.appendChat(chatEntry{Kind: "assistant", Title: assistantLabel, Body: m.helpText(), Status: "final"})
		m.statusNote = "Help opened in the conversation view."
		return nil
	case "/session":
		m.sessionsOpen = true
		m.statusNote = "Opened recent sessions."
		return nil
	case "/skills":
		return m.runSkillsListCommand(input)
	case "/skill":
		return m.runSkillCommand(input, fields)
	case "/new":
		return m.newSession()
	case "/compact":
		return m.runCompactCommand(input)
	default:
		return m.runDirectSkillCommand(input, fields)
	}
}

func (m model) executeCommand(input string) (tea.Model, tea.Cmd, error) {
	if err := m.handleSlashCommand(input); err != nil {
		return m, nil, err
	}
	m.refreshViewport()
	return m, m.loadSessionsCmd(), nil
}

func (m *model) runSkillsListCommand(input string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skillsList, diagnostics := m.runner.ListSkills()
	active, hasActive := m.runner.GetActiveSkill(m.sess)

	lines := make([]string, 0, len(skillsList)+8)
	if hasActive {
		lines = append(lines, fmt.Sprintf("Active skill: %s (%s)", active.Name, active.Scope))
	} else {
		lines = append(lines, "Active skill: none")
	}
	lines = append(lines, "")
	if len(skillsList) == 0 {
		lines = append(lines, "No skills discovered.")
	} else {
		lines = append(lines, "Available skills:")
		for _, skill := range skillsList {
			lines = append(lines, fmt.Sprintf("- %s (%s): %s", skill.Name, skill.Scope, skill.Description))
		}
	}
	if len(diagnostics) > 0 {
		lines = append(lines, "", "Diagnostics:")
		for _, diag := range diagnostics {
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s): %s", diag.Level, diag.Skill, diag.Path, diag.Message))
		}
	}

	m.appendCommandExchange(input, strings.Join(lines, "\n"))
	m.statusNote = fmt.Sprintf("Discovered %d skill(s).", len(skillsList))
	return nil
}

func (m *model) runSkillCommand(input string, fields []string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	if len(fields) != 2 || fields[1] != "clear" {
		return fmt.Errorf("usage: /skill clear")
	}

	if err := m.runner.ClearActiveSkill(m.sess); err != nil {
		return err
	}
	m.appendCommandExchange(input, "Active skill cleared.")
	m.statusNote = "Skill cleared."
	return nil
}

func (m *model) runCompactCommand(input string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	if m.sess == nil {
		return fmt.Errorf("session is unavailable")
	}
	type sessionCompactor interface {
		CompactSession(ctx context.Context, sess *session.Session) (string, bool, error)
	}
	compactor, ok := any(m.runner).(sessionCompactor)
	if !ok {
		return fmt.Errorf("compact is unavailable in this build")
	}
	summary, changed, err := compactor.CompactSession(context.Background(), m.sess)
	if err != nil {
		return err
	}
	if !changed {
		m.appendCommandExchange(input, "No compaction needed yet. Start a longer conversation first.")
		m.statusNote = "No compaction needed."
		return nil
	}
	preview := compact(summary, 360)
	response := "Conversation compacted for long-context continuation."
	if strings.TrimSpace(preview) != "" {
		response += "\nSummary preview: " + preview
	}
	m.chatItems, m.toolRuns = rebuildSessionTimeline(m.sess)
	m.appendCommandExchange(input, response)
	m.statusNote = "Conversation compacted."
	return nil
}

func (m *model) runDirectSkillCommand(input string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	name := strings.TrimSpace(fields[0])
	if !strings.HasPrefix(name, "/") || !m.isKnownSkillCommand(name) {
		return fmt.Errorf("unknown command: %s", fields[0])
	}
	args, err := parseSkillArgs(fields[1:])
	if err != nil {
		return err
	}
	return m.activateSkillCommand(input, name, args)
}

func (m *model) activateSkillCommand(input, name string, args map[string]string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skill, err := m.runner.ActivateSkill(m.sess, name, args)
	if err != nil {
		return err
	}
	response := fmt.Sprintf("Activated skill `%s` (%s).\nTool policy: %s\nEntry: %s", skill.Name, skill.Scope, skill.ToolPolicy.Policy, skill.Entry.Slash)
	if len(args) > 0 {
		argParts := make([]string, 0, len(args))
		keys := make([]string, 0, len(args))
		for key := range args {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			argParts = append(argParts, fmt.Sprintf("%s=%s", key, args[key]))
		}
		response += "\nArgs: " + strings.Join(argParts, ", ")
	}
	m.appendCommandExchange(input, response)
	m.statusNote = "Skill activated."
	return nil
}

func (m *model) appendCommandExchange(command, response string) {
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   command,
		Status: "final",
	})
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   response,
		Status: "final",
	})
}

func (m *model) syncCommandPalette() {
	value := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(value, "/") {
		m.commandOpen = false
		m.commandCursor = 0
		return
	}
	m.commandOpen = true
	m.closeMentionPalette()
	items := m.filteredCommands()
	if len(items) == 0 {
		m.commandCursor = 0
		return
	}
	if m.commandCursor < 0 || m.commandCursor >= len(items) {
		m.commandCursor = 0
	}
}

func (m *model) syncInputOverlays() {
	if m.startupGuide.Active || m.promptSearchOpen {
		return
	}
	m.syncCommandPalette()
	m.syncMentionPalette()
	m.syncInputImageRefs(m.input.Value())
}

func (m model) handlePromptSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isPageUpKey(msg):
		m.stepPromptSearch(-promptSearchPageSize)
		return m, nil
	case isPageDownKey(msg):
		m.stepPromptSearch(promptSearchPageSize)
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closePromptSearch(true)
		m.statusNote = "Prompt search canceled."
		return m, nil
	case "enter":
		selected, ok := m.selectedPromptSearchEntry()
		if ok {
			m.setInputValue(selected.Prompt)
			m.closePromptSearch(false)
			m.statusNote = "Prompt restored from history."
			return m, nil
		}
		m.closePromptSearch(true)
		m.statusNote = "No prompt selected."
		return m, nil
	case "ctrl+f", "down", "j":
		m.stepPromptSearch(1)
		return m, nil
	case "ctrl+s", "up", "k":
		m.stepPromptSearch(-1)
		return m, nil
	case "home":
		m.stepPromptSearch(-len(m.promptSearchMatches))
		return m, nil
	case "end":
		m.stepPromptSearch(len(m.promptSearchMatches))
		return m, nil
	case "backspace", "ctrl+h":
		m.trimPromptSearchQuery()
		return m, nil
	}

	switch msg.Type {
	case tea.KeyBackspace:
		m.trimPromptSearchQuery()
		return m, nil
	case tea.KeySpace:
		m.promptSearchQuery += " "
		m.refreshPromptSearchMatches()
		return m, nil
	case tea.KeyRunes:
		m.promptSearchQuery += string(msg.Runes)
		m.refreshPromptSearchMatches()
		return m, nil
	default:
		return m, nil
	}
}

func (m *model) syncMentionPalette() {
	if m.commandOpen {
		m.closeMentionPalette()
		return
	}
	token, ok := mention.FindActiveToken(m.input.Value())
	if !ok {
		m.closeMentionPalette()
		return
	}

	if m.mentionIndex == nil {
		m.mentionIndex = mention.NewWorkspaceFileIndex(m.workspace)
	}
	results := m.mentionIndex.SearchWithRecency(token.Query, mentionPageSize*3, m.mentionRecent)
	m.mentionOpen = true
	m.mentionQuery = token.Query
	m.mentionToken = token
	m.mentionResults = results

	if len(results) == 0 {
		m.mentionCursor = 0
		return
	}
	if m.mentionCursor < 0 || m.mentionCursor >= len(results) {
		m.mentionCursor = 0
	}
}

func (m *model) closeMentionPalette() {
	m.mentionOpen = false
	m.mentionCursor = 0
	m.mentionQuery = ""
	m.mentionToken = mention.Token{}
	m.mentionResults = nil
}

func (m *model) recordRecentMention(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if m.mentionRecent == nil {
		m.mentionRecent = make(map[string]int, 16)
	}
	m.mentionSeq++
	m.mentionRecent[path] = m.mentionSeq
}

func (m model) hasRecentMention(path string) bool {
	if m.mentionRecent == nil {
		return false
	}
	return m.mentionRecent[path] > 0
}

func (m model) filteredCommands() []commandItem {
	value := strings.TrimSpace(m.input.Value())
	query := commandFilterQuery(value, "")
	items := m.commandPaletteItems()
	if query == "" {
		return items
	}

	result := make([]commandItem, 0, len(items))
	for _, item := range items {
		if matchesCommandItem(item, query) {
			result = append(result, item)
		}
	}
	return result
}

func (m model) commandPaletteItems() []commandItem {
	base := visibleCommandItems("")
	skillItems := m.skillCommandItems()
	if len(skillItems) == 0 {
		return base
	}

	items := make([]commandItem, 0, len(base)+len(skillItems))
	seen := make(map[string]struct{}, len(base)+len(skillItems))
	items = append(items, base...)
	for _, item := range base {
		seen[strings.ToLower(strings.TrimSpace(item.Usage))] = struct{}{}
	}
	for _, item := range skillItems {
		key := strings.ToLower(strings.TrimSpace(item.Usage))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	return items
}

func (m model) skillCommandItems() []commandItem {
	if m.runner == nil {
		return nil
	}
	skillsList, _ := m.runner.ListSkills()
	if len(skillsList) == 0 {
		return nil
	}

	items := make([]commandItem, 0, len(skillsList))
	seen := make(map[string]struct{}, len(skillsList))
	for _, skill := range skillsList {
		name := strings.TrimSpace(skill.Entry.Slash)
		if name == "" {
			name = "/" + strings.TrimSpace(skill.Name)
		}
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		name = "/" + strings.TrimLeft(strings.TrimSpace(name), "/")
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = fmt.Sprintf("Activate %s for this session.", skill.Name)
		}
		items = append(items, commandItem{
			Name:        name,
			Usage:       name,
			Description: description,
			Kind:        "skill",
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Usage < items[j].Usage
	})
	return items
}

func (m model) commandPaletteWidth() int {
	switch m.screen {
	case screenLanding:
		return max(28, m.landingInputShellWidth())
	default:
		return max(32, m.chatPanelInnerWidth())
	}
}

func (m model) visibleCommandItemsPage() []commandItem {
	items := m.filteredCommands()
	if len(items) == 0 {
		return nil
	}
	cursor := clamp(m.commandCursor, 0, len(items)-1)
	start := (cursor / commandPageSize) * commandPageSize
	end := min(len(items), start+commandPageSize)
	return items[start:end]
}

func (m model) selectedMentionCandidate() (mention.Candidate, bool) {
	if len(m.mentionResults) == 0 {
		return mention.Candidate{}, false
	}
	index := clamp(m.mentionCursor, 0, len(m.mentionResults)-1)
	return m.mentionResults[index], true
}

func (m model) visibleMentionItemsPage() []mention.Candidate {
	if len(m.mentionResults) == 0 {
		return nil
	}
	cursor := clamp(m.mentionCursor, 0, len(m.mentionResults)-1)
	start := (cursor / mentionPageSize) * mentionPageSize
	end := min(len(m.mentionResults), start+mentionPageSize)
	return m.mentionResults[start:end]
}

func (m model) selectedPromptSearchEntry() (history.PromptEntry, bool) {
	if len(m.promptSearchMatches) == 0 {
		return history.PromptEntry{}, false
	}
	index := clamp(m.promptSearchCursor, 0, len(m.promptSearchMatches)-1)
	return m.promptSearchMatches[index], true
}

func (m model) visiblePromptSearchEntriesPage() []history.PromptEntry {
	if len(m.promptSearchMatches) == 0 {
		return nil
	}
	cursor := clamp(m.promptSearchCursor, 0, len(m.promptSearchMatches)-1)
	start := (cursor / promptSearchPageSize) * promptSearchPageSize
	end := min(len(m.promptSearchMatches), start+promptSearchPageSize)
	return m.promptSearchMatches[start:end]
}

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func shouldExecuteFromPalette(item commandItem) bool {
	if item.Kind == "skill" {
		return true
	}
	switch item.Name {
	case "/help", "/session", "/skills", "/skill clear", "/new", "/compact", "/quit":
		return true
	default:
		return false
	}
}

func (m model) helpText() string {
	return strings.Join([]string{
		"Entry points",
		"Run `go run ./cmd/bytemind chat` from the repository root to open the TUI.",
		"The chat command opens the landing screen first, then enters the conversation view after you submit a prompt.",
		"Run `go run ./cmd/bytemind run -prompt \"...\"` for one-shot execution.",
		"",
		"Slash commands",
		"/help: show this help inside the conversation.",
		"/session: open recent sessions.",
		"/skills: list discovered skills and diagnostics.",
		"/<skill-name> [k=v...]: activate a skill for this session.",
		"/skill clear: clear the active skill.",
		"/new: start a fresh session.",
		"/compact: summarize long history into a compact continuation context.",
		"/btw <message>: interject while a run is in progress.",
		"/quit: exit the TUI.",
		"",
		"UI notes",
		"Tab toggles between Build and Plan modes.",
		"Plan mode keeps the plan panel visible and focused on structured steps.",
		"Use Ctrl+G to open or close the help panel.",
		"Use Ctrl+F to search prompt history and restore previous input.",
		"Drag across the conversation with the left mouse button to select text, then press Ctrl+C to copy it.",
		"If provider setup is required, paste an API key in the input and press Enter.",
		"Long pasted code/text is compressed to [Paste #N ~X lines].",
		"Use [Paste], [Paste #N], [Paste line3], or [Paste #N line3~line7] to expand references.",
		"After restoring a session with a saved plan, type 'continue execution' to resume it.",
		"Approval requests appear above the input area when a shell command needs confirmation.",
		"The footer keeps only the essential shortcuts: tab agents, / commands, drag select, Ctrl+C copy/quit, Ctrl+F history, Ctrl+L sessions.",
	}, "\n")
}
func visibleCommandItems(group string) []commandItem {
	items := make([]commandItem, 0, len(commandItems))
	for _, item := range commandItems {
		if group == "" {
			if item.Kind == "group" || item.Group == "" {
				items = append(items, item)
			}
			continue
		}
		if item.Kind == "command" && item.Group == group {
			items = append(items, item)
		}
	}
	return items
}

func (m model) isKnownSkillCommand(command string) bool {
	if m.runner == nil {
		return false
	}
	normalized := normalizeSkillCommand(command)
	if normalized == "" {
		return false
	}
	skillsList, _ := m.runner.ListSkills()
	for _, skill := range skillsList {
		if normalizeSkillCommand(skill.Name) == normalized {
			return true
		}
		if normalizeSkillCommand(skill.Entry.Slash) == normalized {
			return true
		}
		for _, alias := range skill.Aliases {
			if normalizeSkillCommand(alias) == normalized {
				return true
			}
		}
	}
	return false
}

func normalizeSkillCommand(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimLeft(name, "/")
	return strings.TrimSpace(name)
}

func commandFilterQuery(value, group string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	value = strings.TrimPrefix(value, "/")
	if group != "" {
		if strings.HasPrefix(value, group) {
			value = strings.TrimSpace(strings.TrimPrefix(value, group))
		}
	}
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "/")))
}

func matchesCommandItem(item commandItem, query string) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	name := strings.ToLower(strings.TrimPrefix(item.Name, "/"))
	usage := strings.ToLower(strings.TrimPrefix(item.Usage, "/"))
	return strings.HasPrefix(name, query) ||
		strings.HasPrefix(usage, query)
}

func matchAllTokens(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func (m model) chatPanelWidth() int {
	return max(20, m.width)
}

func (m model) chatPanelInnerWidth() int {
	width := m.chatPanelWidth() - panelStyle.GetHorizontalFrameSize()
	return max(12, width)
}

func (m model) chatInputContentWidth() int {
	width := m.chatPanelInnerWidth() - m.inputBorderStyle().GetHorizontalFrameSize()
	return max(18, width)
}

func (m model) landingInputShellWidth() int {
	return min(72, max(36, m.width/2))
}

func (m model) landingInputContentWidth() int {
	width := m.landingInputShellWidth() - landingInputStyle.GetHorizontalFrameSize()
	return max(24, width)
}

func (m model) inputBorderStyle() lipgloss.Style {
	return inputStyle.BorderForeground(m.modeAccentColor())
}

func (m model) modeAccentColor() lipgloss.Color {
	if m.mode == modePlan {
		return colorThinking
	}
	return colorAccent
}

func (m *model) syncInputStyle() {
	if m.startupGuide.Active {
		m.input.Placeholder = startupGuideInputPlaceholder(m.startupGuide.CurrentField)
	} else {
		m.input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	}
	m.input.Prompt = ""
	m.input.SetHeight(2)
}

func startupGuideInputHint(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Enter provider and press Enter."
	case startupFieldBaseURL:
		return "Enter base_url and press Enter."
	case startupFieldModel:
		return "Enter model and press Enter."
	case startupFieldAPIKey:
		return "Paste API key and press Enter to verify."
	default:
		return "Input value then press Enter."
	}
}

func startupGuideInputPlaceholder(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Step 1/4: provider (openai-compatible or anthropic)"
	case startupFieldBaseURL:
		return "Step 2/4: base_url (example: https://api.deepseek.com)"
	case startupFieldModel:
		return "Step 3/4: model (example: deepseek-chat)"
	case startupFieldAPIKey:
		return "Step 4/4: paste API key and press Enter..."
	default:
		return "Complete setup and press Enter..."
	}
}
