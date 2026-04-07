package tui

import (
	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/session"
	"os"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	Runner       *agent.Runner
	Store        *session.Store
	Session      *session.Session
	ImageStore   assets.ImageStore
	Config       config.Config
	Workspace    string
	StartupGuide StartupGuide
}

type StartupGuide struct {
	Active       bool
	Title        string
	Status       string
	Lines        []string
	ConfigPath   string
	CurrentField string
}

func Run(opts Options) error {
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if shouldUseInputTTY() {
		// Keep direct console input opt-in on Windows.
		// It can help mouse reporting in some terminals, but it may break CJK/IME input.
		programOptions = append(programOptions, tea.WithInputTTY())
	}
	if shouldEnableMouseCapture() {
		programOptions = append(programOptions, tea.WithMouseAllMotion())
	}
	program := tea.NewProgram(newModel(opts), programOptions...)
	_, err := program.Run()
	return err
}

func shouldEnableMouseCapture() bool {
	return parseMouseCaptureEnv(os.Getenv("BYTEMIND_ENABLE_MOUSE"))
}

func shouldUseInputTTY() bool {
	return runtime.GOOS == "windows" && parseInputTTYEnv(os.Getenv("BYTEMIND_WINDOWS_INPUT_TTY"))
}

func parseMouseCaptureEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func parseInputTTYEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "", "0", "false", "no", "off":
		return false
	default:
		return false
	}
}
