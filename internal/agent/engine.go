package agent

import (
	"context"
	"errors"
)

// Engine executes one turn and emits turn-scoped events.
type Engine interface {
	HandleTurn(ctx context.Context, req TurnRequest) (<-chan TurnEvent, error)
}

type defaultEngine struct {
	runner *Runner
}

// NewDefaultEngine wires the default engine implementation backed by Runner internals.
func NewDefaultEngine(runner *Runner) Engine {
	return &defaultEngine{runner: runner}
}

func (e *defaultEngine) HandleTurn(ctx context.Context, req TurnRequest) (<-chan TurnEvent, error) {
	if e == nil || e.runner == nil {
		return nil, errors.New("agent engine is unavailable")
	}

	events := make(chan TurnEvent, 2)
	go func() {
		defer close(events)

		events <- TurnEvent{Type: TurnEventStarted}

		if req.Session == nil {
			events <- TurnEvent{
				Type:  TurnEventFailed,
				Error: errors.New("session is required"),
			}
			return
		}

		setup, err := e.runner.prepareRunPrompt(req.Session, req.Input, req.Mode)
		if err != nil {
			events <- TurnEvent{
				Type:  TurnEventFailed,
				Error: err,
			}
			return
		}

		answer, err := e.runner.runPromptTurns(ctx, req.Session, setup, req.Out)
		if err != nil {
			events <- TurnEvent{
				Type:  TurnEventFailed,
				Error: err,
			}
			return
		}

		events <- TurnEvent{
			Type:   TurnEventCompleted,
			Answer: answer,
		}
	}()

	return events, nil
}
