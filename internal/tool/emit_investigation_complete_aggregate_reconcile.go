package tool

import (
	"strconv"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

type aggregateDefinitionEvidenceCandidate struct {
	member     string
	key        string
	supportRef string
	role       types.AnswerCandidateRole
	exported   bool
}

type aggregateMemberSetGraphProfile struct {
	role     types.AnswerCandidateRole
	exported int
	known    int
	total    int
}

// reconcileCompletionAggregateFactsWithDefinitionEvidence repairs a narrow,
// structural handoff drift: the explorer has already emitted grounded
// definition evidence for principal candidates, but its aggregate member_set is
// incomplete. This pass only fires for typed scoped inventory/enumeration
// requests and only adds same-role definition anchors that are already in the
// evidence pool, so finalization does not need to re-discover members from
// prose or retry just to recover data the system already has.
func reconcileCompletionAggregateFactsWithDefinitionEvidence(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []types.AnswerAggregateFact {
	if len(facts) == 0 || !aggregateEvidenceReconciliationEnabled(ctx) {
		return facts
	}
	candidates := aggregateDefinitionEvidenceCandidates(ctx, facts, evidence)
	if len(candidates) == 0 {
		return facts
	}
	out := cloneCompletionAggregateFacts(facts)
	changed := false
	for i := range out {
		if !types.AnswerAggregateFactCarriesCompleteMemberSet(out[i]) ||
			!types.AnswerAggregateFactRoleForRequest(out[i], &ctx.AnalysisIR.RequestModel).IsPrincipal() ||
			len(out[i].Members) == 0 {
			continue
		}
		profile, ok := aggregateMemberSetDefinitionProfile(ctx, out[i], evidence)
		if !ok {
			continue
		}
		oldLen := len(out[i].Members)
		existing := make(map[string]bool, oldLen)
		for _, member := range out[i].Members {
			key := aggregateMemberKey(member)
			if key != "" {
				existing[key] = true
			}
		}
		if !profile.hasHomogeneousExportStatus() {
			continue
		}
		for _, candidate := range candidates {
			if candidate.role != profile.role {
				continue
			}
			if !profile.allowsCandidateExported(candidate.exported) {
				continue
			}
			if candidate.key == "" || existing[candidate.key] {
				continue
			}
			out[i].Members = append(out[i].Members, candidate.member)
			out[i].SupportRefs = append(out[i].SupportRefs, candidate.supportRef)
			existing[candidate.key] = true
			changed = true
		}
		if len(out[i].Members) == oldLen {
			continue
		}
		out[i].Value = strconv.Itoa(len(out[i].Members))
		reconcileAdjacentAggregateCountWithMemberSet(out, i, oldLen, len(out[i].Members))
	}
	if !changed {
		return facts
	}
	return out
}

func aggregateEvidenceReconciliationEnabled(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if !types.IsCategoryEnumerationAnswerShape(rm) ||
		rm.Predicates.IsRelationalLookup ||
		types.IsArchitectureNarrativeExplanation(rm) {
		return false
	}
	if rm.SourceScopeProfile != nil && rm.SourceScopeProfile.RequestedScope != "" {
		return true
	}
	if len(rm.AnalyzerHints.RequiredFileHints) > 0 {
		return true
	}
	if len(rm.SubTopics) > 1 || len(rm.QuestionStructure().Buckets) > 1 {
		return true
	}
	return false
}

func aggregateDefinitionEvidenceCandidates(ctx *types.BusContext, facts []types.AnswerAggregateFact, evidence []types.EvidenceItem) []aggregateDefinitionEvidenceCandidate {
	if ctx == nil || ctx.AnalysisIR == nil || len(evidence) == 0 {
		return nil
	}
	excludedRoles := map[types.AnswerCandidateRole]bool{}
	if policy := ctx.AnalysisIR.RequestModel.AnswerExclusionPolicy; policy != nil && policy.Active() {
		excludedRoles = answerDocumentEffectiveExcludedRoleSet(ctx, policy, facts, evidence)
	}
	seen := map[string]bool{}
	var out []aggregateDefinitionEvidenceCandidate
	for _, ev := range evidence {
		if !aggregateDefinitionEvidenceCandidateUsable(ctx, ev) {
			continue
		}
		sym, ok := aggregateGraphSymbolForEvidence(ctx, ev)
		if !ok || sym == nil {
			continue
		}
		if answerDocumentSymbolMatchesExcludedRole(sym, excludedRoles) {
			continue
		}
		role, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok {
			continue
		}
		member := strings.TrimSpace(sym.Name)
		key := aggregateMemberKey(member)
		ref := aggregateDefinitionEvidenceSupportRef(member, ev)
		if key == "" || ref == "" {
			continue
		}
		identity := string(role) + "\x00" + key
		if seen[identity] {
			continue
		}
		seen[identity] = true
		out = append(out, aggregateDefinitionEvidenceCandidate{
			member:     member,
			key:        key,
			supportRef: ref,
			role:       role,
			exported:   sym.Exported,
		})
	}
	return out
}

func aggregateDefinitionEvidenceCandidateUsable(ctx *types.BusContext, ev types.EvidenceItem) bool {
	if ev.Kind != types.EvidenceDirect ||
		ev.AnchorKind != types.AnchorDefinition ||
		ev.GroundingStatus == types.GroundingUngrounded ||
		strings.TrimSpace(ev.Source) == "" ||
		ev.LineStart <= 0 {
		return false
	}
	if ev.Scope != "" && ev.Scope != types.ScopeLine && ev.Scope != types.ScopeLineRange {
		return false
	}
	if !aggregateEvidenceSourceInRequestedScope(ctx, ev.Source) {
		return false
	}
	if !aggregateEvidenceSourceMatchesRequiredFileHints(ctx, ev.Source) {
		return false
	}
	return true
}

func aggregateEvidenceSourceInRequestedScope(ctx *types.BusContext, source string) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return true
	}
	scope := types.SourceScopeProduction
	if profile := ctx.AnalysisIR.RequestModel.SourceScopeProfile; profile != nil && profile.RequestedScope != "" {
		scope = profile.RequestedScope
	}
	return types.SourceScopeAllowsPathRole(scope, types.ClassifySourcePathRole(source))
}

