package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aicoding/internal/llm"
	"aicoding/internal/session"
)

type UpdatePlanTool struct{}

func (UpdatePlanTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "update_plan",
			Description: "Update the task plan for multi-step work. Use it when a task has several meaningful steps.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"explanation": map[string]any{
						"type":        "string",
						"description": "Optional short explanation of why the plan changed.",
					},
					"plan": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"step":   map[string]any{"type": "string"},
								"status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
							},
							"required": []string{"step", "status"},
						},
					},
				},
				"required": []string{"plan"},
			},
		},
	}
}

func (UpdatePlanTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	if execCtx.Session == nil {
		return "", errors.New("session is required for update_plan")
	}

	var args struct {
		Explanation string `json:"explanation"`
		Plan        []struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if len(args.Plan) == 0 {
		return "", errors.New("plan must contain at least one step")
	}

	items := make([]session.PlanItem, 0, len(args.Plan))
	inProgressCount := 0
	for _, item := range args.Plan {
		step := strings.TrimSpace(item.Step)
		status := strings.TrimSpace(item.Status)
		if step == "" {
			return "", errors.New("plan step cannot be empty")
		}
		switch status {
		case "pending", "in_progress", "completed":
		default:
			return "", errors.New("invalid plan status")
		}
		if status == "in_progress" {
			inProgressCount++
		}
		items = append(items, session.PlanItem{Step: step, Status: status})
	}
	if inProgressCount > 1 {
		return "", errors.New("only one plan item can be in_progress")
	}

	execCtx.Session.Plan = items
	return toJSON(map[string]any{
		"ok":          true,
		"explanation": strings.TrimSpace(args.Explanation),
		"plan":        items,
	})
}
