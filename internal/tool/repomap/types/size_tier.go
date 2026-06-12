package types

// SizeTier buckets a graph by indexed file count so view defaults can
// scale with repository size: tiny repos drown in 20-row listings,
// huge repos need narrowing guidance instead of wider output. Cut
// points follow the empirically validated CodeGraph explore-budget
// tiers (150/500/5000/15000).
type SizeTier int

const (
	SizeTierTiny SizeTier = iota
	SizeTierSmall
	SizeTierMedium
	SizeTierLarge
	SizeTierVeryLarge
)

// RepoSizeTier classifies an indexed file count. The input is the
// served graph's len(FileIndex) — the scope this call can actually
// enumerate — never an aggregate across sub-repos.
func RepoSizeTier(fileCount int) SizeTier {
	switch {
	case fileCount < 150:
		return SizeTierTiny
	case fileCount < 500:
		return SizeTierSmall
	case fileCount < 5000:
		return SizeTierMedium
	case fileCount < 15000:
		return SizeTierLarge
	default:
		return SizeTierVeryLarge
	}
}

// GraphSizeTier reads the size tier off a graph, tolerating
// test-constructed graphs whose FileIndex was never populated.
func GraphSizeTier(g *Graph) SizeTier {
	if g == nil {
		return SizeTierMedium
	}
	n := len(g.FileIndex)
	if n == 0 {
		n = len(g.Files)
	}
	return RepoSizeTier(n)
}

// DefaultTopN is the per-view × per-tier default row budget, applied
// only when the caller did not pass an explicit top_n (explicit user
// values always win). Only tiny/small tiers shrink below the
// historical per-view defaults; medium and above keep them — the
// uniform-small-budget alternative was rejected because relation_map's
// single topN feeds three sections (sources, rows, verification
// files) and source_inventory's row budget carries count-checklist
// semantics. Views absent from the table (overview, file_map,
// semantic_subgraph, call_path, edit_impact) are deliberately
// untiered: overview feeds the analyzer prompt pre-injection with
// zero params, and the others are exhaustive or already small.
//
// Invariant (pinned by test): a larger tier never gets a smaller
// default than a smaller tier.
func DefaultTopN(view string, tier SizeTier) int {
	table, ok := viewTierDefaults[view]
	if !ok {
		return 0 // untiered view: caller keeps its historical default
	}
	if tier < SizeTierTiny || tier > SizeTierVeryLarge {
		tier = SizeTierMedium
	}
	return table[tier]
}

var viewTierDefaults = map[string][5]int{
	// tiny, small, medium, large, very_large
	"task_map":         {6, 10, 20, 20, 20},
	"relation_map":     {12, 20, 40, 40, 40},
	"source_inventory": {24, 40, 60, 60, 60},
}
