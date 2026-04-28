package agent

import (
	"testing"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

func TestLocalRepoClaimRepairRequiresDirectPathEvidence(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("Check whether the config implementation exists."),
	}
	reply := llm.NewAssistantTextMessage("The implementation already exists in internal/config/config.go.")

	kind, _ := evaluateLocalRepoClaimRepairTurn(planpkg.ModeBuild, "", reply, messages)

	if kind != localRepoClaimRepairPathUnverified {
		t.Fatalf("expected path-unverified repair, got %v", kind)
	}
}

func TestLocalRepoClaimRepairAcceptsApplyPatchAsDirectEvidence(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("Change the default max iterations."),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "apply_patch",
						Arguments: `{"patch":"*** Begin Patch\n*** Update File: internal/config/config.go\n@@\n-\tMaxIterations: 32,\n+\tMaxIterations: 64,\n*** End Patch"}`,
					},
				},
			},
		},
		llm.NewToolResultMessage("call_1", `{"ok":true}`),
	}
	reply := llm.NewAssistantTextMessage("The implementation already exists in internal/config/config.go.")

	kind, _ := evaluateLocalRepoClaimRepairTurn(planpkg.ModeBuild, "", reply, messages)

	if kind != localRepoClaimRepairNone {
		t.Fatalf("expected no repair after apply_patch evidence, got %v", kind)
	}
}
