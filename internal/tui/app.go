package tui

import (
	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/session"

	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	Runner    *agent.Runner
	Store     *session.Store
	Session   *session.Session
	Config    config.Config
	Workspace string
}

func Run(opts Options) error {
	program := tea.NewProgram(newModel(opts), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}
