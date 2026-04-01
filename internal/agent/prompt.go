package agent

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	planpkg "bytemind/internal/plan"
)

//go:embed prompts/core.md
var corePromptSource string

//go:embed prompts/mode-build.md
var buildModePromptSource string

//go:embed prompts/mode-plan.md
var planModePromptSource string

//go:embed prompts/block-environment.md
var environmentPromptSource string

//go:embed prompts/block-plan.md
var planPromptSource string

//go:embed prompts/block-repo-rules.md
var repoRulesPromptSource string

//go:embed prompts/block-skills-summary.md
var skillsPromptSource string

//go:embed prompts/block-output-contract.md
var outputContractPromptSource string

type PromptSkill struct {
	Name        string
	Description string
	Enabled     bool
}

type PromptInput struct {
	Workspace        string
	ApprovalPolicy   string
	ProviderType     string
	Model            string
	MaxIterations    int
	Mode             string
	Platform         string
	Now              time.Time
	Plan             planpkg.State
	RepoRulesSummary string
	Skills           []PromptSkill
	OutputContract   string
}

func systemPrompt(input PromptInput) string {
	parts := []string{
		strings.TrimSpace(corePromptSource),
		strings.TrimSpace(modePrompt(input.Mode)),
		renderEnvironmentPrompt(input),
		renderPlanPrompt(input.Plan),
		renderRepoRulesPrompt(input.RepoRulesSummary),
		renderSkillsPrompt(input.Skills),
		renderOutputContractPrompt(input.OutputContract),
	}
	return strings.Join(filterPromptParts(parts), "\n\n")
}

func modePrompt(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan":
		return planModePromptSource
	default:
		return buildModePromptSource
	}
}

func renderEnvironmentPrompt(input PromptInput) string {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = runtime.GOOS + "/" + runtime.GOARCH
	}

	providerType := strings.TrimSpace(input.ProviderType)
	if providerType == "" {
		providerType = "openai-compatible"
	}

	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "build"
	}

	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "unknown"
	}

	replacer := strings.NewReplacer(
		"{{CWD}}", input.Workspace,
		"{{WORKSPACE}}", input.Workspace,
		"{{PLATFORM}}", platform,
		"{{DATE}}", now.Format("2006-01-02"),
		"{{APPROVAL_POLICY}}", input.ApprovalPolicy,
		"{{MODE}}", mode,
		"{{PROVIDER_TYPE}}", providerType,
		"{{MODEL}}", model,
		"{{MAX_ITERATIONS}}", fmt.Sprintf("%d", input.MaxIterations),
	)
	return replacer.Replace(strings.TrimSpace(environmentPromptSource))
}

func renderPlanPrompt(state planpkg.State) string {
	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) {
		return ""
	}

	lines := make([]string, 0, len(state.Steps)+12)
	if state.Goal != "" {
		lines = append(lines, "Goal: "+state.Goal)
	}
	if state.Summary != "" {
		lines = append(lines, "Summary: "+state.Summary)
	}
	if state.Phase != "" && state.Phase != planpkg.PhaseNone {
		lines = append(lines, "Phase: "+string(state.Phase))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}

	completed := make([]planpkg.Step, 0, 3)
	upcoming := make([]planpkg.Step, 0, 4)
	var current planpkg.Step
	var hasCurrent bool
	for _, step := range state.Steps {
		switch planpkg.NormalizeStepStatus(string(step.Status)) {
		case planpkg.StepCompleted:
			completed = append(completed, step)
			if len(completed) > 3 {
				completed = completed[len(completed)-3:]
			}
		case planpkg.StepInProgress, planpkg.StepBlocked:
			if !hasCurrent {
				current = step
				hasCurrent = true
			}
		default:
			if len(upcoming) < 4 {
				upcoming = append(upcoming, step)
			}
		}
	}
	if len(completed) > 0 {
		lines = append(lines, "Recently completed:")
		for _, step := range completed {
			lines = append(lines, fmt.Sprintf("- [%s] %s", step.Status, step.Title))
		}
		lines = append(lines, "")
	}
	if hasCurrent {
		lines = append(lines, "Current step:")
		lines = append(lines, fmt.Sprintf("- [%s] %s", current.Status, current.Title))
		if len(current.Files) > 0 {
			lines = append(lines, "  files: "+strings.Join(current.Files, ", "))
		}
		if len(current.Verify) > 0 {
			lines = append(lines, "  verify: "+strings.Join(current.Verify, " | "))
		}
		if current.Risk != "" {
			lines = append(lines, "  risk: "+string(current.Risk))
		}
		if current.Description != "" {
			lines = append(lines, "  note: "+current.Description)
		}
		lines = append(lines, "")
	}
	if len(upcoming) > 0 {
		lines = append(lines, "Upcoming:")
		for _, step := range upcoming {
			lines = append(lines, fmt.Sprintf("- [%s] %s", step.Status, step.Title))
		}
		lines = append(lines, "")
	}
	if state.NextAction != "" {
		lines = append(lines, "Next Action: "+state.NextAction)
	}
	if state.BlockReason != "" {
		lines = append(lines, "Blocked Reason: "+state.BlockReason)
	}

	return strings.ReplaceAll(strings.TrimSpace(planPromptSource), "{{PLAN_ITEMS}}", strings.Join(lines, "\n"))
}

func renderRepoRulesPrompt(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(repoRulesPromptSource), "{{REPO_RULES_SUMMARY}}", summary)
}

func renderSkillsPrompt(skills []PromptSkill) string {
	if len(skills) == 0 {
		return ""
	}

	lines := make([]string, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		description := strings.TrimSpace(skill.Description)
		if name == "" || description == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s enabled=%t", name, description, skill.Enabled))
	}
	if len(lines) == 0 {
		return ""
	}

	return strings.ReplaceAll(strings.TrimSpace(skillsPromptSource), "{{SKILLS_SUMMARY}}", strings.Join(lines, "\n"))
}

func renderOutputContractPrompt(contract string) string {
	contract = strings.TrimSpace(contract)
	if contract == "" {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(outputContractPromptSource), "{{OUTPUT_CONTRACT}}", contract)
}

func filterPromptParts(parts []string) []string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}
