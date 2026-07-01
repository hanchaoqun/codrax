package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// dominantViolationKind returns the most common ViolationKind in the failed
// contract result. When violations are distributed evenly, returns the first
// violation's Kind (stable ordering). An empty result returns "" so the caller
// can treat it as "no per-kind budget applicable".
//
// Used by the retry-budget gate to pick which kind's cap to consult. The
// dominance rule keeps the gate sticky: a run that keeps producing shape_swap
// events stays on the shape-violation budget even when occasional citation
// violations sneak in.
func dominantViolationKind(res contract.Result) types.ViolationKind {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	violations := FilterFinalizerRetryRootViolations(res.Violations)
	if len(violations) == 0 {
		return ""
	}
	counts := make(map[types.ViolationKind]int)
	for _, v := range violations {
		counts[v.Kind]++
	}
	var (
		bestKind  types.ViolationKind
		bestCount int
	)
	for _, v := range violations {
		if counts[v.Kind] > bestCount {
			bestCount = counts[v.Kind]
			bestKind = v.Kind
		}
	}
	return bestKind
}

func sameErrorClassRetryCap() int { return 1 }

func dominantViolationClass(res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	roots := FilterFinalizerRetryRootViolations(res.Violations)
	if len(roots) == 0 {
		return ""
	}
	counts := make(map[string]int, len(roots))
	for _, v := range roots {
		class := violationRetryClass(v)
		if class == "" {
			continue
		}
		counts[class]++
	}
	var (
		bestClass string
		bestCount int
	)
	for _, v := range roots {
		class := violationRetryClass(v)
		if class == "" {
			continue
		}
		if counts[class] > bestCount {
			bestCount = counts[class]
			bestClass = class
		}
	}
	return bestClass
}

func violationRetryClass(v types.Violation) string {
	target := FallbackTargetForViolation(v)
	if spec, ok := types.ViolKindSpecFor(v.Kind); ok {
		family := strings.TrimSpace(string(spec.CaveatFamilyID))
		if family != "" {
			return string(target) + ":" + family
		}
	}
	if v.Kind == "" {
		return ""
	}
	return string(target) + ":kind:" + string(v.Kind)
}

// applyContractViolations is the single decision point for where the contract
// checker's per-violation digest goes: user panel vs operator log.
func (o *Orchestrator) applyContractViolations(answer string, res contract.Result) string {
	digest := formatViolationsForLogger(res)
	if digest == "" {
		return answer
	}
	logging.Warning("[orchestrator] %s", digest)
	if !o.settings.ViolationBudget.UserVisibleViolationCaveat {
		return answer
	}
	return digest + "\n\n" + answer
}

// formatViolationFieldTally renders the ledger's per-field histogram as a
// compact stable string for the CGEC summary line.
func formatViolationFieldTally(tally map[string]int) string {
	if len(tally) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s:%d", k, tally[k])
	}
	b.WriteString("}")
	return b.String()
}
