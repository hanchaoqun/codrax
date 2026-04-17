package types

import "strings"

// CanonicalToolName maps common tool-name aliases to the canonical
// form used as the key in ExploreBudget.PerToolCap / PerToolUsed.
// Keeps agent.canonicalToolName and sourcemix.canonicalTool in sync
// — both used to carry byte-identical local copies and drift risk
// was only caught when the budget lookup silently missed an alias.
//
// Accepted aliases:
//   read    → read_file
//   repomap → repo_map
//   ls      → list_files
//
// Anything else passes through lowercased-and-trimmed.
func CanonicalToolName(name string) string {
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

// ExploreBudget is the runtime counter + ceiling the explorer
// consults before every tool dispatch. The orchestrator installs it
// on MutableState at the start of runTaskGraph (derived from
// AnalysisIR.EvidencePlan.NodeBudgetHints), and the explorer reads
// it through Mutable.ExploreBudget() on every tool call.
//
// PerToolCap / PerToolUsed are keyed by canonical tool name (e.g.
// "grep", "read_file", "repo_map") — see types.CanonicalToolName.
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
