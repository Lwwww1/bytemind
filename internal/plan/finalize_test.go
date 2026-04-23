package plan

import (
	"strings"
	"testing"
)

func TestFinalizeAssistantAnswerBuildModeUnchanged(t *testing.T) {
	answer := "normal build answer"
	got := FinalizeAssistantAnswer(ModeBuild, State{}, answer)
	if got != answer {
		t.Fatalf("expected unchanged answer, got %q", got)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithOpenDecisionGapKeepsQuestionOnly(t *testing.T) {
	answer := "plan answer"
	got := FinalizeAssistantAnswer(ModePlan, State{
		Goal:         "Ship plan loop",
		Phase:        PhaseClarify,
		Steps:        []Step{{Title: "step1", Status: StepPending}},
		DecisionGaps: []string{"Choose the execution trigger wording"},
	}, answer)
	if got != answer {
		t.Fatalf("expected clarify answer to stay question-only, got %q", got)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithoutDecisionGapAppendsCanonicalBlock(t *testing.T) {
	answer := "plan answer"
	got := FinalizeAssistantAnswer(ModePlan, State{
		Goal:                "Ship plan loop",
		ImplementationBrief: "Objective: ship the plan loop.\nDeliverable: prompt + finalize behavior.",
		Phase:               PhaseConvergeReady,
		Steps:               []Step{{Title: "step1", Status: StepPending}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}, answer)
	for _, want := range []string{
		"plan answer",
		"<proposed_plan>",
		"Implementation Brief",
		"Objective: ship the plan loop.",
		"Goal",
		"1. [pending] step1",
		"</proposed_plan>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected finalized answer to include %q, got %q", want, got)
		}
	}
}

func TestFinalizeAssistantAnswerPlanModeWithoutStructuredPlanAppendsReminder(t *testing.T) {
	answer := "drafted plan"
	got := FinalizeAssistantAnswer(ModePlan, State{}, answer)
	want := answer + "\n\n" + StructuredPlanReminder
	if got != want {
		t.Fatalf("unexpected finalized answer: got=%q want=%q", got, want)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithoutStructuredPlanHandlesEmptyAnswer(t *testing.T) {
	got := FinalizeAssistantAnswer(ModePlan, State{}, "   ")
	if got != StructuredPlanReminder {
		t.Fatalf("expected reminder-only answer, got %q", got)
	}
}
