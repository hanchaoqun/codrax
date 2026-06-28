package agent

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	structuredAggregateDefaultPromptFacts = 16
	structuredAggregateMaxPromptFacts     = 48
)

func renderStructuredAggregateFactsForContext(ctx *types.AgentContext, facts []types.AnswerAggregateFact) string {
	facts = types.PruneAggregateMemberSetsByStructuredExclusions(facts)
	if ctx != nil && ctx.AnalysisIR != nil {
		facts = types.NormalizeAggregateFactRolesForRequest(facts, &ctx.AnalysisIR.RequestModel)
		facts = types.DemoteAggregateCountFactsConflictingWithPrincipalMemberSets(facts, &ctx.AnalysisIR.RequestModel)
	}
	return renderStructuredAggregateFactsWithOptions(facts, structuredAggregatePromptFactLimit(ctx, facts), structuredAggregatePrincipalMemberSetRefs(ctx, facts), aggregateFactRenderOptions{
		omitExcludedCandidates: aggregateFactPromptOmitExcludedCandidates(ctx),
		compactMemberSetRows:   structuredAggregateCompactPrincipalMemberSetIndexes(ctx, facts),
		compactShadowedRows:    structuredAggregateCompactShadowedMemberSetIndexes(ctx, facts),
		requestModel:           aggregateFactRenderRequestModel(ctx),
	})
}

func structuredAggregatePromptFactLimit(ctx *types.AgentContext, facts []types.AnswerAggregateFact) int {
	if len(facts) == 0 {
		return 0
	}
	limit := structuredAggregateDefaultPromptFacts
	principalCount := len(structuredAggregatePrincipalFactIndexes(ctx, facts))
	if principalCount > 0 {
		limit += aggregateFactMinInt(principalCount*2, 16)
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		rm := ctx.AnalysisIR.RequestModel
		if len(rm.SubTopics) >= 2 {
			limit += aggregateFactMinInt(len(rm.SubTopics)*2, 8)
		}
		if buckets := rm.QuestionStructure().Buckets; len(buckets) >= 2 {
			limit += aggregateFactMinInt(len(buckets)*2, 8)
		}
		if rm.Complexity == types.ComplexityComplex {
			limit += 4
		}
		if rm.Predicates.IsCrossComponent || rm.Predicates.IsCategoryEnumeration || rm.Predicates.IsRelationalLookup {
			limit += 4
		}
	}
	if limit > structuredAggregateMaxPromptFacts {
		limit = structuredAggregateMaxPromptFacts
	}
	if limit < principalCount {
		// The max cap is for auxiliary aggregate context. Principal aggregate
		// facts are answer payloads: they may be file:line member sets, tool-
		// sourced counts, negative searches, or other structured scalars. Once
		// exploration emitted them structurally, prompt-budget projection must
		// not hide them from the finalizer.
		limit = principalCount
	}
	if limit > len(facts) {
		limit = len(facts)
	}
	return limit
}

func renderStructuredAggregateFacts(facts []types.AnswerAggregateFact, maxFacts int) string {
	return renderStructuredAggregateFactsWithPrincipalRefs(facts, maxFacts, types.PrincipalAggregateMemberSetFactRefs(facts))
}

func renderStructuredAggregateFactsWithPrincipalRefs(facts []types.AnswerAggregateFact, maxFacts int, refs []types.AnswerAggregateFactRef) string {
	return renderStructuredAggregateFactsWithOptions(facts, maxFacts, refs, aggregateFactRenderOptions{})
}

type aggregateFactRenderOptions struct {
	omitExcludedCandidates bool
	compactMemberSetRows   map[int]bool
	compactShadowedRows    map[int]bool
	requestModel           *types.RequestModel
}

