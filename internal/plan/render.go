package plan

import (
	"fmt"
	"strings"
)

func RenderStructuredPlanBlock(state State) string {
	state = NormalizeState(state)
	if !ShouldRenderStructuredPlanBlock(state) {
		return ""
	}

	lines := []string{"<proposed_plan>"}
	if goal := strings.TrimSpace(state.Goal); goal != "" {
		lines = appendSection(lines, "Goal", []string{"- " + goal})
	}
	if summary := strings.TrimSpace(state.Summary); summary != "" {
		lines = appendSection(lines, "Summary", []string{"- " + summary})
	}
	if brief := strings.TrimSpace(state.ImplementationBrief); brief != "" {
		lines = appendSection(lines, "Implementation Brief", splitDocumentLines(brief))
	}
	if phase := NormalizePhase(string(state.Phase)); phase != PhaseNone {
		lines = appendSection(lines, "Phase", []string{"- " + string(phase)})
	}
	if len(state.DecisionLog) > 0 {
		items := make([]string, 0, len(state.DecisionLog))
		for _, entry := range state.DecisionLog {
			line := "- " + entry.Decision
			if entry.Reason != "" {
				line += " (Why: " + entry.Reason + ")"
			}
			items = append(items, line)
		}
		lines = appendSection(lines, "Decision Log", items)
	}
	if len(state.DecisionGaps) > 0 {
		items := make([]string, 0, len(state.DecisionGaps))
		for _, gap := range state.DecisionGaps {
			items = append(items, "- "+gap)
		}
		lines = appendSection(lines, "Decision Gaps", items)
	}
	if len(state.Steps) > 0 {
		items := make([]string, 0, len(state.Steps))
		for i, step := range state.Steps {
			line := fmt.Sprintf("%d. [%s] %s", i+1, step.Status, step.Title)
			if step.Description != "" {
				line += " - " + step.Description
			}
			items = append(items, line)
		}
		lines = appendSection(lines, "Plan", items)
	}
	if len(state.Risks) > 0 {
		items := make([]string, 0, len(state.Risks))
		for _, risk := range state.Risks {
			items = append(items, "- "+risk)
		}
		lines = appendSection(lines, "Risks", items)
	}
	if len(state.Verification) > 0 {
		items := make([]string, 0, len(state.Verification))
		for _, item := range state.Verification {
			items = append(items, "- "+item)
		}
		lines = appendSection(lines, "Verification", items)
	}
	readiness := []string{
		"- Scope defined: " + yesNo(state.ScopeDefined),
		"- Risks and rollback defined: " + yesNo(state.RiskRollbackDefined),
		"- Verification path defined: " + yesNo(state.VerificationDefined),
	}
	lines = appendSection(lines, "Execution Readiness", readiness)
	if next := strings.TrimSpace(state.NextAction); next != "" {
		lines = appendSection(lines, "Next Action", []string{"- " + next})
	}
	if reason := strings.TrimSpace(state.BlockReason); reason != "" {
		lines = appendSection(lines, "Blocked Reason", []string{"- " + reason})
	}
	lines = append(lines, "</proposed_plan>")
	return strings.Join(lines, "\n")
}

func RenderPromptStateBlock(state State) string {
	state = NormalizeState(state)
	if !hasRenderablePlanState(state) {
		return ""
	}

	lines := []string{}
	if phase := NormalizePhase(string(state.Phase)); phase != PhaseNone {
		lines = append(lines, "phase: "+string(phase))
	}
	if goal := strings.TrimSpace(state.Goal); goal != "" {
		lines = append(lines, "goal: "+goal)
	}
	if summary := strings.TrimSpace(state.Summary); summary != "" {
		lines = append(lines, "summary: "+summary)
	}
	if len(state.DecisionGaps) > 0 {
		lines = append(lines, "decision_gaps: "+strings.Join(state.DecisionGaps, " | "))
	}
	if len(state.DecisionLog) > 0 {
		lines = append(lines, "decision_log:")
		for _, entry := range state.DecisionLog {
			line := "- " + entry.Decision
			if entry.Reason != "" {
				line += " -- " + entry.Reason
			}
			lines = append(lines, line)
		}
	}
	if len(state.Steps) > 0 {
		lines = append(lines, "steps:")
		for _, step := range state.Steps {
			lines = append(lines, fmt.Sprintf("- [%s] %s", step.Status, step.Title))
		}
	}
	if len(state.Risks) > 0 {
		lines = append(lines, "risks: "+strings.Join(state.Risks, " | "))
	}
	if len(state.Verification) > 0 {
		lines = append(lines, "verification: "+strings.Join(state.Verification, " | "))
	}
	if state.ScopeDefined || state.RiskRollbackDefined || state.VerificationDefined {
		lines = append(lines, fmt.Sprintf(
			"execution_readiness: scope=%s, risks_rollback=%s, verification=%s",
			yesNo(state.ScopeDefined),
			yesNo(state.RiskRollbackDefined),
			yesNo(state.VerificationDefined),
		))
	}
	if next := strings.TrimSpace(state.NextAction); next != "" {
		lines = append(lines, "next_action: "+next)
	}
	if reason := strings.TrimSpace(state.BlockReason); reason != "" {
		lines = append(lines, "block_reason: "+reason)
	}
	return strings.Join(lines, "\n")
}

func ShouldRenderStructuredPlanBlock(state State) bool {
	state = NormalizeState(state)
	if !hasRenderablePlanState(state) || !HasStructuredPlan(state) || HasDecisionGaps(state) {
		return false
	}
	switch NormalizePhase(string(state.Phase)) {
	case PhaseExplore, PhaseClarify:
		return false
	default:
		return true
	}
}

func hasRenderablePlanState(state State) bool {
	return HasStructuredPlan(state) ||
		strings.TrimSpace(state.Goal) != "" ||
		strings.TrimSpace(state.Summary) != "" ||
		strings.TrimSpace(state.ImplementationBrief) != "" ||
		len(state.DecisionLog) > 0 ||
		len(state.DecisionGaps) > 0 ||
		len(state.Risks) > 0 ||
		len(state.Verification) > 0
}

func appendSection(lines []string, title string, items []string) []string {
	if len(items) == 0 {
		return lines
	}
	lines = append(lines, title)
	lines = append(lines, items...)
	return lines
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func splitDocumentLines(text string) []string {
	rawLines := strings.Split(strings.TrimSpace(text), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
