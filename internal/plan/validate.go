package plan

import "strings"

type ValidationResult struct {
	OK             bool
	Warnings       []string
	RequiresReplan bool
}

func CanTransition(from, to Phase) bool {
	switch from {
	case PhaseNone:
		return to == PhaseDrafting
	case PhaseDrafting:
		return to == PhaseReady
	case PhaseReady:
		return to == PhaseDrafting || to == PhaseApproved
	case PhaseApproved:
		return to == PhaseExecuting
	case PhaseExecuting:
		return to == PhaseBlocked || to == PhaseCompleted
	case PhaseBlocked:
		return to == PhaseDrafting || to == PhaseExecuting
	default:
		return false
	}
}

func ValidateState(state State) ValidationResult {
	result := ValidationResult{OK: true}
	inProgressCount := 0
	blockedCount := 0
	for _, step := range state.Steps {
		switch NormalizeStepStatus(string(step.Status)) {
		case StepInProgress:
			inProgressCount++
		case StepBlocked:
			blockedCount++
		}
	}
	if inProgressCount > 1 {
		result.OK = false
		result.RequiresReplan = true
		result.Warnings = append(result.Warnings, "only one step can be in_progress")
	}
	if blockedCount > 0 && strings.TrimSpace(state.BlockReason) == "" {
		result.OK = false
		result.RequiresReplan = true
		result.Warnings = append(result.Warnings, "blocked plans must include a block reason")
	}
	return result
}
