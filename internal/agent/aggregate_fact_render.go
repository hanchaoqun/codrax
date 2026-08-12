package agent

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Aggregate-fact prompt caps reference the unified observation-view budget
// source (Batch E2) so this render layer cannot drift from the observation
// render caps. Principal answer-payload facts are never hidden by these caps
// (see structuredAggregatePromptFactLimit).
const (
	structuredAggregateDefaultPromptFacts = types.AggregateFactsPromptDefaultLimit
	structuredAggregateMaxPromptFacts     = types.AggregateFactsPromptMaxLimit
)

func renderStructuredAggregateFactsForContext(ctx *types.AgentContext, facts []types.AnswerAggregateFact) string {
	facts = types.PruneAggregateMemberSetsByStructuredExclusions(facts)
	if ctx != nil && ctx.AnalysisIR != nil {
		facts = types.NormalizeAggregateFactRolesForRequest(facts, &ctx.AnalysisIR.RequestModel)
		facts = types.DemoteAggregateCountFactsConflictingWithPrincipalMemberSets(facts, &ctx.AnalysisIR.RequestModel)
	}
	return renderStructuredAggregateFactsWithOptions(facts, structuredAggregatePromptFactLimit(ctx, facts), structuredAggregatePrincipalMemberSetRefs(ctx, facts), aggregateFactRenderOptions{
		omitExcludedCandidates:   aggregateFactPromptOmitExcludedCandidates(ctx),
		compactMemberSetRows:     structuredAggregateCompactPrincipalMemberSetIndexes(ctx, facts),
		compactShadowedRows:      structuredAggregateCompactShadowedMemberSetIndexes(ctx, facts),
		principalContractIndexes: structuredAggregatePrincipalContractIndexes(ctx, facts),
		requestModel:             aggregateFactRenderRequestModel(ctx),
		supportEvidence:          aggregateFactRenderSupportEvidence(ctx),
	})
}

