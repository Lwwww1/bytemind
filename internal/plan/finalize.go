package plan

import (
	"regexp"
	"strings"
)

const StructuredPlanReminder = "Plan mode requires a structured plan before finishing. Please restate the plan using update_plan."

var proposedPlanBlockPattern = regexp.MustCompile(`(?is)<proposed_plan>.*?</proposed_plan>`)

// FinalizeAssistantAnswer enforces plan-mode completion policy on final text.
func FinalizeAssistantAnswer(mode AgentMode, state State, answer string) string {
	if mode != ModePlan {
		return answer
	}
	clean := strings.TrimSpace(answer)
	if !HasStructuredPlan(state) {
		if clean == "" {
			return StructuredPlanReminder
		}
		return clean + "\n\n" + StructuredPlanReminder
	}
	clean = strings.TrimSpace(proposedPlanBlockPattern.ReplaceAllString(clean, ""))
	block := RenderStructuredPlanBlock(state)
	if block == "" {
		return clean
	}
	if clean == "" {
		return block
	}
	return clean + "\n\n" + block
}
