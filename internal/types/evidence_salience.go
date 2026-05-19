package types

import "strings"

// EvidenceSalience is the explorer's typed claim about how an
// evidence row participates in the user-facing answer.
//
// The zero value is intentionally inert for ranking and cap policy:
// legacy evidence rows that do not carry the lane must behave exactly
// as they did before the lane existed. Resolve projects that zero
// value to supporting only for semantic/default interpretation.
type EvidenceSalience string

const (
	SalienceUnset         EvidenceSalience = ""
	SalienceLoadBearing   EvidenceSalience = "load_bearing"
	SalienceExhaustListed EvidenceSalience = "exhaust_listed"
	SalienceSupporting    EvidenceSalience = "supporting"
	SalienceContext       EvidenceSalience = "context"
)

var validEvidenceSaliences = []EvidenceSalience{
	SalienceLoadBearing,
	SalienceExhaustListed,
	SalienceSupporting,
	SalienceContext,
}

func EvidenceSalienceValues() []EvidenceSalience {
	out := make([]EvidenceSalience, len(validEvidenceSaliences))
	copy(out, validEvidenceSaliences)
	return out
}

func EvidenceSalienceStrings() []string {
	values := EvidenceSalienceValues()
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func (s EvidenceSalience) IsValid() bool {
	if s == SalienceUnset {
		return true
	}
	for _, value := range validEvidenceSaliences {
		if s == value {
			return true
		}
	}
	return false
}

// IsSet reports whether the model explicitly emitted a salience lane.
// Consumers that change ranking, cap, or dedup behavior must check
// this rather than Resolve so the empty-lane path stays no-op.
func (s EvidenceSalience) IsSet() bool {
	return s != SalienceUnset
}

// Resolve maps unset to the routine supporting tier for presentation
// and default semantic interpretation. It must not be used as proof
// that the model explicitly emitted a salience signal.
func (s EvidenceSalience) Resolve() EvidenceSalience {
	if s == SalienceUnset {
		return SalienceSupporting
	}
	return s
}

// IsLocked reports whether this explicitly emitted salience value must
// be protected from silent ranking/cap/dedup loss.
func (s EvidenceSalience) IsLocked() bool {
	switch s {
	case SalienceLoadBearing, SalienceExhaustListed:
		return true
	default:
		return false
	}
}

// Rank maps salience values into a stable merge priority. Lower means
// higher salience. Unset is deliberately lower priority than every
// explicit value, including context, so explicit model intent is
// preserved during evidence merges.
func (s EvidenceSalience) Rank() int {
	switch s {
	case SalienceLoadBearing:
		return 0
	case SalienceExhaustListed:
		return 1
	case SalienceSupporting:
		return 2
	case SalienceContext:
		return 3
	default:
		return 4
	}
}

func ParseEvidenceSalience(raw string) (EvidenceSalience, bool) {
	trimmed := EvidenceSalience(strings.ToLower(strings.TrimSpace(raw)))
	if trimmed == SalienceUnset {
		return SalienceUnset, true
	}
	if trimmed.IsValid() {
		return trimmed, true
	}
	return SalienceUnset, false
}

func MergeEvidenceSalience(dst, src EvidenceSalience) EvidenceSalience {
	if !src.IsSet() {
		return dst
	}
	if !dst.IsSet() || src.Rank() < dst.Rank() {
		return src
	}
	return dst
}
