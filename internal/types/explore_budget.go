package types

// ExploreBudget is the runtime counter + ceiling the explorer
// consults before every tool dispatch. The orchestrator installs it
// on MutableState at the start of runTaskGraph (derived from
// AnalysisIR.EvidencePlan.NodeBudgetHints), and the explorer reads
// it through Mutable.ExploreBudget() on every tool call.
//
// PerToolCap / PerToolUsed are keyed by canonical tool name (e.g.
// "grep", "read_file", "repo_map") — see sourcemix.canonicalTool.
// OverallCap is the ceiling for total tool calls across every tool;
// 0 disables the ceiling so the per-tool caps alone govern.
type ExploreBudget struct {
	PerToolCap  map[string]int
	PerToolUsed map[string]int
	OverallCap  int
	OverallUsed int
}

// Clone returns a deep copy of the budget. Used by the getter on
// MutableState so the caller cannot race against concurrent updates
// through the shared map pointer.
func (b *ExploreBudget) Clone() *ExploreBudget {
	if b == nil {
		return nil
	}
	out := &ExploreBudget{
		OverallCap:  b.OverallCap,
		OverallUsed: b.OverallUsed,
	}
	if b.PerToolCap != nil {
		out.PerToolCap = make(map[string]int, len(b.PerToolCap))
		for k, v := range b.PerToolCap {
			out.PerToolCap[k] = v
		}
	}
	if b.PerToolUsed != nil {
		out.PerToolUsed = make(map[string]int, len(b.PerToolUsed))
		for k, v := range b.PerToolUsed {
			out.PerToolUsed[k] = v
		}
	}
	return out
}
