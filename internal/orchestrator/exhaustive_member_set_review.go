package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runDeterministicExhaustiveMemberSetReview replaces the
// self-consistency + semantic-quality LLM reviewers for large
// model-authored exhaustive member sets. It runs in O(N) over
// typed labels + citation indices and never dispatches an LLM.
//
// Activation is gated on a typed signature: a model-emitted
// aggregate_facts entry with kind=member_set AND value==len(members)
// AND len(members) >= threshold. Below the threshold, the existing
// LLM reviewers stay in charge. Returns (violations, true) when the
// deterministic path replaced the LLM reviewers; (nil, false) when
// the typed signature was absent and the caller should fall through
// to the legacy reviewer dispatch.
func (o *Orchestrator) runDeterministicExhaustiveMemberSetReview(
	doc *types.AnswerDocumentV2,
	mut *types.MutableState,
) ([]types.Violation, bool) {
	if o == nil || doc == nil || mut == nil {
		return nil, false
	}
	threshold := o.exhaustiveDeterministicReviewThresholdValue()
	if threshold <= 0 {
		return nil, false
	}
	facts := mut.StableInvestigationAggregateFacts()
	fact, ok := types.LargestExhaustiveMemberSet(facts, threshold)
	if !ok {
		return nil, false
	}
	cov := types.ComputeExhaustiveMemberCoverage(doc, fact)
	logging.Info(
		"[deterministic-exhaustive-review] members=%d principal_items=%d missing=%d unexpected=%d dup_cit=%d invalid_cit=%d",
		cov.MemberCount, cov.PrincipalItemCount,
		len(cov.MissingMembers), len(cov.UnexpectedItems),
		len(cov.DuplicateCitations), len(cov.InvalidCitationRefs),
	)
	if !cov.HasFailures() {
		return nil, true
	}
	return exhaustiveMemberSetViolations(fact, cov), true
}

func exhaustiveMemberSetViolations(fact types.AnswerAggregateFact, cov types.ExhaustiveMemberCoverage) []types.Violation {
	var out []types.Violation
	label := strings.TrimSpace(fact.Label)
	if label == "" {
		label = "exhaustive member set"
	}

	if len(cov.MissingMembers) > 0 {
		out = append(out, types.Violation{
			Kind: types.ViolExhaustiveMemberSetCoverageDrift,
			Detail: fmt.Sprintf(
				"%s declares %d members, but %d are missing from the principal list/table: %s",
				label, cov.MemberCount, len(cov.MissingMembers),
				types.FormatExhaustiveMissingMembersHint(cov.MissingMembers, 6),
			),
			Repair: fmt.Sprintf(
				"re-emit the principal list/table so every model-authored member appears as exactly one item label. Cite the typed support_ref / location for each missing member; do not invent labels not present in the structured aggregate fact.",
			),
			Stage: string(types.StageFinalize),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "aggregate_facts.member_set.members",
				Reason:     "exhaustive principal slate omitted members declared in the typed aggregate fact",
				Confidence: 0.92,
			},
			ClusterKey: types.IdentityClusterKey("exhaustive_member_set_missing:"+label, "coverage"),
		})
	}
	if len(cov.UnexpectedItems) > 0 {
		out = append(out, types.Violation{
			Kind: types.ViolExhaustiveMemberSetCoverageDrift,
			Detail: fmt.Sprintf(
				"%s declares %d members, but the principal list/table includes %d label(s) not in the typed set: %s",
				label, cov.MemberCount, len(cov.UnexpectedItems),
				types.FormatExhaustiveMissingMembersHint(cov.UnexpectedItems, 6),
			),
			Repair: fmt.Sprintf(
				"remove principal items whose labels are not in the model-authored member set, or extend the aggregate fact's members and update its value before re-emitting.",
			),
			Stage: string(types.StageFinalize),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "aggregate_facts.member_set.members",
				Reason:     "principal slate includes items outside the typed exhaustive member set",
				Confidence: 0.92,
			},
			ClusterKey: types.IdentityClusterKey("exhaustive_member_set_unexpected:"+label, "coverage"),
		})
	}
	if len(cov.DuplicateCitations) > 0 {
		refs := formatCitationRefs(cov.DuplicateCitations, 6)
		out = append(out, types.Violation{
			Kind: types.ViolExhaustiveMemberSetCoverageDrift,
			Detail: fmt.Sprintf(
				"%s principal items reuse citation_ref values %s; each member needs its own typed citation",
				label, refs,
			),
			Repair: "split the shared citations into one citation per principal member (or extend citations[] with member-specific entries) so each item.citation_ref is unique.",
			Stage:  string(types.StageFinalize),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "blocks[].items[].citation_ref",
				Reason:     "exhaustive principal items share citation_ref indices",
				Confidence: 0.9,
			},
			ClusterKey: types.IdentityClusterKey("exhaustive_member_set_dup_cit:"+label, "coverage"),
		})
	}
	if len(cov.InvalidCitationRefs) > 0 {
		refs := formatCitationRefs(cov.InvalidCitationRefs, 6)
		out = append(out, types.Violation{
			Kind: types.ViolExhaustiveMemberSetCoverageDrift,
			Detail: fmt.Sprintf(
				"%s principal items reference citation_ref values %s that are outside the emitted citations[] pool",
				label, refs,
			),
			Repair: "set each principal item's citation_ref to a valid index within citations[], or extend citations[] before raising the index.",
			Stage:  string(types.StageFinalize),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "blocks[].items[].citation_ref",
				Reason:     "exhaustive principal items reference citation indices outside the emitted pool",
				Confidence: 0.95,
			},
			ClusterKey: types.IdentityClusterKey("exhaustive_member_set_invalid_cit:"+label, "coverage"),
		})
	}
	return out
}

func formatCitationRefs(refs []int, maxShown int) string {
	if len(refs) == 0 {
		return ""
	}
	if maxShown <= 0 {
		maxShown = 6
	}
	shown := refs
	suffix := ""
	if len(refs) > maxShown {
		shown = refs[:maxShown]
		suffix = fmt.Sprintf(" (+%d more)", len(refs)-maxShown)
	}
	parts := make([]string, 0, len(shown))
	for _, r := range shown {
		parts = append(parts, fmt.Sprintf("%d", r))
	}
	return "[" + strings.Join(parts, ", ") + "]" + suffix
}
