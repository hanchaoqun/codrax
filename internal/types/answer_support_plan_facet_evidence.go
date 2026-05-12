package types

import (
	"sort"
	"strings"
)

// compileFacetEvidenceSupportPlan compiles the generic per-family
// support lanes used by every non-root-cause / non-call-chain family.
// The plan composes:
//
//   - an observation lane (only when FacetObservedArtifactFact source
//     candidates exist on the active facet coverage),
//   - a principal-evidence lane keyed by the family-specific facet
//     kinds, and
//   - a facet-uncertainty lane for absence / drift / authority caveats.
//
// Returns nil when none of the lanes have entries so downstream
// renderers can skip the section entirely.
func compileFacetEvidenceSupportPlan(family QuestionFamily, rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	out := &AnswerSupportPlan{Family: family}
	if len(supportFacetCandidateIDs(plan, FacetObservedArtifactFact)) > 0 {
		if lane := compileObservedArtifactSupportLane(rm, plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
		}
	}
	if lane := compilePrincipalEvidenceSupportLane(family, plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if family == QFEnumeration {
		if lane := compileEnumerationSupportingContextLane(plan); len(lane.Entries) > 0 {
			out.Lanes = append(out.Lanes, lane)
		}
	}
	if lane := compileFacetUncertaintySupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if len(out.Lanes) == 0 {
		return nil
	}
	return out
}

func compilePrincipalEvidenceSupportLane(family QuestionFamily, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLanePrincipalEvidence,
		Title:         principalEvidenceLaneTitle(family),
		AllowedBlocks: principalEvidenceAllowedBlocks(family),
		Guidance:      principalEvidenceLaneGuidance(family),
	}
	items := PrincipalSupportEvidenceItemsForFamily(family, plan)
	if len(items) == 0 {
		return lane
	}
	limit := facetSupportEntryLimitForFamily(family, len(items))
	for _, item := range items {
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
		if len(lane.Entries) >= limit {
			break
		}
	}
	return lane
}

func principalEvidenceLaneTitle(family QuestionFamily) string {
	switch family {
	case QFConfigPrecedence:
		return "Grounded config / precedence evidence"
	case QFRoleLookup:
		return "Grounded role lookup evidence"
	case QFEnumeration:
		return "Grounded enumeration evidence"
	case QFArchitecture:
		return "Grounded architecture evidence"
	case QFComparison:
		return "Grounded comparison evidence"
	default:
		return "Grounded principal evidence"
	}
}

func principalEvidenceLaneGuidance(family QuestionFamily) string {
	base := "Use this lane for principal user-visible claims in this family. " +
		"Each entry is selected from typed facet source candidates, so it may support the block kinds listed below. " +
		"Evidence notes can enrich the cited fact, but do not add uncited helper names, search hints, prior-turn guesses, or nearby context as new principal claims. " +
		"When entries use different visible syntax or label surfaces (assignment, object/struct literal, import/path, route, macro, table label), preserve each entry's own snippet/operator instead of collapsing them into one generic wording."
	switch family {
	case QFConfigPrecedence:
		return base + " For config answers, keep scalar/table/list content to real default/config/CLI/runtime layer anchors; general precedence rules belong in prose unless this lane cites that layer."
	case QFEnumeration:
		return base + " For enumerations, each principal item must correspond to a listed entry here or to an extractor-backed symbol; do not invent missing members from context."
	case QFArchitecture:
		return base + " For architecture answers, sections should describe component responsibilities supported by these anchors; avoid turning unrelated helper calls into architectural layers."
	case QFComparison:
		return base + " For comparisons, preserve the user's bucket labels from the semantic view and use these anchors only for the bucket content they actually support."
	default:
		return base
	}
}

func principalEvidenceAllowedBlocks(family QuestionFamily) []string {
	switch family {
	case QFConfigPrecedence:
		return blockKindStrings(BlockSummary, BlockScalar, BlockTable, BlockOrderedList)
	case QFRoleLookup:
		return blockKindStrings(BlockSummary, BlockScalar, BlockSection)
	case QFEnumeration:
		return blockKindStrings(BlockSummary, BlockOrderedList, BlockTable, BlockBulletList, BlockSection)
	case QFArchitecture:
		return blockKindStrings(BlockSummary, BlockSection, BlockBulletList, BlockDiagram)
	case QFComparison:
		return blockKindStrings(BlockSummary, BlockSection, BlockTable, BlockOrderedList, BlockBulletList)
	case QFGeneric:
		return blockKindStrings(BlockSummary, BlockSection, BlockOrderedList, BlockBulletList, BlockDiagram)
	default:
		return blockKindStrings(BlockSummary)
	}
}