func renderStructuredAggregateFactsWithOptions(facts []types.AnswerAggregateFact, maxFacts int, refs []types.AnswerAggregateFactRef, opts aggregateFactRenderOptions) string {
	if len(facts) == 0 {
		return ""
	}
	principalFacts := map[int]bool{}
	roleByIndex := map[int]types.AnswerAggregateRole{}
	provenanceByIndex := map[int]string{}
	for _, ref := range refs {
		principalFacts[ref.Index] = true
		if ref.Role != "" {
			roleByIndex[ref.Index] = ref.Role
		}
		if ref.Provenance != "" {
			provenanceByIndex[ref.Index] = ref.Provenance
		}
	}
	for i, fact := range facts {
		if types.NormalizeAnswerAggregateRole(fact.Role).IsPrincipal() {
			principalFacts[i] = true
		}
	}
	order := orderedAggregateFactIndexes(facts, principalFacts)
	if maxFacts <= 0 || maxFacts > len(order) {
		maxFacts = len(order)
	}
	var b strings.Builder
	for displayIdx := 0; displayIdx < maxFacts; displayIdx++ {
		i := order[displayIdx]
		fact := facts[i]
		compactMembers := opts.compactMemberSetRows[i] &&
			fact.Kind == types.AnswerAggregateMemberSet &&
			types.AnswerAggregateFactCarriesCompleteMemberSet(fact) &&
			len(fact.Members) > 0
		compactShadowed := opts.compactShadowedRows[i] &&
			fact.Kind == types.AnswerAggregateMemberSet &&
			types.AnswerAggregateFactCarriesCompleteMemberSet(fact) &&
			len(fact.Members) > 0
		fmt.Fprintf(&b, "- kind=`%s`, label=%s", fact.Kind, fact.Label)
		if compactShadowed {
			fmt.Fprintf(&b, ", value_omitted=shadowed_by_authoritative_principal_rows")
		} else {
			fmt.Fprintf(&b, ", value=`%s`", fact.Value)
		}
		if role := aggregatePromptRoleForFact(i, fact, principalFacts, roleByIndex); role != "" {
			fmt.Fprintf(&b, ", role=`%s`", role)
		}
		if provenance := aggregatePromptProvenanceForFact(i, fact, provenanceByIndex); provenance != "" {
			fmt.Fprintf(&b, ", provenance=%s", provenance)
		}
		if origins := renderAggregateEvidenceOrigins(types.AnswerAggregateFactEvidenceOrigins(fact, opts.requestModel)); origins != "" {
			fmt.Fprintf(&b, ", evidence_origin=[%s]", origins)
		}
		if fact.Unit != "" {
			fmt.Fprintf(&b, ", unit=%s", fact.Unit)
		}
		if dims := renderAggregateDimensions(fact.Dimensions); dims != "" {
			fmt.Fprintf(&b, ", dimensions=[%s]", dims)
		}
		if compactMembers {
			fmt.Fprintf(&b, ", member_count=%d", len(fact.Members))
			fmt.Fprintf(&b, ", members_rendered_in=authoritative_principal_member_rows")
		} else if compactShadowed {
			fmt.Fprintf(&b, ", shadowed_aggregate=metadata_only")
			fmt.Fprintf(&b, ", members_omitted=shadowed_by_authoritative_principal_rows")
		} else {
			memberLimit := aggregateFactPromptMemberLimit(fact)
			if members := renderAggregateStringList(fact.Members, memberLimit); members != "" {
				fmt.Fprintf(&b, ", members=[%s]", members)
			}
		}
		if len(fact.Excluded) > 0 {
			if opts.omitExcludedCandidates {
				fmt.Fprintf(&b, ", excluded_count=%d, excluded_candidates=omitted_by_typed_exclusion_policy", len(fact.Excluded))
			} else if excluded := renderAggregateStringList(fact.Excluded, 8); excluded != "" {
				fmt.Fprintf(&b, ", excluded=[%s]", excluded)
			}
		}
		if compactMembers {
			if len(fact.SupportRefs) > 0 {
				fmt.Fprintf(&b, ", support_ref_count=%d", len(fact.SupportRefs))
			}
		} else if compactShadowed {
			if len(fact.SupportRefs) > 0 {
				fmt.Fprintf(&b, ", support_refs_omitted=shadowed_by_authoritative_principal_rows")
			}
		} else {
			refLimit := aggregateFactPromptSupportRefLimit(fact)
			if refs := renderAggregateStringList(fact.SupportRefs, refLimit); refs != "" {
				fmt.Fprintf(&b, ", support_refs=[%s]", refs)
			}
		}
		b.WriteString("\n")
	}
	if len(facts) > maxFacts {
		fmt.Fprintf(&b, "- ... %d more aggregate fact(s) omitted from prompt\n", len(facts)-maxFacts)
	}
	return b.String()
}

