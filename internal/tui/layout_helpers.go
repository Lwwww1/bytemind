package tui

import tuilayout "bytemind/internal/tui/layout"

func candidateOriginsNear(expectedTop, maxOrigin, window int) []int {
	return tuilayout.CandidateOriginsNear(expectedTop, maxOrigin, window)
}

func roundedScaledDivision(value, scale, denominator int) int {
	return tuilayout.RoundedScaledDivision(value, scale, denominator)
}
