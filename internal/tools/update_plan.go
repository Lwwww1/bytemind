package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
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
					"goal": map[string]any{"type": "string"},
					"summary": map[string]any{"type": "string"},
					"phase": map[string]any{"type": "string", "enum": []string{"none", "drafting", "ready", "approved", "executing", "blocked", "completed"}},
					"next_action": map[string]any{"type": "string"},
					"block_reason": map[string]any{"type": "string"},
					"explanation": map[string]any{
						"type":        "string",
						"description": "Optional short explanation of why the plan changed.",
					},
					"plan": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":          map[string]any{"type": "string"},
								"step":        map[string]any{"type": "string"},
								"title":       map[string]any{"type": "string"},
								"description": map[string]any{"type": "string"},
								"status":      map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "blocked"}},
								"files": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
								"verify": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
								"risk": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
							},
							"required": []string{"status"},
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
		Goal        string `json:"goal"`
		Summary     string `json:"summary"`
		Phase       string `json:"phase"`
		NextAction  string `json:"next_action"`
		BlockReason string `json:"block_reason"`
		Explanation string `json:"explanation"`
		Plan        []struct {
			ID          string   `json:"id"`
			Step        string   `json:"step"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Files       []string `json:"files"`
			Verify      []string `json:"verify"`
			Risk        string   `json:"risk"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if len(args.Plan) == 0 {
		return "", errors.New("plan must contain at least one step")
	}

	steps := make([]planpkg.Step, 0, len(args.Plan))
	inProgressCount := 0
	for i, item := range args.Plan {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(item.Step)
		}
		status := planpkg.NormalizeStepStatus(item.Status)
		if title == "" {
			return "", errors.New("plan step title cannot be empty")
		}
		if status == planpkg.StepInProgress {
			inProgressCount++
		}
		step := planpkg.Step{
			ID:          strings.TrimSpace(item.ID),
			Title:       title,
			Description: strings.TrimSpace(item.Description),
			Status:      status,
			Files:       trimPlanStrings(item.Files),
			Verify:      trimPlanStrings(item.Verify),
			Risk:        normalizeRisk(item.Risk),
		}
		if step.ID == "" {
			step.ID = fmt.Sprintf("s%d", i+1)
		}
		steps = append(steps, step)
	}
	if inProgressCount > 1 {
		return "", errors.New("only one plan item can be in_progress")
	}

	state := execCtx.Session.Plan
	state.Goal = chooseNonEmpty(strings.TrimSpace(args.Goal), state.Goal)
	state.Summary = chooseNonEmpty(strings.TrimSpace(args.Summary), strings.TrimSpace(args.Explanation), state.Summary)
	state.NextAction = chooseNonEmpty(strings.TrimSpace(args.NextAction), state.NextAction)
	state.BlockReason = chooseNonEmpty(strings.TrimSpace(args.BlockReason), state.BlockReason)
	state.Steps = steps
	state.Phase = planpkg.NormalizePhase(args.Phase)
	if state.Phase == planpkg.PhaseNone {
		state.Phase = planpkg.DerivePhase(execCtx.Mode, steps, state.BlockReason)
	}
	state.UpdatedAt = time.Now().UTC()
	state = planpkg.NormalizeState(state)
	if state.NextAction == "" {
		state.NextAction = planpkg.DefaultNextAction(state)
	}

	validation := planpkg.ValidateState(state)
	if !validation.OK {
		return "", errors.New(strings.Join(validation.Warnings, "; "))
	}

	execCtx.Session.Plan = state
	return toJSON(map[string]any{
		"ok":          true,
		"explanation": strings.TrimSpace(args.Explanation),
		"plan":        state,
	})
}

func trimPlanStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRisk(raw string) planpkg.RiskLevel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return planpkg.NormalizeRisk(raw)
}

func chooseNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}