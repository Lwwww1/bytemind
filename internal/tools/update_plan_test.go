package tools

import (
	"context"
	"encoding/json"
	"testing"

	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
)

func TestUpdatePlanToolUpdatesSessionPlan(t *testing.T) {
	workspace := t.TempDir()
	sess := session.New(workspace)
	tool := UpdatePlanTool{}
	payload, _ := json.Marshal(map[string]any{
		"explanation": "starting work",
		"plan": []map[string]any{
			{"step": "Inspect provider", "status": "completed"},
			{"step": "Add tests", "status": "in_progress"},
		},
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Session: sess})
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Plan.Steps) != 2 || sess.Plan.Steps[1].Title != "Add tests" || sess.Plan.Steps[1].Status != planpkg.StepInProgress {
		t.Fatalf("unexpected session plan %#v", sess.Plan)
	}

	var parsed struct {
		Explanation string        `json:"explanation"`
		Plan        planpkg.State `json:"plan"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Explanation != "starting work" || len(parsed.Plan.Steps) != 2 {
		t.Fatalf("unexpected result %#v", parsed)
	}
}

func TestUpdatePlanToolRequiresSession(t *testing.T) {
	tool := UpdatePlanTool{}
	payload := []byte(`{"plan":[{"step":"x","status":"pending"}]}`)
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected missing session error")
	}
}

func TestUpdatePlanToolRejectsInvalidPlanShapes(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty plan",
			payload: `{"plan":[]}`,
		},
		{
			name:    "empty step",
			payload: `{"plan":[{"step":" ","status":"pending"}]}`,
		},
		{
			name:    "multiple in progress",
			payload: `{"plan":[{"step":"x","status":"in_progress"},{"step":"y","status":"in_progress"}]}`,
		},
	}

	tool := UpdatePlanTool{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := session.New(t.TempDir())
			_, err := tool.Run(context.Background(), []byte(tt.payload), &ExecutionContext{Session: sess})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