func principalSupportFacetKinds(family QuestionFamily) []AnswerFacetKind {
	switch family {
	case QFConfigPrecedence:
		return []AnswerFacetKind{FacetResolvedLiteralOrSymbol, FacetConfigPrecedenceRole}
	case QFRoleLookup:
		return []AnswerFacetKind{FacetResolvedLiteralOrSymbol, FacetCurrentCodePath, FacetNearestMechanism}
	case QFEnumeration:
		return []AnswerFacetKind{FacetEnumerationItem}
	case QFArchitecture:
		return []AnswerFacetKind{FacetCurrentCodePath, FacetComponentRelation, FacetDiagramSpine}
	case QFComparison:
		return []AnswerFacetKind{FacetCurrentCodePath, FacetComponentRelation}
	case QFGeneric:
		return []AnswerFacetKind{FacetResolvedLiteralOrSymbol, FacetCurrentCodePath}
	default:
		return nil
	}
}

func supportFacetCandidateIDs(plan *AnswerSurfacePlan, facets ...AnswerFacetKind) map[string]bool {
	if plan == nil || plan.FacetCoverage == nil || len(facets) == 0 {
		return nil
	}
	want := make(map[AnswerFacetKind]bool, len(facets))
	for _, facet := range facets {
		want[facet] = true
	}
	out := make(map[string]bool)
	collect := func(req FacetRequirement) {
		if !want[req.Kind] {
			return
		}
		for _, id := range req.SourceCandidate {
			id = strings.TrimSpace(id)
			if id != "" {
				out[id] = true
			}
		}
	}
	for _, req := range plan.FacetCoverage.Required {
		collect(req)
	}
	for _, req := range plan.FacetCoverage.Optional {
		collect(req)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// PrincipalSupportEvidenceItemsForFamily returns the same ordered,
// de-duplicated typed evidence pool that the principal support lane
// consumes for a question family. Consumers that need to reason about
// the principal evidence shape should use this helper rather than
// re-walking SurfaceEvidence and facet candidates with local rules.
func PrincipalSupportEvidenceItemsForFamily(family QuestionFamily, plan *AnswerSurfacePlan) []EvidenceItem {
	return principalSupportEvidenceItemsForFacets(family, plan, principalSupportFacetKinds(family)...)
}

// PrincipalSupportEvidenceItemsForFacet returns the same curated
// model-authored / de-duplicated principal evidence pool, scoped to a
// single facet. Prompt renderers use this as the answer-grade evidence
// count so broad raw SourceCandidate pools do not look like hundreds of
// equally principal facts.
func PrincipalSupportEvidenceItemsForFacet(family QuestionFamily, plan *AnswerSurfacePlan, facet AnswerFacetKind) []EvidenceItem {
	return principalSupportEvidenceItemsForFacets(family, plan, facet)
}

func principalSupportEvidenceItemsForFacets(family QuestionFamily, plan *AnswerSurfacePlan, facets ...AnswerFacetKind) []EvidenceItem {
	out := principalSupportEvidenceItemsForFacetsRaw(family, plan, facets...)
	if family == QFEnumeration {
		return enumerationPrincipalEvidenceMatchingBackbone(plan, out)
	}
	return out
}

func principalSupportEvidenceItemsForFacetsRaw(family QuestionFamily, plan *AnswerSurfacePlan, facets ...AnswerFacetKind) []EvidenceItem {
	candidates := supportFacetCandidateIDs(plan, facets...)
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(candidates))
	out := make([]EvidenceItem, 0, len(candidates))
	for _, item := range orderedFacetSupportEvidenceItems(family, plan.SurfaceEvidence) {
		id := strings.TrimSpace(item.ID)
		if id == "" || !candidates[id] || !principalEvidenceItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		location := supportEntryLocation(item)
		key := strings.ToLower(text) + "\x00" + strings.ToLower(location)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	out = preferModelAuthoredPrincipalEvidence(out)
	return dedupePrincipalSupportEvidenceBySurfaceRole(out)
}

func compileEnumerationSupportingContextLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneNearestMechanism,
		Title:         "Grounded enumeration support context",
		AllowedBlocks: blockKindStrings(BlockSummary, BlockTable, BlockSection, BlockCaveat),
		Guidance: "Use this lane to explain why the principal members satisfy the requested set, " +
			"or to disclose nearby proof boundaries. These entries are supporting mechanism / context: " +
			"do not turn them into additional principal ordered-list or bullet-list members.",
	}
	items := principalSupportEvidenceItemsForFacetsRaw(QFEnumeration, plan, principalSupportFacetKinds(QFEnumeration)...)
	if len(items) == 0 {
		return lane
	}
	for _, item := range enumerationEvidenceNotMatchingBackbone(plan, items) {
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
		if len(lane.Entries) >= facetSupportEntryLimitDefault {
			break
		}
	}
	return lane
}

