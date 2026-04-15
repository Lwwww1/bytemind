package agent

import (
	"io"

	"bytemind/internal/session"
)

// TurnRequest defines the minimum input needed to execute one agent turn.
type TurnRequest struct {
	Session *session.Session
	Input   RunPromptInput
	Mode    string
	Out     io.Writer
}

// TurnEventType identifies engine turn event categories.
type TurnEventType string

const (
	TurnEventStarted   TurnEventType = "started"
	TurnEventCompleted TurnEventType = "completed"
	TurnEventFailed    TurnEventType = "failed"
)

// TurnEvent is the minimal compatibility event emitted by Engine.
type TurnEvent struct {
	Type   TurnEventType
	Answer string
	Error  error
}
