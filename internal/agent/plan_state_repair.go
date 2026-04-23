package agent

import (
	"fmt"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

func shouldRepairPlanDecisionTurn(runMode planpkg.AgentMode, state planpkg.State, intent assistantTurnIntent, reply llm.Message) bool {
	if runMode != planpkg.ModePlan || len(reply.ToolCalls) > 0 {
		return false
	}

	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) || !planpkg.HasDecisionGaps(state) {
		return false
	}

	text := strings.TrimSpace(reply.Content)
	if text == "" || looksLikeClarifyingQuestion(text) {
		return false
	}

	if intent == turnIntentFinalize {
		return true
	}
	return looksLikePlanDecisionAcknowledgement(text)
}

func buildPlanDecisionRepairInstruction(state planpkg.State, reply llm.Message, attempt, maxAttempts int) string {
	state = planpkg.NormalizeState(state)
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)

	gaps := "(none recorded)"
	if len(state.DecisionGaps) > 0 {
		items := make([]string, 0, len(state.DecisionGaps))
		for _, gap := range state.DecisionGaps {
			gap = strings.TrimSpace(gap)
			if gap != "" {
				items = append(items, "- "+gap)
			}
		}
		if len(items) > 0 {
			gaps = strings.Join(items, "\n")
		}
	}

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn appears to have accepted or acted on a user decision in plan mode without calling update_plan first.
Attempt %d/%d.

Reply text preview:
%s

Current unresolved decision gaps:
%s

For this next turn:
1) If the user's latest reply resolves one of these decisions, call update_plan first.
2) Record the chosen option in decision_log and remove or replace the resolved decision_gaps.
3) If no decision gaps remain, populate implementation_brief, set phase to draft or converge_ready as appropriate, and then finalize so the full proposed plan is shown.
4) If another decision is still required, include <turn_intent>ask_user</turn_intent> and ask only the next question.
5) Do not reply with choice-acknowledgement or start-execution text in plan mode without updating plan state first.`,
		attempt,
		maxAttempts,
		preview,
		gaps,
	))
}

func looksLikeClarifyingQuestion(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "question:") ||
		strings.Contains(normalized, "question：") ||
		strings.Contains(normalized, "问题:") ||
		strings.Contains(normalized, "问题：") {
		return true
	}
	hasOptions := strings.Contains(normalized, "a.") && strings.Contains(normalized, "b.")
	if hasOptions && (strings.Contains(normalized, "other:") || strings.Contains(normalized, "other：")) {
		return true
	}
	return false
}

func looksLikePlanDecisionAcknowledgement(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return containsAnyToken(normalized,
		"adopt",
		"adopted",
		"go with",
		"going with",
		"chosen",
		"choose ",
		"recorded",
		"using option",
		"start execution",
		"adjust plan",
		"switch to build",
		"采用",
		"已收到",
		"已记录",
		"记录为",
		"选用",
		"选择",
		"开始执行",
		"调整计划",
		"切到 build",
		"切换到 build",
	)
}
