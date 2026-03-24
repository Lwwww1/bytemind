package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gocode/internal/llm"
	"gocode/internal/session"
	"gocode/internal/tools"
)

type ConfirmFunc func(prompt string) (bool, error)

type Agent struct {
	client   *llm.Client
	runtime  *tools.Runtime
	store    *session.Store
	confirm  ConfirmFunc
	maxTurns int
}

type taskPlan struct {
	Summary    string   `json:"summary"`
	Steps      []string `json:"steps"`
	Validation []string `json:"validation"`
}

func New(client *llm.Client, runtime *tools.Runtime, store *session.Store, confirm ConfirmFunc, maxTurns int) *Agent {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	return &Agent{
		client:   client,
		runtime:  runtime,
		store:    store,
		confirm:  confirm,
		maxTurns: maxTurns,
	}
}

func (a *Agent) RunTask(ctx context.Context, input string) (session.TaskRecord, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return session.TaskRecord{}, nil
	}

	a.store.BeginTask(input)
	a.runtime.BeginTask(input)

	plan, planErr := a.planTask(ctx, input)
	if plan.Summary != "" {
		a.store.SetSummary(plan.Summary)
	}
	a.store.SetPlan(plan.Steps)
	if planErr != nil {
		return a.store.FailCurrentTask("规划失败: " + planErr.Error()), planErr
	}

	messages := executorMessages(a.runtime.Workspace(), a.store.HistoryDigest(3), plan, input)
	toolDefs := a.runtime.Definitions()

	for turn := 0; turn < a.maxTurns; turn++ {
		message, err := a.client.Chat(ctx, messages, toolDefs)
		if err != nil {
			return a.store.FailCurrentTask("执行失败: " + err.Error()), err
		}

		messages = append(messages, message)
		if len(message.ToolCalls) == 0 {
			finalNote := strings.TrimSpace(message.Content)
			if finalNote == "" {
				finalNote = "任务已执行完成。"
			}
			a.store.SetAssistant(finalNote)
			return a.store.CompleteTask("completed"), nil
		}

		for _, call := range message.ToolCalls {
			a.store.AddToolCall(call.Function.Name, call.Function.Arguments)
			result, execErr := a.runtime.Execute(ctx, call.Function.Name, []byte(call.Function.Arguments), tools.ConfirmFunc(a.confirm))
			if execErr != nil {
				result = encodeToolError(execErr)
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				Name:       call.Function.Name,
				ToolCallID: call.ID,
				Content:    result,
			})
		}
	}

	err := fmt.Errorf("agent exceeded max turns (%d)", a.maxTurns)
	return a.store.FailCurrentTask(err.Error()), err
}

func (a *Agent) planTask(ctx context.Context, input string) (taskPlan, error) {
	message, err := a.client.Chat(ctx, plannerMessages(a.runtime.Workspace(), a.store.HistoryDigest(3), input), nil)
	if err != nil {
		return fallbackPlan(input), err
	}

	plan := fallbackPlan(input)
	content := strings.TrimSpace(message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if content == "" {
		return plan, nil
	}
	if decodeErr := json.Unmarshal([]byte(content), &plan); decodeErr != nil {
		return fallbackPlan(input), nil
	}
	if strings.TrimSpace(plan.Summary) == "" {
		plan.Summary = input
	}
	if len(plan.Steps) == 0 {
		plan.Steps = fallbackPlan(input).Steps
	}
	return plan, nil
}

func fallbackPlan(input string) taskPlan {
	return taskPlan{
		Summary: input,
		Steps: []string{
			"Inspect the workspace and gather the files needed for the task.",
			"Create or modify code in focused steps.",
			"Run a targeted validation command when it materially checks the result.",
		},
	}
}

func encodeToolError(err error) string {
	payload, marshalErr := json.Marshal(map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
	if marshalErr != nil {
		return fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error())
	}
	return string(payload)
}