func aggregateFactPromptMemberLimit(fact types.AnswerAggregateFact) int {
	if types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
		return 200
	}
	return 12
}

func aggregateFactPromptSupportRefLimit(fact types.AnswerAggregateFact) int {
	if types.AnswerAggregateFactCarriesCompleteMemberSet(fact) {
		return 200
	}
	return 8
}

func aggregateFactPromptOmitExcludedCandidates(ctx *types.AgentContext) bool {
	return len(tool.EffectiveAnswerExclusionRolesForAgentContext(ctx)) > 0
}

func sanitizeAggregateExcludedCandidatesForPrompt(ctx *types.AgentContext, text string, facts []types.AnswerAggregateFact) string {
	if strings.TrimSpace(text) == "" || !aggregateFactPromptOmitExcludedCandidates(ctx) {
		return text
	}
	replacements := aggregateExcludedCandidateSurfaces(facts)
	if len(replacements) == 0 {
		return text
	}
	out := text
	for _, candidate := range replacements {
		if candidate == "" {
			continue
		}
		out = strings.ReplaceAll(out, candidate, "[excluded candidate omitted]")
	}
	return out
}

func aggregateExcludedCandidateSurfaces(facts []types.AnswerAggregateFact) []string {
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
	}
	for _, fact := range facts {
		for _, excluded := range fact.Excluded {
			add(excluded)
			add(aggregateExcludedCandidateHead(excluded))
		}
	}
	out := make([]string, 0, len(seen))
	for candidate := range seen {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

func aggregateExcludedCandidateHead(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for i, r := range raw {
		if i == 0 {
			continue
		}
		if r == '(' || r == '（' || r == ':' || r == '：' || r == ',' || r == '，' || unicode.IsSpace(r) {
			return strings.TrimSpace(raw[:i])
		}
	}
	return raw
}

func aggregateFactRenderRequestModel(ctx *types.AgentContext) *types.RequestModel {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return &ctx.AnalysisIR.RequestModel
}

func renderAggregateEvidenceOrigins(origins []types.AnswerEvidenceOrigin) string {
	if len(origins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin == types.AnswerEvidenceOriginUnknown {
			continue
		}
		parts = append(parts, "`"+string(origin)+"`")
	}
	return strings.Join(parts, ", ")
}

func aggregatePromptRoleForFact(index int, fact types.AnswerAggregateFact, principal map[int]bool, roleByIndex map[int]types.AnswerAggregateRole) types.AnswerAggregateRole {
	if role := roleByIndex[index]; role != "" {
		return role
	}
	if role := types.NormalizeAnswerAggregateRole(fact.Role); role != "" {
		return role
	}
	if fact.Kind == types.AnswerAggregateMemberSet {
		if principal[index] {
			return types.AnswerAggregateRolePrincipalAnswer
		}
		return types.AnswerAggregateRoleSupportingCoverage
	}
	return ""
}

func aggregatePromptProvenanceForFact(index int, fact types.AnswerAggregateFact, provenanceByIndex map[int]string) string {
	if provenance := strings.TrimSpace(provenanceByIndex[index]); provenance != "" {
		return provenance
	}
	return strings.TrimSpace(fact.Provenance)
}

func structuredAggregatePrincipalMemberSetRefs(ctx *types.AgentContext, facts []types.AnswerAggregateFact) []types.AnswerAggregateFactRef {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.PrincipalAggregateMemberSetFactRefs(facts)
	}
	return types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ctx.AnalysisIR.RequestModel)
}

