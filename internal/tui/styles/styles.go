package styles

import "github.com/charmbracelet/lipgloss"

var (
	ColorPanel    = lipgloss.Color("#000000")
	ColorBorder   = lipgloss.Color("#314156")
	ColorAccent   = lipgloss.Color("#6CB6FF")
	ColorCard     = lipgloss.Color("#171717")
	ColorHotPink  = lipgloss.Color("#F05AA6")
	ColorThinking = lipgloss.Color("#9D8AC8")
	ColorUser     = lipgloss.Color("#F59E0B")
	ColorTool     = lipgloss.Color("#BEA15A")
	ColorMuted    = lipgloss.Color("#93A4B8")
	ColorDanger   = lipgloss.Color("#F7A8A8")
	ColorSuccess  = lipgloss.Color("#8EE6A0")
)

var (
	PanelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237")).
			Background(lipgloss.Color("#000000")).
			Padding(0, 1)

	LandingCanvasStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#000000"))

	LandingLogoByteStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#7A7A7A", Dark: "#8A8A8A"}).
				Bold(false).
				Align(lipgloss.Center)

	LandingLogoMindStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Align(lipgloss.Center)

	LandingLogoStyle = LandingLogoMindStyle

	LandingInputStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1A1A1A")).
				Padding(1, 4)

	LandingPlaceholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7C7C7C"))

	LandingInputValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EAEAEA"))

	LandingModeStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	LandingModelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B9B9B9"))

	LandingHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#6C6C6C", Dark: "#565656"})

	LandingTipDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))

	LandingTipLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#BEBEBE")).
				Bold(true)

	LandingTipTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A4A4A4"))

	FooterHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "246", Dark: "240"})

	FooterHintKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)

	FooterHintLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8D97A6"))

	FooterHintDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5C6675"))

	ModeBuildActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6CB6FF")).
				Bold(true)

	ModePlanActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A78BFA")).
				Bold(true)

	ModeInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280"))

	HelpHeadingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D9E6F2")).
				Bold(true)

	HelpCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F1F5F9")).
			Background(lipgloss.Color("#101010")).
			Padding(0, 1)

	InputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	ModeTabStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1).
			MarginRight(1)

	CardTitleStyle = lipgloss.NewStyle().Bold(true)

	ChatBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D8D8D8"))

	SelectionToastStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DFF7E8")).
				Background(lipgloss.Color("#163423")).
				Padding(0, 1).
				Bold(true)

	SelectionHighlightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0B1118")).
				Background(lipgloss.Color("#9CCBFF"))

	AssistantHeading1Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#EAF6FF"))

	AssistantHeading2Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#CDEBFF"))

	AssistantHeading3Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#A9D8FF"))

	ListMarkerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	QuoteLineStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(ColorMuted).
			PaddingLeft(1).
			Foreground(lipgloss.Color("#D7E3F1"))

	TableLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D7E3F1")).
			Background(lipgloss.Color("#101923"))

	ChatAssistantStyle = lipgloss.NewStyle().
				Padding(1, 1)

	ChatThinkingStyle = lipgloss.NewStyle().
				Padding(1, 1)

	ThinkingBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7F758F")).
				Faint(true)

	ChatUserStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2A2A2A")).
			Padding(1, 1)

	ChatToolStyle = lipgloss.NewStyle().
			Padding(1, 1)

	ToolBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8893A1")).
			Faint(true)

	ChatSystemStyle = lipgloss.NewStyle().
			Padding(1, 1)

	ApprovalBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorTool).
				Background(lipgloss.Color("#17140D")).
				Padding(0, 1)

	ActiveSkillBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorAccent).
				Background(lipgloss.Color("#0F1A28")).
				Padding(0, 1)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Faint(true)

	CommandPaletteStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(false).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	CommandPaletteRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#0B1118")).
				Padding(0, 1)

	CommandPaletteSelectedRowStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#231421")).
					Padding(0, 1)

	CommandPaletteNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B7C4FF"))

	CommandPaletteSelectedNameStyle = lipgloss.NewStyle().
					Foreground(ColorHotPink).
					Bold(true)

	CommandPaletteDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A9B7C6"))

	CommandPaletteSelectedDescStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#F7D9EA"))

	CommandPaletteMetaStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	ScrollbarTrackStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1F1F1F"))

	ScrollbarThumbIdleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#262626"))

	ScrollbarThumbActiveStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#4D4D4D"))

	ModalBoxStyle = lipgloss.NewStyle().
			Background(ColorPanel).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	CodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8FAFC")).
			Background(lipgloss.Color("#0B1220")).
			Padding(0, 1)

	MutedStyle  = lipgloss.NewStyle().Foreground(ColorMuted)
	AccentStyle = lipgloss.NewStyle().Foreground(ColorAccent)
	DoneStyle   = lipgloss.NewStyle().Foreground(ColorSuccess)
	WarnStyle   = lipgloss.NewStyle().Foreground(ColorTool)
	ErrorStyle  = lipgloss.NewStyle().Foreground(ColorDanger)
)

func Spacer(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(width).Render("")
}
