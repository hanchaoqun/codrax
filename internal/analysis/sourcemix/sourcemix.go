// Package sourcemix compiles the template-level SourceMix ratios
// into NodeBudgetHints and checks per-tool budgets at dispatch time.
//
// The analyzer writes SourceMix as a human-readable declaration
// (e.g. grep:30, repomap:40, read:30). FromTemplateMix converts
// those ratios into concrete per-tool caps sized against the
// EvidenceBudget, producing a NodeBudgetHints that the explorer's
// ReAct loop reads on every tool dispatch.
//
// The explorer itself calls BudgetForTool to ask "can I run this
// tool in the current window?" and blocks the call when the cap has
// been reached. The cap is a hard wall, not a soft hint: once hit,
// the LLM is told it has exhausted that source and must use the
// remaining budget on a different tool.
package sourcemix

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// FromTemplateMix converts a raw ratio map (e.g. {"grep":30,
// "repomap":40, "read":30}) into NodeBudgetHints sized against the
// total tool-call budget. Zero or negative totals collapse to an
// empty hint.
//
// Tool-name canonicalization: both "read" and "read_file" are
// normalized to "read_file"; both "repomap" and "repo_map" to
// "repo_map". This matches the tool names the explorer actually
// dispatches, so BudgetForTool matches per-call without aliasing.
func FromTemplateMix(mix map[string]int, budget types.EvidenceBudget) types.NodeBudgetHints {
	overall := budget.MaxToolCalls
	if overall <= 0 || len(mix) == 0 {
		return types.NodeBudgetHints{OverallCap: overall}
	}
	total := 0
	for _, v := range mix {
		if v > 0 {
			total += v
		}
	}
	if total <= 0 {
		return types.NodeBudgetHints{OverallCap: overall}
	}
	per := make(map[string]int, len(mix))
	spent := 0
	for name, ratio := range mix {
		if ratio <= 0 {
			continue
		}
		canonical := canonicalTool(name)
		cap := (overall * ratio) / total
		if cap < 1 {
			cap = 1
		}
		per[canonical] = cap
		spent += cap
	}
	// Distribute the rounding remainder to the largest cap so
	// PerTool sums to OverallCap within ±1.
	if spent < overall {
		biggest := ""
		max := 0
		for k, v := range per {
			if v > max {
				biggest = k
				max = v
			}
		}
		if biggest != "" {
			per[biggest] += overall - spent
		}
	}
	return types.NodeBudgetHints{PerToolCap: per, OverallCap: overall}
}

// BudgetForTool reports whether `tool` can be dispatched under the
// current ExploreBudget. allowed=false means the per-tool cap or
// the overall cap has been reached; the explorer must refuse the
// call and inform the LLM. remaining is the minimum of the two
// relevant caps (per-tool, overall).
func BudgetForTool(tool string, used types.ExploreBudget) (allowed bool, remaining int) {
	name := canonicalTool(tool)
	overallCap := used.OverallCap
	overallRem := overallCap - used.OverallUsed
	if overallCap <= 0 {
		overallRem = 1 << 30 // effectively infinite
	}
	perCap, hasPer := used.PerToolCap[name]
	perUsed := used.PerToolUsed[name]
	perRem := 1 << 30
	if hasPer {
		perRem = perCap - perUsed
	}
	min := overallRem
	if perRem < min {
		min = perRem
	}
	if min <= 0 {
		return false, 0
	}
	return true, min
}

func canonicalTool(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "read":
		return "read_file"
	case "repomap":
		return "repo_map"
	case "ls":
		return "list_files"
	}
	return n
}