func structuredAggregateCompactPrincipalMemberSetIndexes(ctx *types.AgentContext, facts []types.AnswerAggregateFact) map[int]bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(facts) == 0 {
		return nil
	}
	refs := structuredAggregatePrincipalMemberSetRefs(ctx, facts)
	if len(refs) == 0 {
		return nil
	}
	out := make(map[int]bool, len(refs))
	for _, ref := range refs {
		fact := ref.Fact
		if ref.Index < 0 || ref.Index >= len(facts) {
			continue
		}
		if fact.Kind != types.AnswerAggregateMemberSet ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
			len(fact.Members) == 0 {
			continue
		}
		out[ref.Index] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func structuredAggregateCompactShadowedMemberSetIndexes(ctx *types.AgentContext, facts []types.AnswerAggregateFact) map[int]bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(facts) == 0 || !structuredAggregateHasSourceInventoryPrincipalRows(facts) {
		return nil
	}
	out := map[int]bool{}
	for i, fact := range facts {
		if !strings.Contains(fact.Provenance, "demoted:shadowed_by_source_inventory_principal_row_set") {
			continue
		}
		if fact.Kind != types.AnswerAggregateMemberSet ||
			!types.AnswerAggregateFactCarriesCompleteMemberSet(fact) ||
			len(fact.Members) == 0 {
			continue
		}
		out[i] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func structuredAggregateHasSourceInventoryPrincipalRows(facts []types.AnswerAggregateFact) bool {
	for _, fact := range facts {
		if strings.TrimSpace(fact.Provenance) == types.SourceInventoryPrincipalRowSetAggregateProvenance &&
			fact.Kind == types.AnswerAggregateMemberSet &&
			types.AnswerAggregateFactCarriesCompleteMemberSet(fact) &&
			len(fact.Members) > 0 {
			return true
		}
	}
	return false
}

func structuredAggregatePrincipalFactIndexes(ctx *types.AgentContext, facts []types.AnswerAggregateFact) map[int]bool {
	out := map[int]bool{}
	for _, ref := range structuredAggregatePrincipalMemberSetRefs(ctx, facts) {
		out[ref.Index] = true
	}
	for i, fact := range facts {
		if types.NormalizeAnswerAggregateRole(fact.Role).IsPrincipal() {
			out[i] = true
		}
	}
	return out
}

func orderedAggregateFactIndexes(facts []types.AnswerAggregateFact, principalFacts map[int]bool) []int {
	indexes := make([]int, len(facts))
	for i := range facts {
		indexes[i] = i
	}
	sort.SliceStable(indexes, func(a, b int) bool {
		ia, ib := indexes[a], indexes[b]
		pa := aggregateFactPromptPriority(facts[ia], principalFacts[ia])
		pb := aggregateFactPromptPriority(facts[ib], principalFacts[ib])
		if pa != pb {
			return pa < pb
		}
		return ia < ib
	})
	return indexes
}

func aggregateFactPromptPriority(fact types.AnswerAggregateFact, principal bool) int {
	if principal || types.NormalizeAnswerAggregateRole(fact.Role).IsPrincipal() {
		return 0
	}
	switch fact.Kind {
	case types.AnswerAggregateScalar:
		return 1
	case types.AnswerAggregateMemberSet:
		return 2
	case types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount,
		types.AnswerAggregateNegativeSearch:
		return 3
	case types.AnswerAggregateExcluded:
		return 4
	default:
		return 5
	}
}

func aggregateFactMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func renderAggregateDimensions(dims []types.AnswerAggregateDimension) string {
	if len(dims) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dims))
	for _, dim := range dims {
		if dim.Name == "" || dim.Value == "" {
			continue
		}
		parts = append(parts, dim.Name+"="+dim.Value)
	}
	return strings.Join(parts, ", ")
}

func renderAggregateStringList(items []string, limit int) string {
	if len(items) == 0 || limit <= 0 {
		return ""
	}
	if len(items) < limit {
		limit = len(items)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		item := strings.TrimSpace(items[i])
		if item == "" {
			continue
		}
		parts = append(parts, "`"+item+"`")
	}
	if len(items) > limit {
		parts = append(parts, fmt.Sprintf("... +%d", len(items)-limit))
	}
	return strings.Join(parts, ", ")
}