func enumerationPrincipalEvidenceMatchingBackbone(plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if plan == nil || len(plan.StepBackbone) == 0 || len(items) == 0 {
		return items
	}
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		if enumerationEvidenceMatchesStepBackbone(plan.StepBackbone, item) {
			out = append(out, item)
		}
	}
	return out
}

func enumerationEvidenceNotMatchingBackbone(plan *AnswerSurfacePlan, items []EvidenceItem) []EvidenceItem {
	if plan == nil || len(plan.StepBackbone) == 0 || len(items) == 0 {
		return nil
	}
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		if !enumerationEvidenceMatchesStepBackbone(plan.StepBackbone, item) {
			out = append(out, item)
		}
	}
	return out
}

func enumerationEvidenceMatchesStepBackbone(backbone []StepSurfaceAnchor, item EvidenceItem) bool {
	source := normalizeAnswerSupportPath(item.Source)
	if source == "" || item.LineStart <= 0 {
		return false
	}
	itemNames := normalizedEnumerationEvidenceNames(item)
	for _, anchor := range backbone {
		anchorFile := normalizeAnswerSupportPath(anchor.File)
		if anchorFile == "" || anchorFile != source {
			continue
		}
		if anchor.Line > 0 && anchor.Line == item.LineStart {
			return true
		}
		if normalizedEnumerationName(anchor.Name) == "" {
			continue
		}
		if anchor.Line > 0 && absInt(anchor.Line-item.LineStart) > definitionSupportMemberCitationWindow {
			continue
		}
		if itemNames[normalizedEnumerationName(anchor.Name)] {
			return true
		}
	}
	return false
}

func normalizedEnumerationEvidenceNames(item EvidenceItem) map[string]bool {
	out := make(map[string]bool, 6+len(item.SurfaceTerms))
	for _, raw := range []string{item.AnchorSymbol, item.Subject, item.Object} {
		if name := normalizedEnumerationName(raw); name != "" {
			out[name] = true
		}
	}
	for _, raw := range item.SurfaceTerms {
		if name := normalizedEnumerationName(raw); name != "" {
			out[name] = true
		}
	}
	return out
}

func normalizedEnumerationName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToLower(NormalizedSurfaceSymbolTail(raw))
}

func preferModelAuthoredPrincipalEvidence(items []EvidenceItem) []EvidenceItem {
	modelAuthored := 0
	modelForms := make(map[ClaimForm]bool)
	for _, item := range items {
		if principalEvidenceModelAuthored(item) {
			modelAuthored++
			if form := ClaimFormOf(item); form != ClaimUnknown {
				modelForms[form] = true
			}
		}
	}
	if modelAuthored == 0 {
		return items
	}
	out := make([]EvidenceItem, 0, modelAuthored)
	for _, item := range items {
		if principalEvidenceModelAuthored(item) {
			out = append(out, item)
		}
	}
	for _, item := range items {
		if principalEvidenceModelAuthored(item) {
			continue
		}
		form := ClaimFormOf(item)
		if form != ClaimReturnFact || modelForms[form] {
			continue
		}
		modelForms[form] = true
		out = append(out, item)
	}
	return out
}

func principalEvidenceModelAuthored(item EvidenceItem) bool {
	if strings.TrimSpace(item.Producer) == "explorer.emit_evidence" {
		return true
	}
	return strings.TrimSpace(item.Producer) != "" && item.Kind.IsLLMEmittable()
}