func aggregateFactRenderSupportEvidence(ctx *types.AgentContext) []types.EvidenceItem {
	if ctx == nil || len(ctx.EvidenceItems) == 0 {
		return nil
	}
	return ctx.EvidenceItems
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
	omitExcludedCandidates   bool
	compactMemberSetRows     map[int]bool
	compactShadowedRows      map[int]bool
	principalContractIndexes map[int]bool
	requestModel             *types.RequestModel
	supportEvidence          []types.EvidenceItem
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
		omitAdvisoryNumeric := aggregateFactRuntimeAdvisoryNumericMustOmitValue(opts.requestModel, fact)
		compactMembers := opts.compactMemberSetRows[i] &&
			fact.Kind == types.AnswerAggregateMemberSet &&
			types.AnswerAggregateFactCarriesCompleteMemberSet(fact) &&
			len(fact.Members) > 0
		compactShadowed := opts.compactShadowedRows[i] &&
			fact.Kind == types.AnswerAggregateMemberSet &&
			types.AnswerAggregateFactCarriesCompleteMemberSet(fact) &&
			len(fact.Members) > 0
		fmt.Fprintf(&b, "- kind=`%s`", fact.Kind)
		if omitAdvisoryNumeric {
			// A model-extracted runtime scalar/count without typed support is
			// useful as an audit receipt, but its label and value are not an
			// authorized numeric observation. Keeping the raw value in the
			// finalizer prompt after role demotion still lets it become an
			// arithmetic operand (for example an invented frame budget). This
			// projection is driven only by typed origin/shape/support fields;
			// it does not inspect user or model prose and does not mutate the
			// model-authored answer.
			fmt.Fprintf(&b, ", label_omitted=runtime_advisory_without_typed_support, value_omitted=runtime_advisory_without_typed_support, numeric_observation_authority=`not_authorized`, arithmetic_operand=`not_authorized`")
		} else {
			fmt.Fprintf(&b, ", label=%s", fact.Label)
		}
		if omitAdvisoryNumeric {
			// The omission receipt above is the complete value projection.
		} else if compactShadowed {
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
		if types.AnswerAggregateFactRoleForRequest(fact, opts.requestModel).IsPrincipal() &&
			!opts.principalContractIndexes[i] &&
			!types.AnswerAggregateFactAuthorizesPrincipalContract(fact, opts.requestModel) {
			fmt.Fprintf(&b, ", fact_authority=`advisory_model_inference`, principal_contract=`not_authorized`")
		}
		if fact.Unit != "" && !omitAdvisoryNumeric {
			fmt.Fprintf(&b, ", unit=%s", fact.Unit)
		}
		if dims := renderAggregateDimensions(fact.Dimensions); dims != "" && !omitAdvisoryNumeric {
			fmt.Fprintf(&b, ", dimensions=[%s]", dims)
		}
		if omitAdvisoryNumeric {
			// Runtime aggregate members can smuggle the same unsupported
			// arithmetic back into the prompt after the scalar value was
			// correctly withheld (for example "direction A total = X+Y").
			// Preserve only the structural receipt. Exact trace rows remain
			// available through their typed support/projection lanes.
			if len(fact.Members) > 0 {
				fmt.Fprintf(&b, ", member_count=%d, members_omitted=runtime_advisory_without_typed_support", len(fact.Members))
			}
			if len(fact.MemberNotes) > 0 {
				fmt.Fprintf(&b, ", member_note_count=%d, member_notes_omitted=runtime_advisory_without_typed_support", len(fact.MemberNotes))
			}
		} else if compactMembers {
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
			if notes := renderAggregateMemberNotes(fact.MemberNotes, aggregateFactPromptMemberNoteLimit(fact), opts.requestModel); notes != "" {
				fmt.Fprintf(&b, ", member_notes=[%s]", notes)
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
		if authority := aggregateMemberNoteSupportAuthority(fact, opts.supportEvidence); authority != "" {
			fmt.Fprintf(&b, ", member_note_support_authority=[%s]", authority)
		}
		b.WriteString("\n")
	}
	if len(facts) > maxFacts {
		dropped := map[string]int{}
		for displayIdx := maxFacts; displayIdx < len(order); displayIdx++ {
			fact := facts[order[displayIdx]]
			key := string(fact.Kind)
			if key == "" {
				key = "unknown"
			}
			if role := types.NormalizeAnswerAggregateRole(fact.Role); role != "" {
				key += "/" + string(role)
			}
			dropped[key]++
		}
		fmt.Fprintf(&b, "- ... (showing %d of %d aggregate facts; dropped: %s)\n",
			maxFacts, len(facts), formatAggregateDroppedCategories(dropped))
	}
	return b.String()
}

const aggregateMemberNoteSupportAuthorityLimit = 12

// aggregateMemberNoteSupportAuthority binds each model-authored member note
// to the deterministic ClaimForm of its positional support ref. A source
// location is only an identity until it matches an accepted EvidenceItem; a
// definition match proves a definition site, not the note's description of a
// whole function body. This is a bounded prompt projection only: it does not
// rewrite or delete model-authored notes, and it never parses user/final prose.
func aggregateMemberNoteSupportAuthority(fact types.AnswerAggregateFact, evidence []types.EvidenceItem) string {
	if len(fact.MemberNotes) == 0 || len(fact.SupportRefs) == 0 || len(evidence) == 0 {
		return ""
	}
	limit := len(fact.SupportRefs)
	if len(fact.MemberNotes) < limit {
		limit = len(fact.MemberNotes)
	}
	if limit > aggregateMemberNoteSupportAuthorityLimit {
		limit = aggregateMemberNoteSupportAuthorityLimit
	}
	entries := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		_, location, ok := types.ParseAnswerSupportRefMemberLocation(fact.SupportRefs[i])
		if !ok {
			continue
		}
		forms := aggregateSupportLocationClaimForms(location, evidence)
		if len(forms) == 0 {
			continue
		}
		parts := make([]string, 0, len(forms))
		for _, form := range forms {
			part := string(form)
			if boundary := types.MechanismAuthorityBoundaryForClaimForm(form); boundary != "" {
				part += "{" + boundary + "}"
			}
			parts = append(parts, part)
		}
		entries = append(entries, fmt.Sprintf("`%d:%s`", i+1, strings.Join(parts, "+")))
	}
	return strings.Join(entries, ", ")
}

func aggregateSupportLocationClaimForms(location types.AnswerSourceLocationSurface, evidence []types.EvidenceItem) []types.ClaimForm {
	seen := map[types.ClaimForm]bool{}
	forms := make([]types.ClaimForm, 0, 2)
	for _, item := range evidence {
		if item.GroundingStatus == types.GroundingUngrounded || strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
			continue
		}
		if !aggregateSupportLocationMatchesEvidence(location, item) {
			continue
		}
		form := types.ClaimFormOf(item)
		if form == types.ClaimUnknown || seen[form] {
			continue
		}
		seen[form] = true
		forms = append(forms, form)
	}
	sort.SliceStable(forms, func(i, j int) bool { return forms[i] < forms[j] })
	return forms
}

