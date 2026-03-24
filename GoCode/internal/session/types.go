package session

import "time"

type ToolCall struct {
	Name string
	Args string
	At   time.Time
}

type Change struct {
	Action string
	Path   string
	Detail string
}

type CommandResult struct {
	Command  string
	Cwd      string
	ExitCode int
	Output   string
	At       time.Time
}

type TaskRecord struct {
	Input      string
	Summary    string
	Plan       []string
	ToolCalls  []ToolCall
	Files      []string
	Changes    []Change
	Commands   []CommandResult
	Assistant  string
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
}
