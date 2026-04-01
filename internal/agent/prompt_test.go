package agent

import (
	"strings"
	"testing"
	"time"

	planpkg "bytemind/internal/plan"
)

func TestSystemPromptRendersConfiguredBlocks(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "on-request",
		ProviderType:   "openai-compatible",
		Model:          "gpt-5.4-mini",
		MaxIterations:  32,
		Mode:           "build",
		Platform:       "linux/amd64",
		Now:            time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Plan: planpkg.State{
			Goal:    "Rewrite prompt architecture",
			Summary: "Keep the prompt composable and mode-aware.",
			Phase:   planpkg.PhaseExecuting,
			Steps: []planpkg.Step{
				{Title: "Inspect relevant files", Status: planpkg.StepCompleted},
				{Title: "Rewrite prompt architecture", Status: planpkg.StepInProgress, Files: []string{"internal/agent/prompt.go"}, Verify: []string{"go test ./internal/agent"}, Risk: planpkg.RiskMedium},
			},
			NextAction: "Continue: Rewrite prompt architecture",
		},
		RepoRulesSummary: "- Prefer Go standard library when practical.",
		Skills: []PromptSkill{
			{Name: "review", Description: "Review code changes for bugs and regressions.", Enabled: true},
		},
		OutputContract: "{\"summary\": string}",
	})

	assertContains(t, prompt, "You are ByteMind")
	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "build")
	assertContains(t, prompt, "/tmp/workspace")
	assertContains(t, prompt, "on-request")
	assertContains(t, prompt, "gpt-5.4-mini")
	assertContains(t, prompt, "linux/amd64")
	assertContains(t, prompt, "2026-03-31")
	assertContains(t, prompt, "Goal: Rewrite prompt architecture")
	assertContains(t, prompt, "- [completed] Inspect relevant files")
	assertContains(t, prompt, "- [in_progress] Rewrite prompt architecture")
	assertContains(t, prompt, "[Current Execution Plan]")
	assertContains(t, prompt, "[Repo Rules]")
	assertContains(t, prompt, "[Available Skills]")
	assertContains(t, prompt, "[Output Contract]")

	assertNoTemplateMarkers(t, prompt)
}

func TestSystemPromptOmitsOptionalBlocksWhenEmpty(t *testing.T) {
	prompt := systemPrompt(PromptInput{
		Workspace:      "/tmp/workspace",
		ApprovalPolicy: "never",
		ProviderType:   "anthropic",
		Model:          "claude-sonnet-4",
		MaxIterations:  16,
		Mode:           "plan",
		Platform:       "darwin/arm64",
		Now:            time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})

	assertContains(t, prompt, "[Current Mode]")
	assertContains(t, prompt, "plan")
	assertContains(t, prompt, "Required final answer structure:")
	assertContains(t, prompt, "Plan")
	assertContains(t, prompt, "Risks")
	assertContains(t, prompt, "Verification")
	assertContains(t, prompt, "Next Action")
	if strings.Contains(prompt, "[Current Plan]") {
		t.Fatalf("did not expect plan block in prompt: %q", prompt)
	}
	if strings.Contains(prompt, "[Repo Rules]") {
		t.Fatalf("did not expect repo rules block in prompt: %q", prompt)
	}
	if strings.Contains(prompt, "[Available Skills]") {
		t.Fatalf("did not expect skills block in prompt: %q", prompt)
	}
	if strings.Contains(prompt, "[Output Contract]") {
		t.Fatalf("did not expect output contract block in prompt: %q", prompt)
	}
}

func assertContains(t *testing.T, prompt, needle string) {
	t.Helper()
	if !strings.Contains(prompt, needle) {
		t.Fatalf("expected %q in prompt, got %q", needle, prompt)
	}
}

func assertNoTemplateMarkers(t *testing.T, prompt string) {
	t.Helper()
	markers := []string{
		"{{WORKSPACE}}",
		"{{APPROVAL_POLICY}}",
		"{{PLAN_ITEMS}}",
		"{{REPO_RULES_SUMMARY}}",
		"{{SKILLS_SUMMARY}}",
		"{{OUTPUT_CONTRACT}}",
	}
	for _, marker := range markers {
		if strings.Contains(prompt, marker) {
			t.Fatalf("expected template marker %q to be rendered, got %q", marker, prompt)
		}
	}
}