func aggregateEvidenceSourceMatchesRequiredFileHints(ctx *types.BusContext, source string) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return true
	}
	hints := ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints
	if len(hints) == 0 {
		return true
	}
	for _, hint := range hints {
		path := strings.TrimSpace(hint.Path)
		if path == "" {
			continue
		}
		if aggregateReadFilePathMatchesQualifier(source, path) ||
			aggregateReadFilePathMatchesQualifier(path, source) {
			return true
		}
	}
	return false
}

func aggregateMemberSetDefinitionProfile(ctx *types.BusContext, fact types.AnswerAggregateFact, evidence []types.EvidenceItem) (aggregateMemberSetGraphProfile, bool) {
	roleCounts := map[types.AnswerCandidateRole]int{}
	known := 0
	exported := 0
	for _, member := range fact.Members {
		sym, ok := aggregateGraphSymbolForMember(ctx, fact, member, evidence)
		if !ok || sym == nil {
			continue
		}
		role, ok := aggregateAnswerCandidateRoleForSymbol(sym)
		if !ok {
			continue
		}
		roleCounts[role]++
		known++
		if sym.Exported {
			exported++
		}
	}
	if known == 0 || len(roleCounts) != 1 {
		return aggregateMemberSetGraphProfile{}, false
	}
	var role types.AnswerCandidateRole
	for r := range roleCounts {
		role = r
	}
	return aggregateMemberSetGraphProfile{
		role:     role,
		exported: exported,
		known:    known,
		total:    len(fact.Members),
	}, true
}

func (p aggregateMemberSetGraphProfile) hasHomogeneousExportStatus() bool {
	return p.known > 0 && (p.exported == 0 || p.exported == p.known)
}

func (p aggregateMemberSetGraphProfile) allowsCandidateExported(exported bool) bool {
	if !p.hasHomogeneousExportStatus() {
		return false
	}
	if p.exported == p.known {
		return exported
	}
	return !exported
}

func aggregateGraphSymbolForMember(ctx *types.BusContext, fact types.AnswerAggregateFact, member string, evidence []types.EvidenceItem) (*repotypes.Symbol, bool) {
	labels := aggregateMemberDisplayCandidates(member)
	if len(labels) == 0 {
		return nil, false
	}
	for _, ref := range fact.SupportRefs {
		refLabel, location, ok := aggregateMemberSupportRefParts(ref)
		if !ok || location == "" {
			continue
		}
		if !aggregateSupportLabelMatchesMember(refLabel, labels) {
			continue
		}
		file, line := aggregateLocationParts(location)
		if sym, found := aggregateGraphSymbolAtLocation(ctx, labels, file, line); found {
			return sym, true
		}
	}
	if ev, ok := aggregateMemberEvidence(member, evidence); ok {
		if sym, found := aggregateGraphSymbolForEvidence(ctx, ev); found {
			return sym, true
		}
	}
	return aggregateGraphSymbolByLabels(ctx, labels)
}

func aggregateGraphSymbolForEvidence(ctx *types.BusContext, ev types.EvidenceItem) (*repotypes.Symbol, bool) {
	labels := aggregateEvidenceMemberLabels(ev)
	return aggregateGraphSymbolAtLocation(ctx, labels, ev.Source, ev.LineStart)
}