func aggregateSupportLocationMatchesEvidence(location types.AnswerSourceLocationSurface, item types.EvidenceItem) bool {
	if types.AnswerSourceLocationSurfaceMatchesCitation(location, types.Citation{File: item.Source, Line: item.LineStart}) {
		return true
	}
	end := item.LineEnd
	if end < item.LineStart {
		end = item.LineStart
	}
	surfaceText := fmt.Sprintf("%s:%d", item.Source, item.LineStart)
	if end > item.LineStart {
		surfaceText = fmt.Sprintf("%s:%d-%d", item.Source, item.LineStart, end)
	}
	evidenceLocation, ok := types.ParseAnswerSourceLocationSurface(surfaceText)
	return ok && types.AnswerSourceLocationSurfaceMatchesCitation(evidenceLocation, types.Citation{File: location.File, Line: location.LineStart})
}

func aggregateFactRuntimeAdvisoryNumericMustOmitValue(rm *types.RequestModel, fact types.AnswerAggregateFact) bool {
	if !types.AggregateFactIsRuntimeObservationAdvisory(rm, fact) {
		return false
	}
	switch fact.Kind {
	case types.AnswerAggregateScalar,
		types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount:
		return true
	default:
		return false
	}
}

// formatAggregateDroppedCategories renders dropped aggregate-fact categories
// as "kind[/role]×count" sorted by count (descending) then name, mirroring
// types.SummarizeDroppedObservationRecords for the aggregate-fact layer.
func formatAggregateDroppedCategories(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s×%d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
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

// aggregateFactPromptMemberNoteLimit keeps explanatory handoff available
// without letting a supporting roster duplicate an unbounded evidence dump.
// Principal enumeration rows already carry all row notes through their richer
// single-source renderer, so this path mainly rescues compact supporting
// mechanism sets whose members would otherwise arrive at finalization dry.
func aggregateFactPromptMemberNoteLimit(fact types.AnswerAggregateFact) int {
	const supportingNoteCap = 24
	if len(fact.MemberNotes) < supportingNoteCap {
		return len(fact.MemberNotes)
	}
	return supportingNoteCap
}

func renderAggregateMemberNotes(notes []string, limit int, rm *types.RequestModel) string {
	if len(notes) == 0 || limit <= 0 {
		return ""
	}
	if len(notes) < limit {
		limit = len(notes)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		note := strings.TrimSpace(notes[i])
		if note == "" {
			continue
		}
		note = types.SanitizeSourceInventoryNoteForRequest(rm, note)
		parts = append(parts, "`"+note+"`")
	}
	if len(notes) > limit {
		parts = append(parts, fmt.Sprintf("... +%d", len(notes)-limit))
	}
	return strings.Join(parts, ", ")
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
	authoritative := structuredAggregatePrincipalContractIndexes(ctx, facts)
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
		if !authoritative[ref.Index] {
			continue
		}
		out[ref.Index] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func structuredAggregatePrincipalContractIndexes(ctx *types.AgentContext, facts []types.AnswerAggregateFact) map[int]bool {
	out := map[int]bool{}
	var rm *types.RequestModel
	if ctx != nil && ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	for idx, fact := range facts {
		if types.AnswerAggregateFactAuthorizesPrincipalContract(fact, rm) {
			out[idx] = true
		}
	}
	if rm == nil {
		return out
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return out
	}
	for _, set := range types.CompileEnumerationDisplaySets(rm, plan) {
		if set.FactIndex < 0 || set.FactIndex >= len(facts) {
			continue
		}
		if types.EnumerationDisplaySetAuthorizesPrincipalContract(rm, facts[set.FactIndex], set) {
			out[set.FactIndex] = true
		}
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
