package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type sourceInventoryCandidateFamilyKey struct {
	role     types.AnswerCandidateRole
	class    types.SourcePathRole
	language string
}

type sourceInventoryCandidateSelection struct {
	candidates []sourceInventoryCandidate
	total      int
	partial    bool
	truncated  bool
}

func sourceInventorySelectCandidatesForBudget(candidates []sourceInventoryCandidate, budget sourceInventoryExecBudget) sourceInventoryCandidateSelection {
	total := len(candidates)
	if total == 0 {
		return sourceInventoryCandidateSelection{}
	}
	ordered := sourceInventoryCoverageOrderCandidates(candidates)
	offset := budget.kernel.CursorOffset()
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		return sourceInventoryCandidateSelection{total: total, partial: true}
	}
	limit := budget.kernel.PageLimit()
	if limit <= 0 || limit > total {
		limit = total
	}
	if maxPerRole := budget.kernel.MaxPerRole(); maxPerRole > 0 && limit > maxPerRole {
		limit = maxPerRole
	}
	end := offset + limit
	if end > total {
		end = total
	}
	selected := append([]sourceInventoryCandidate(nil), ordered[offset:end]...)
	partial := offset > 0 || end < total
	return sourceInventoryCandidateSelection{
		candidates: selected,
		total:      total,
		partial:    partial,
		truncated:  end < total,
	}
}

func sourceInventoryCoverageOrderCandidates(candidates []sourceInventoryCandidate) []sourceInventoryCandidate {
	if len(candidates) <= 1 {
		return append([]sourceInventoryCandidate(nil), candidates...)
	}
	ordered := append([]sourceInventoryCandidate(nil), candidates...)
	sourceInventorySortCandidates(ordered)
	groups := map[sourceInventoryCandidateFamilyKey][]sourceInventoryCandidate{}
	groupOrder := make([]sourceInventoryCandidateFamilyKey, 0, len(ordered))
	seenGroup := map[sourceInventoryCandidateFamilyKey]bool{}
	for _, candidate := range ordered {
		key := sourceInventoryCandidateFamily(candidate)
		if !seenGroup[key] {
			seenGroup[key] = true
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], candidate)
	}
	out := make([]sourceInventoryCandidate, 0, len(ordered))
	emitted := map[string]bool{}
	for {
		added := false
		for _, key := range groupOrder {
			group := groups[key]
			if len(group) == 0 {
				continue
			}
			candidate := group[0]
			groups[key] = group[1:]
			dedupeKey := sourceInventoryCandidateDedupeKey(candidate)
			if dedupeKey != "" && emitted[dedupeKey] {
				continue
			}
			emitted[dedupeKey] = true
			out = append(out, candidate)
			added = true
		}
		if !added {
			break
		}
	}
	return out
}

func sourceInventoryCandidateFamily(candidate sourceInventoryCandidate) sourceInventoryCandidateFamilyKey {
	class := types.ClassifySourcePathRole(candidate.file)
	if class == types.SourcePathRoleUnknown {
		class = types.SourcePathRoleProduction
	}
	language := strings.ToLower(strings.TrimSpace(candidate.language))
	if language == "" {
		language = "unknown"
	}
	return sourceInventoryCandidateFamilyKey{
		role:     candidate.role,
		class:    class,
		language: language,
	}
}
