package types

import "sort"

// PresentationRowGroup describes already accepted rows for display only.
// ParentKey identifies a source/query scope; Key identifies a table or family
// within it. Empty parent keys remain independent, never an inferred join.
// Smaller Priority values prefer residual capacity after source/group coverage.
type PresentationRowGroup struct {
	Key       string
	ParentKey string
	Rows      int
	Priority  int
}

// AllocatePresentationRows returns counts aligned with groups. It first gives
// each parent a turn, then each of that parent's groups a turn, so neither one
// large source nor its many tables can hide another source. Remaining capacity
// completes small groups by priority, then is fairly shared by unfinished rows.
// Whole rows are the indivisible unit. These counts confer no evidence,
// completeness, window, or causal authority and never change the retained rows.
func AllocatePresentationRows(groups []PresentationRowGroup, budget int) []int {
	counts := make([]int, len(groups))
	if budget <= 0 {
		return counts
	}
	var order []int
	for i, group := range groups {
		if group.Rows > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return groups[order[i]].Priority < groups[order[j]].Priority })
	var parents [][]int
	parentIndex := map[string]int{}
	for _, i := range order {
		key := groups[i].ParentKey
		index, exists := parentIndex[key]
		if key == "" || !exists {
			index = len(parents)
			parents = append(parents, nil)
			if key != "" {
				parentIndex[key] = index
			}
		}
		parents[index] = append(parents[index], i)
	}
	for round := 0; budget > 0; round++ {
		progress := false
		for _, parent := range parents {
			if round < len(parent) {
				counts[parent[round]]++
				budget--
				progress = true
				if budget == 0 {
					return counts
				}
			}
		}
		if !progress {
			break
		}
	}
	// Prefer complete small groups rather than leaving unused capacity behind
	// independent per-table caps. Stable ties preserve the producer's order.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if groups[a].Priority != groups[b].Priority {
			return groups[a].Priority < groups[b].Priority
		}
		return groups[a].Rows-counts[a] < groups[b].Rows-counts[b]
	})
	for _, i := range order {
		remaining := groups[i].Rows - counts[i]
		if remaining <= budget {
			counts[i] += remaining
			budget -= remaining
		}
	}
	// Bulk water filling keeps cost bounded by the number of groups, not by
	// a potentially large row count or budget.
	for budget > 0 {
		var active []int
		for _, i := range order {
			if counts[i] < groups[i].Rows {
				active = append(active, i)
			}
		}
		if len(active) == 0 {
			break
		}
		share := budget / len(active)
		if share == 0 {
			share = 1
		}
		for _, i := range active {
			n := min(share, groups[i].Rows-counts[i], budget)
			counts[i] += n
			budget -= n
			if budget == 0 {
				break
			}
		}
	}
	return counts
}