func dedupePrincipalSupportEvidenceBySurfaceRole(items []EvidenceItem) []EvidenceItem {
	if len(items) < 2 {
		return items
	}
	seen := make(map[string]bool, len(items))
	out := make([]EvidenceItem, 0, len(items))
	for _, item := range items {
		key := principalSupportEvidenceSurfaceRoleKey(item)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, item)
	}
	return out
}

func principalSupportEvidenceSurfaceRoleKey(item EvidenceItem) string {
	location := strings.TrimSpace(strings.ToLower(supportEntryLocation(item)))
	if location == "" {
		return ""
	}
	form := ClaimFormOf(item)
	if form == ClaimUnknown {
		return ""
	}
	parts := []string{
		location,
		string(form),
		string(item.AnchorKind),
		strings.TrimSpace(item.AnchorSymbol),
		strings.TrimSpace(item.OwnerSymbol),
	}
	if item.AnchorKind != AnchorAssignment {
		parts = append(parts,
			strings.TrimSpace(item.Subject),
			strings.TrimSpace(item.Object),
			strings.TrimSpace(item.Condition),
		)
	}
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return strings.Join(parts, "\x00")
}

func facetSupportEntryLimitForFamily(family QuestionFamily, candidateCount int) int {
	if family == QFEnumeration {
		// Enumeration lane entries are principal answer members, not
		// optional enrichment. Capping them silently drops part of the
		// closed set before the finalizer sees it, so keep the full
		// typed evidence member set and let explicit uncertainty /
		// completeness contracts disclose any real boundary.
		return candidateCount
	}
	return facetSupportEntryLimitDefault
}

func orderedFacetSupportEvidenceItems(family QuestionFamily, items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := append([]EvidenceItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		leftPriority := facetSupportEvidencePriority(family, out[i])
		rightPriority := facetSupportEvidencePriority(family, out[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftSource := supportEvidenceSortLocation(out[i])
		rightSource := supportEvidenceSortLocation(out[j])
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		if out[i].LineStart != out[j].LineStart {
			return out[i].LineStart < out[j].LineStart
		}
		return strings.TrimSpace(out[i].AnchorSymbol) < strings.TrimSpace(out[j].AnchorSymbol)
	})
	return out
}

func facetSupportEvidencePriority(_ QuestionFamily, item EvidenceItem) int {
	switch {
	case item.Producer == "explorer.emit_evidence":
		return 0
	case item.Kind.IsLLMEmittable() && item.Producer != "":
		return 1
	case item.Kind == EvidenceConcrete || item.Producer == "concrete_values":
		return 2
	case strings.HasPrefix(item.Producer, "dataflow."):
		return 3
	default:
		return 2
	}
}

func principalEvidenceItemEligible(item EvidenceItem) bool {
	if item.GroundingStatus == GroundingUngrounded {
		return false
	}
	return supportEvidenceHasUsableLocation(item) && item.IsCitable()
}

func compileFacetUncertaintySupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneUncertaintyBound,
		Title:         "Evidence boundary disclosures",
		AllowedBlocks: blockKindStrings(BlockCaveat, BlockSummary),
		Guidance: "Use this lane only for scope, absence, drift, or proof-boundary disclosures. " +
			"It can qualify the principal answer but must not become extra principal list items, table rows, diagram nodes, or comparison buckets.",
	}
	if plan == nil {
		return lane
	}
	for _, item := range orderedFacetSupportEvidenceItems(QFGeneric, plan.SurfaceEvidence) {
		if !uncertaintySupportItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			text = strings.TrimSpace(EvidenceDeterministicSurfaceText(item, false))
		}
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
		if len(lane.Entries) >= facetUncertaintySupportEntryLimit {
			break
		}
	}
	return lane
}

func uncertaintySupportItemEligible(item EvidenceItem) bool {
	if !supportEvidenceHasUsableLocation(item) {
		return false
	}
	if !item.IsCitable() {
		return false
	}
	return item.ContextRole == EvidenceContextRoleAbsenceSupport ||
		item.Kind == EvidenceAbsent ||
		item.DriftReason != "" ||
		item.Authority == AuthorityConditional
}
