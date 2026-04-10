package layout

func CandidateOriginsNear(expectedTop, maxOrigin, window int) []int {
	if maxOrigin < 0 {
		return nil
	}
	expectedTop = Clamp(expectedTop, 0, maxOrigin)
	out := make([]int, 0, window*2+1)
	seen := make(map[int]struct{}, window*2+1)
	add := func(candidate int) {
		if candidate < 0 || candidate > maxOrigin {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	add(expectedTop)
	for delta := 1; delta <= window; delta++ {
		add(expectedTop - delta)
		add(expectedTop + delta)
	}
	return out
}

func RoundedScaledDivision(value, scale, denominator int) int {
	if denominator <= 0 || value <= 0 || scale <= 0 {
		return 0
	}
	numerator := int64(value)*int64(scale) + int64(denominator)/2
	result := numerator / int64(denominator)
	maxInt := int64(^uint(0) >> 1)
	if result > maxInt {
		return int(maxInt)
	}
	return int(result)
}

func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