func aggregateGraphSymbolAtLocation(ctx *types.BusContext, labels []string, source string, line int) (*repotypes.Symbol, bool) {
	if strings.TrimSpace(source) == "" || line <= 0 {
		return nil, false
	}
	candidates := aggregateGraphSymbolNameCandidates(labels)
	for _, graph := range answerDocumentExclusionGraphs(ctx) {
		for _, name := range candidates {
			for _, sym := range graph.SymbolDefs[name] {
				if aggregateGraphSymbolMatchesLocation(sym, name, source, line) {
					return sym, true
				}
			}
		}
	}
	return nil, false
}

func aggregateGraphSymbolByLabels(ctx *types.BusContext, labels []string) (*repotypes.Symbol, bool) {
	candidates := aggregateGraphSymbolNameCandidates(labels)
	var found *repotypes.Symbol
	for _, graph := range answerDocumentExclusionGraphs(ctx) {
		for _, name := range candidates {
			for _, sym := range graph.SymbolDefs[name] {
				if sym == nil || aggregateMemberKey(sym.Name) != aggregateMemberKey(name) {
					continue
				}
				if found != nil && found != sym {
					return nil, false
				}
				found = sym
			}
		}
	}
	return found, found != nil
}

func aggregateGraphSymbolNameCandidates(labels []string) []string {
	var out []string
	for _, label := range labels {
		for _, candidate := range aggregateMemberDisplayCandidates(label) {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			out = append(out, candidate)
			if tail := aggregateMemberKey(candidate); tail != "" {
				out = append(out, tail)
			}
		}
	}
	return dedupStringsPreserveOrder(out)
}

func aggregateGraphSymbolMatchesLocation(sym *repotypes.Symbol, name, source string, line int) bool {
	if sym == nil || strings.TrimSpace(sym.Name) == "" {
		return false
	}
	if aggregateMemberKey(sym.Name) != aggregateMemberKey(name) {
		return false
	}
	if !aggregateReadFilePathMatchesQualifier(sym.File, source) &&
		!aggregateReadFilePathMatchesQualifier(source, sym.File) {
		return false
	}
	if sym.Line == line {
		return true
	}
	return sym.Line > 0 && sym.EndLine >= sym.Line && line >= sym.Line && line <= sym.EndLine
}

func aggregateAnswerCandidateRoleForSymbol(sym *repotypes.Symbol) (types.AnswerCandidateRole, bool) {
	if sym == nil {
		return types.AnswerCandidateRoleUnknown, false
	}
	switch kind := strings.ToLower(strings.TrimSpace(sym.Kind)); {
	case kind == "function":
		return types.AnswerCandidateRoleFunction, true
	case kind == "method":
		return types.AnswerCandidateRoleMethod, true
	case kind == "const" || kind == "constant":
		return types.AnswerCandidateRoleConstant, true
	case kind == "var" || kind == "variable":
		return types.AnswerCandidateRoleVariable, true
	case kind == "field" || kind == "property":
		return types.AnswerCandidateRoleField, true
	case kind == "package" || kind == "module" || kind == "namespace":
		return types.AnswerCandidateRolePackage, true
	case answerDocumentSymbolKindIsType(kind):
		return types.AnswerCandidateRoleType, true
	default:
		return types.AnswerCandidateRoleUnknown, false
	}
}

func aggregateDefinitionEvidenceSupportRef(member string, ev types.EvidenceItem) string {
	member = strings.TrimSpace(member)
	loc := aggregateSupportLocationKey(ev.Source, ev.LineStart)
	if member == "" || loc == "" {
		return ""
	}
	return member + ": " + loc
}

func aggregateLocationParts(location string) (string, int) {
	idx := strings.LastIndex(location, ":")
	if idx <= 0 || idx+1 >= len(location) {
		return "", 0
	}
	line, err := strconv.Atoi(strings.TrimSpace(location[idx+1:]))
	if err != nil || line <= 0 {
		return "", 0
	}
	return strings.TrimSpace(location[:idx]), line
}

func reconcileAdjacentAggregateCountWithMemberSet(facts []types.AnswerAggregateFact, memberSetIdx, oldLen, newLen int) {
	if oldLen <= 0 || newLen <= oldLen {
		return
	}
	oldValue := strconv.Itoa(oldLen)
	newValue := strconv.Itoa(newLen)
	for _, idx := range []int{memberSetIdx - 1, memberSetIdx + 1} {
		if idx < 0 || idx >= len(facts) {
			continue
		}
		if !aggregateFactKindCanMirrorMemberSetCount(facts[idx].Kind) {
			continue
		}
		if strings.TrimSpace(facts[idx].Value) != oldValue {
			continue
		}
		facts[idx].Value = newValue
	}
}

func aggregateFactKindCanMirrorMemberSetCount(kind types.AnswerAggregateKind) bool {
	switch kind {
	case types.AnswerAggregateTotalCount,
		types.AnswerAggregateUniqueCount,
		types.AnswerAggregateGroupedCount,
		types.AnswerAggregateBucketCount,
		types.AnswerAggregateScalar:
		return true
	default:
		return false
	}
}
