package tui

import tuistyles "bytemind/internal/tui/styles"

var (
	colorPanel    = tuistyles.ColorPanel
	colorBorder   = tuistyles.ColorBorder
	colorAccent   = tuistyles.ColorAccent
	colorCard     = tuistyles.ColorCard
	colorHotPink  = tuistyles.ColorHotPink
	colorThinking = tuistyles.ColorThinking
	colorUser     = tuistyles.ColorUser
	colorTool     = tuistyles.ColorTool
	colorMuted    = tuistyles.ColorMuted
	colorDanger   = tuistyles.ColorDanger
	colorSuccess  = tuistyles.ColorSuccess
)

var (
	panelStyle                      = tuistyles.PanelStyle
	landingCanvasStyle              = tuistyles.LandingCanvasStyle
	landingLogoByteStyle            = tuistyles.LandingLogoByteStyle
	landingLogoMindStyle            = tuistyles.LandingLogoMindStyle
	landingLogoStyle                = tuistyles.LandingLogoStyle
	landingInputStyle               = tuistyles.LandingInputStyle
	landingPlaceholderStyle         = tuistyles.LandingPlaceholderStyle
	landingInputValueStyle          = tuistyles.LandingInputValueStyle
	landingModeStyle                = tuistyles.LandingModeStyle
	landingModelStyle               = tuistyles.LandingModelStyle
	landingHintStyle                = tuistyles.LandingHintStyle
	landingTipDotStyle              = tuistyles.LandingTipDotStyle
	landingTipLabelStyle            = tuistyles.LandingTipLabelStyle
	landingTipTextStyle             = tuistyles.LandingTipTextStyle
	footerHintStyle                 = tuistyles.FooterHintStyle
	footerHintKeyStyle              = tuistyles.FooterHintKeyStyle
	footerHintLabelStyle            = tuistyles.FooterHintLabelStyle
	footerHintDividerStyle          = tuistyles.FooterHintDividerStyle
	modeBuildActiveStyle            = tuistyles.ModeBuildActiveStyle
	modePlanActiveStyle             = tuistyles.ModePlanActiveStyle
	modeInactiveStyle               = tuistyles.ModeInactiveStyle
	helpHeadingStyle                = tuistyles.HelpHeadingStyle
	helpCodeStyle                   = tuistyles.HelpCodeStyle
	inputStyle                      = tuistyles.InputStyle
	modeTabStyle                    = tuistyles.ModeTabStyle
	cardTitleStyle                  = tuistyles.CardTitleStyle
	chatBodyStyle                   = tuistyles.ChatBodyStyle
	selectionToastStyle             = tuistyles.SelectionToastStyle
	selectionHighlightStyle         = tuistyles.SelectionHighlightStyle
	assistantHeading1Style          = tuistyles.AssistantHeading1Style
	assistantHeading2Style          = tuistyles.AssistantHeading2Style
	assistantHeading3Style          = tuistyles.AssistantHeading3Style
	listMarkerStyle                 = tuistyles.ListMarkerStyle
	quoteLineStyle                  = tuistyles.QuoteLineStyle
	tableLineStyle                  = tuistyles.TableLineStyle
	chatAssistantStyle              = tuistyles.ChatAssistantStyle
	chatThinkingStyle               = tuistyles.ChatThinkingStyle
	thinkingBodyStyle               = tuistyles.ThinkingBodyStyle
	chatUserStyle                   = tuistyles.ChatUserStyle
	chatToolStyle                   = tuistyles.ChatToolStyle
	toolBodyStyle                   = tuistyles.ToolBodyStyle
	chatSystemStyle                 = tuistyles.ChatSystemStyle
	approvalBannerStyle             = tuistyles.ApprovalBannerStyle
	activeSkillBannerStyle          = tuistyles.ActiveSkillBannerStyle
	statusBarStyle                  = tuistyles.StatusBarStyle
	commandPaletteStyle             = tuistyles.CommandPaletteStyle
	commandPaletteRowStyle          = tuistyles.CommandPaletteRowStyle
	commandPaletteSelectedRowStyle  = tuistyles.CommandPaletteSelectedRowStyle
	commandPaletteNameStyle         = tuistyles.CommandPaletteNameStyle
	commandPaletteSelectedNameStyle = tuistyles.CommandPaletteSelectedNameStyle
	commandPaletteDescStyle         = tuistyles.CommandPaletteDescStyle
	commandPaletteSelectedDescStyle = tuistyles.CommandPaletteSelectedDescStyle
	commandPaletteMetaStyle         = tuistyles.CommandPaletteMetaStyle
	scrollbarTrackStyle             = tuistyles.ScrollbarTrackStyle
	scrollbarThumbIdleStyle         = tuistyles.ScrollbarThumbIdleStyle
	scrollbarThumbActiveStyle       = tuistyles.ScrollbarThumbActiveStyle
	modalBoxStyle                   = tuistyles.ModalBoxStyle
	modalTitleStyle                 = tuistyles.ModalTitleStyle
	codeStyle                       = tuistyles.CodeStyle
	mutedStyle                      = tuistyles.MutedStyle
	accentStyle                     = tuistyles.AccentStyle
	doneStyle                       = tuistyles.DoneStyle
	warnStyle                       = tuistyles.WarnStyle
	errorStyle                      = tuistyles.ErrorStyle
)

func spacer(width int) string {
	return tuistyles.Spacer(width)
}
