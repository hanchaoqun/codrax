package types

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// AnswerSupportPlan is the typed support-lane contract between the
// compiled answer surface and the finalizer prompt. Unlike free-form
// closure prose, support lanes describe what kind of user-visible
// claims are safe to build and which grounded evidence entries belong
// to each lane.
type AnswerSupportPlan struct {
	Family QuestionFamily
	Lanes  []AnswerSupportLane
}

type AnswerSupportLaneKind string

const (
	SupportLaneObservedArtifact  AnswerSupportLaneKind = "observed_artifact"
	SupportLanePrincipalEvidence AnswerSupportLaneKind = "principal_evidence"
	SupportLaneCurrentCodePath   AnswerSupportLaneKind = "current_code_path"
	SupportLaneNearestMechanism  AnswerSupportLaneKind = "nearest_mechanism"
	SupportLaneUncertaintyBound  AnswerSupportLaneKind = "uncertainty_boundary"
	SupportLaneCurrentVerdict    AnswerSupportLaneKind = "current_status_verdict"
)

const callChainSupportEntryLimit = 24
const facetSupportEntryLimit = 18

type AnswerSupportLane struct {
	Kind          AnswerSupportLaneKind
	Title         string
	Guidance      string
	AllowedBlocks []string
	Entries       []AnswerSupportEntry
}

type AnswerSupportEntry struct {
	Text     string
	Detail   string
	Location string
}

// BuildAnswerSupportPlanForAgentContext compiles the current typed
// support lanes for the active family. Returning nil means the family
// currently uses no additional support-lane contract beyond the base
// AnswerSurfacePlan / AnswerSemanticView.
func BuildAnswerSupportPlanForAgentContext(ctx *AgentContext) *AnswerSupportPlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	plan := BuildAnswerSurfacePlanForAgentContext(ctx)
	if plan == nil {
		return nil
	}
	if len(ctx.AnswerSymbols) > 0 {
		ApplyAnswerSymbolStepBackbone(plan, ctx.AnalysisIR, ctx.AnswerSymbols, ctx.AnswerSymbolCompleteness)
	}
	view := BuildAnswerSemanticViewForAgentContext(ctx)
	if view != nil {
		if out := buildAnswerSupportPlanForFamily(view.Family, ctx.AnalysisIR.RequestModel, plan); out != nil {
			return augmentCurrentStatusVerdictLane(out, view.CurrentStatusDiagnostic)
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		return augmentCurrentStatusVerdictLane(
			buildAnswerSupportPlanForFamily(QFRootCauseTrace, ctx.AnalysisIR.RequestModel, plan),
			currentStatusDiagnosticContractFromIR(ctx.AnalysisIR),
		)
	}
	return augmentCurrentStatusVerdictLane(
		BuildAnswerSupportPlan(ctx.AnalysisIR.RequestModel, plan),
		currentStatusDiagnosticContractFromIR(ctx.AnalysisIR),
	)
}

// BuildAnswerSupportPlan compiles a family-aware support-lane view from
// the resolved RequestModel and current AnswerSurfacePlan. Root-cause
// and call-chain families use specialised lanes for artifact/current-
// path/mechanism boundaries; other families reuse facet-backed
// principal evidence lanes so "main answer" content and exploratory
// context stay separated without per-case prompt patches.
func BuildAnswerSupportPlan(rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	return buildAnswerSupportPlanForFamily(ResolveQuestionFamily(rm), rm, plan)
}

// BuildAnswerSupportPlanForBusContext mirrors
// BuildAnswerSupportPlanForAgentContext for the orchestrator's
// BusContext — the same compile rules but reachable from the
// contract-check dispatch path which already holds *BusContext + mut.
// Returns nil when the bus state is incomplete (no AnalysisIR) or
// when no family currently materialises a support-lane plan.
func BuildAnswerSupportPlanForBusContext(bus *BusContext) *AnswerSupportPlan {
	if bus == nil || bus.AnalysisIR == nil {
		return nil
	}
	plan := BuildAnswerSurfacePlanForBusContext(bus)
	if plan == nil {
		return nil
	}
	view := BuildAnswerSemanticViewForBusContext(bus)
	if view != nil {
		if out := buildAnswerSupportPlanForFamily(view.Family, bus.AnalysisIR.RequestModel, plan); out != nil {
			return augmentCurrentStatusVerdictLane(out, view.CurrentStatusDiagnostic)
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		return augmentCurrentStatusVerdictLane(
			buildAnswerSupportPlanForFamily(QFRootCauseTrace, bus.AnalysisIR.RequestModel, plan),
			currentStatusDiagnosticContractFromIR(bus.AnalysisIR),
		)
	}
	return augmentCurrentStatusVerdictLane(
		BuildAnswerSupportPlan(bus.AnalysisIR.RequestModel, plan),
		currentStatusDiagnosticContractFromIR(bus.AnalysisIR),
	)
}

func currentStatusDiagnosticContractFromIR(ir *AnalysisIR) *CurrentStatusDiagnosticContract {
	if ir == nil || ir.AnswerContract.CurrentStatusDiagnostic == nil ||
		!ir.AnswerContract.CurrentStatusDiagnostic.Required {
		return nil
	}
	return ir.AnswerContract.CurrentStatusDiagnostic
}

func buildAnswerSupportPlanForFamily(family QuestionFamily, rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	switch family {
	case QFRootCauseTrace:
		return compileRootCauseSupportPlan(rm, plan)
	case QFCallChain:
		return compileCallChainSupportPlan(rm, plan)
	case QFConfigPrecedence, QFRoleLookup, QFEnumeration, QFArchitecture, QFComparison, QFGeneric:
		return compileFacetEvidenceSupportPlan(family, rm, plan)
	default:
		return nil
	}
}

func compileRootCauseSupportPlan(rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	mechanismStrength := rootCauseMechanismSupportStrength(plan)
	out := &AnswerSupportPlan{Family: QFRootCauseTrace}

	if lane := compileObservedArtifactSupportLane(rm, plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileCurrentCodePathSupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileNearestMechanismSupportLane(plan, mechanismStrength); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileUncertaintyBoundarySupportLane(plan, mechanismStrength); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if len(out.Lanes) == 0 {
		return nil
	}
	return out
}

func compileCallChainSupportPlan(rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	out := &AnswerSupportPlan{Family: QFCallChain}
	if lane := compileObservedArtifactSupportLane(rm, plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileCallChainCurrentPathSupportLane(rm, plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileCallChainUncertaintySupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if len(out.Lanes) == 0 {
		return nil
	}
	return out
}

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
	candidates := supportFacetCandidateIDs(plan, principalSupportFacetKinds(family)...)
	if len(candidates) == 0 {
		return lane
	}
	seen := make(map[string]bool, len(candidates))
	for _, item := range orderedFacetSupportEvidenceItems(plan.SurfaceEvidence) {
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
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Detail:   callChainEvidenceSupportDetail(item, text),
			Location: location,
		})
		if len(lane.Entries) >= facetSupportEntryLimit {
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
		"Evidence notes can enrich the cited fact, but do not add uncited helper names, search hints, prior-turn guesses, or nearby context as new principal claims."
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
		return blockKindStrings(BlockSummary, BlockSection, BlockTable)
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
		return []AnswerFacetKind{FacetCurrentCodePath}
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

func orderedFacetSupportEvidenceItems(items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	out := append([]EvidenceItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
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
	for _, item := range orderedFacetSupportEvidenceItems(plan.SurfaceEvidence) {
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
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Detail:   callChainEvidenceSupportDetail(item, text),
			Location: supportEntryLocation(item),
		})
		if len(lane.Entries) >= 4 {
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

func supportEvidenceHasUsableLocation(item EvidenceItem) bool {
	if strings.TrimSpace(supportEntryLocation(item)) != "" {
		return true
	}
	switch item.Scope {
	case ScopeCrossfile:
		return item.CrossfileQuery != nil && len(item.CrossfileQuery.Files) > 0
	case ScopeNegative:
		return item.NegativeQuery != nil && strings.TrimSpace(item.NegativeQuery.File) != ""
	}
	return false
}

func supportEvidenceSortLocation(item EvidenceItem) string {
	if loc := strings.TrimSpace(strings.ReplaceAll(supportEntryLocation(item), `\`, `/`)); loc != "" {
		return loc
	}
	if item.Scope == ScopeCrossfile && item.CrossfileQuery != nil && len(item.CrossfileQuery.Files) > 0 {
		files := append([]string(nil), item.CrossfileQuery.Files...)
		for i := range files {
			files[i] = strings.TrimSpace(strings.ReplaceAll(files[i], `\`, `/`))
		}
		sort.Strings(files)
		return strings.Join(files, ",")
	}
	return ""
}

func blockKindStrings(kinds ...AnswerBlockKind) []string {
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if kind != "" {
			out = append(out, string(kind))
		}
	}
	return out
}

func compileCallChainCurrentPathSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneCurrentCodePath,
		Title:         "Current grounded call chain",
		AllowedBlocks: []string{"summary", "ordered_list", "diagram"},
		Guidance: "Use this lane for the principal ordered path and any sequence diagram. " +
			"Preserve the hop order already established by these entries. Do not add nearby helpers, " +
			"search-hint subjects, prior-turn subjects, or runtime frames as additional principal hops " +
			"unless they also appear in this lane or are separately cited as part of the same requested chain.",
	}
	if plan == nil {
		return lane
	}
	seen := make(map[string]bool)
	add := func(entry AnswerSupportEntry) {
		entry.Text = strings.TrimSpace(entry.Text)
		entry.Location = strings.TrimSpace(strings.ReplaceAll(entry.Location, `\`, `/`))
		if entry.Text == "" || len(lane.Entries) >= callChainSupportEntryLimit {
			return
		}
		key := strings.ToLower(entry.Text) + "\x00" + strings.ToLower(entry.Location)
		if seen[key] {
			return
		}
		seen[key] = true
		lane.Entries = append(lane.Entries, entry)
	}
	for _, entry := range selectCallChainSupportEntries(rm, plan) {
		add(entry)
	}
	return lane
}

func selectCallChainSupportEntries(rm RequestModel, plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil {
		return nil
	}
	endpoints := callChainRequestedEndpointHints(rm)
	stepEntries := callChainStepBackboneEntries(plan)
	evidenceEntries := callChainSurfaceEvidenceEntries(rm, plan)
	if callChainPreferSurfaceEvidence(rm, stepEntries, evidenceEntries) {
		return callChainCondenseSupportEntries(evidenceEntries, endpoints, callChainSupportEntryLimit)
	}
	if len(stepEntries) > 0 {
		return callChainCondenseSupportEntries(stepEntries, endpoints, callChainSupportEntryLimit)
	}
	return callChainCondenseSupportEntries(evidenceEntries, endpoints, callChainSupportEntryLimit)
}

func callChainStepBackboneEntries(plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil || len(plan.StepBackbone) == 0 {
		return nil
	}
	out := make([]AnswerSupportEntry, 0, len(plan.StepBackbone))
	for _, anchor := range plan.StepBackbone {
		text := callChainStepSupportText(anchor)
		if text == "" {
			continue
		}
		out = append(out, AnswerSupportEntry{
			Text:     text,
			Detail:   callChainStepSupportDetail(anchor, text),
			Location: stepSurfaceAnchorLocation(anchor),
		})
	}
	return out
}

func callChainSurfaceEvidenceEntries(rm RequestModel, plan *AnswerSurfacePlan) []AnswerSupportEntry {
	if plan == nil {
		return nil
	}
	var out []AnswerSupportEntry
	for _, item := range orderedCallChainSupportEvidenceItems(rm, plan) {
		if !callChainPathItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		out = append(out, AnswerSupportEntry{
			Text:     text,
			Detail:   callChainEvidenceSupportDetail(item, text),
			Location: supportEntryLocation(item),
		})
	}
	return out
}

func orderedCallChainSupportEvidenceItems(rm RequestModel, plan *AnswerSurfacePlan) []EvidenceItem {
	items := callChainSupportEvidenceItems(plan)
	if len(items) == 0 {
		return nil
	}
	out := append([]EvidenceItem(nil), items...)
	if callChainShouldSortSurfaceEvidenceByLine(rm, out) {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].LineStart == out[j].LineStart {
				return strings.TrimSpace(out[i].AnchorSymbol) < strings.TrimSpace(out[j].AnchorSymbol)
			}
			return out[i].LineStart < out[j].LineStart
		})
	}
	return out
}

func callChainShouldSortSurfaceEvidenceByLine(rm RequestModel, items []EvidenceItem) bool {
	if len(items) < 3 {
		return false
	}
	if rm.Intent != IntentTrace && NormalizeRequirementKind(rm.AnalyzerHints.Kind) != ReqCallChain {
		return false
	}
	var source string
	count := 0
	for _, item := range items {
		if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
			continue
		}
		canonical := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" {
			source = canonical
		}
		if canonical != source {
			return false
		}
		count++
	}
	return source != "" && count >= 3
}

func callChainStepSupportDetail(anchor StepSurfaceAnchor, text string) string {
	for _, raw := range []string{anchor.Chain, anchor.Rationale} {
		if detail := answerSupportEntryDetail(raw, text); detail != "" {
			return detail
		}
	}
	return ""
}

func callChainEvidenceSupportDetail(item EvidenceItem, text string) string {
	detail := strings.TrimSpace(item.Summary)
	if cond := strings.TrimSpace(item.Condition); cond != "" && !strings.Contains(strings.ToLower(detail), strings.ToLower(cond)) {
		if detail == "" {
			detail = "condition: " + cond
		} else {
			detail += "; condition: " + cond
		}
	}
	return answerSupportEntryDetail(detail, text)
}

func answerSupportEntryDetail(raw, text string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(text), strings.ToLower(raw)) {
		return ""
	}
	const max = 260
	if len(raw) > max {
		raw = strings.TrimSpace(raw[:max]) + "..."
	}
	return raw
}

func callChainPreferSurfaceEvidence(
	rm RequestModel,
	stepEntries []AnswerSupportEntry,
	evidenceEntries []AnswerSupportEntry,
) bool {
	if len(evidenceEntries) == 0 {
		return false
	}
	if len(stepEntries) == 0 {
		return true
	}
	endpoints := callChainRequestedEndpointHints(rm)
	if len(endpoints) == 0 {
		return false
	}
	stepCoverage := callChainEndpointCoverage(stepEntries, endpoints)
	evidenceCoverage := callChainEndpointCoverage(evidenceEntries, endpoints)
	if evidenceCoverage > stepCoverage {
		return true
	}
	if evidenceCoverage == stepCoverage && evidenceCoverage > 0 && len(evidenceEntries) > len(stepEntries)+1 {
		return true
	}
	return false
}

func callChainCondenseSupportEntries(entries []AnswerSupportEntry, endpoints []string, limit int) []AnswerSupportEntry {
	if limit <= 0 || len(entries) <= limit {
		return append([]AnswerSupportEntry(nil), entries...)
	}
	selected := make(map[int]bool, limit)
	add := func(idx int) {
		if idx < 0 || idx >= len(entries) || selected[idx] {
			return
		}
		if len(selected) >= limit {
			return
		}
		selected[idx] = true
	}
	add(0)
	for i := 1; i <= 6; i++ {
		add(len(entries) - i)
	}
	terminalEndpoints := callChainTerminalEndpointHints(endpoints)
	for i, entry := range entries {
		for _, endpoint := range terminalEndpoints {
			if callChainEntryMentionsEndpoint(entry, endpoint) {
				add(i)
				break
			}
		}
	}
	for slot := 0; slot < limit && len(selected) < limit; slot++ {
		idx := 0
		if limit > 1 {
			idx = slot * (len(entries) - 1) / (limit - 1)
		}
		add(idx)
	}
	if len(selected) < limit {
		for i := range entries {
			add(i)
			if len(selected) >= limit {
				break
			}
		}
	}
	indices := make([]int, 0, len(selected))
	for idx := range selected {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	out := make([]AnswerSupportEntry, 0, len(indices))
	for _, idx := range indices {
		out = append(out, entries[idx])
	}
	return out
}

func callChainTerminalEndpointHints(endpoints []string) []string {
	if len(endpoints) == 0 {
		return nil
	}
	last := strings.TrimSpace(endpoints[len(endpoints)-1])
	if last == "" {
		return nil
	}
	return []string{last}
}

func callChainEndpointCoverage(entries []AnswerSupportEntry, endpoints []string) int {
	if len(entries) == 0 || len(endpoints) == 0 {
		return 0
	}
	covered := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		for _, entry := range entries {
			if callChainEntryMentionsEndpoint(entry, endpoint) {
				covered[strings.ToLower(endpoint)] = true
				break
			}
		}
	}
	return len(covered)
}

func callChainEntryMentionsEndpoint(entry AnswerSupportEntry, endpoint string) bool {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return false
	}
	haystacks := []string{entry.Text, entry.Location}
	for _, haystack := range haystacks {
		if callChainEndpointCompatible(haystack, endpoint) {
			return true
		}
	}
	return false
}

func callChainEndpointCompatible(candidate, endpoint string) bool {
	candidate = strings.TrimSpace(candidate)
	endpoint = strings.TrimSpace(endpoint)
	if candidate == "" || endpoint == "" {
		return false
	}
	cLower := strings.ToLower(candidate)
	eLower := strings.ToLower(endpoint)
	if strings.Contains(cLower, eLower) {
		return true
	}
	cTail := normalizedSurfaceSymbolTail(candidate)
	eTail := normalizedSurfaceSymbolTail(endpoint)
	if cTail == "" || eTail == "" {
		return false
	}
	return cTail == eTail ||
		strings.HasPrefix(cTail, eTail) ||
		strings.HasPrefix(eTail, cTail)
}

func callChainRequestedEndpointHints(rm RequestModel) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || callChainEndpointHintLooksLikePath(raw) {
			return
		}
		key := strings.ToLower(raw)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, raw)
	}
	for _, entity := range rm.AnalyzerHints.MentionedEntities {
		add(entity)
	}
	if len(out) == 0 {
		for _, entity := range rm.AnalyzerHints.PrimaryEntities {
			add(entity)
		}
	}
	if len(out) == 0 {
		for _, entity := range rm.AnalyzerHints.Entities {
			add(entity)
		}
	}
	return out
}

func callChainEndpointHintLooksLikePath(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`)))
	if lower == "" || strings.Contains(lower, "/") {
		return true
	}
	for _, suffix := range []string{
		".go", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx",
		".cj", ".cjo", ".ets", ".ts", ".js", ".jsx", ".tsx",
		".java", ".kt", ".kts", ".py", ".rs", ".rb", ".php", ".swift",
		".yaml", ".yml", ".json", ".toml", ".ini", ".xml", ".md",
	} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func callChainStepSupportText(anchor StepSurfaceAnchor) string {
	name := strings.TrimSpace(anchor.Name)
	text := strings.TrimSpace(anchor.SurfaceText)
	if text == "" {
		text = strings.TrimSpace(anchor.Rationale)
	}
	switch {
	case name == "" && text == "":
		return ""
	case name == "":
		return text
	case text == "":
		return fmt.Sprintf("`%s` is one grounded hop in the resolved sequence.", name)
	default:
		return fmt.Sprintf("`%s` — %s", name, text)
	}
}

func callChainSupportEvidenceItems(plan *AnswerSurfacePlan) []EvidenceItem {
	if plan == nil {
		return nil
	}
	if len(plan.DriftBoundedSurfaceItems) > 0 {
		return DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems)
	}
	if len(plan.SurfaceEvidence) == 0 {
		return nil
	}
	candidateIDs := callChainPrincipalCandidateIDs(plan.FacetCoverage)
	out := make([]EvidenceItem, 0, len(plan.SurfaceEvidence))
	for _, item := range plan.SurfaceEvidence {
		if len(candidateIDs) > 0 {
			id := strings.TrimSpace(item.ID)
			if id == "" || !candidateIDs[id] {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func callChainPrincipalCandidateIDs(facets *FacetCoverageContract) map[string]bool {
	if facets == nil {
		return nil
	}
	out := make(map[string]bool)
	collect := func(req FacetRequirement) {
		switch req.Kind {
		case FacetPrincipalPathEdge, FacetCurrentCodePath:
		default:
			return
		}
		for _, id := range req.SourceCandidate {
			id = strings.TrimSpace(id)
			if id != "" {
				out[id] = true
			}
		}
	}
	for _, req := range facets.Required {
		collect(req)
	}
	for _, req := range facets.Optional {
		collect(req)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compileCallChainUncertaintySupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneUncertaintyBound,
		Title:         "Call-chain boundary disclosures",
		AllowedBlocks: []string{"summary", "caveat"},
		Guidance: "Use this lane only to disclose runtime/current-code drift, incomplete chain proof, " +
			"or scope limits. Do not turn these entries into ordered-list hops or diagram edges.",
	}
	if plan == nil {
		return lane
	}
	for _, anchor := range plan.LogSourceDriftAnchors {
		file := strings.TrimSpace(anchor.File)
		if file == "" || anchor.ObservedLine <= 0 || anchor.AnchoredLine <= 0 {
			continue
		}
		funcLabel := strings.TrimSpace(firstNonEmptySurfaceString(anchor.Func, anchor.OriginalFunc))
		text := fmt.Sprintf("observed chain frame %s:%d", file, anchor.ObservedLine)
		if funcLabel != "" {
			text += fmt.Sprintf(" in %s", funcLabel)
		}
		text += fmt.Sprintf(" maps to current grounded anchor %s:%d", file, anchor.AnchoredLine)
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Location: fmt.Sprintf("%s:%d", file, anchor.AnchoredLine),
		})
		if len(lane.Entries) >= 3 {
			break
		}
	}
	return lane
}

func callChainPathItemEligible(item EvidenceItem) bool {
	switch item.Kind {
	case EvidenceDirect, EvidenceConditional, EvidenceRegistration, EvidenceMechanism, EvidenceRelationship:
	default:
		return false
	}
	switch item.AnchorKind {
	case AnchorCall, AnchorDefinition, AnchorCondition, AnchorAssignment, AnchorReturn:
		return item.GroundingStatus != GroundingUngrounded
	default:
		return false
	}
}

func augmentCurrentStatusVerdictLane(
	plan *AnswerSupportPlan,
	contract *CurrentStatusDiagnosticContract,
) *AnswerSupportPlan {
	if plan == nil || contract == nil || !contract.Required {
		return plan
	}
	for _, lane := range plan.Lanes {
		if lane.Kind == SupportLaneCurrentVerdict {
			return plan
		}
	}
	lane := compileCurrentStatusVerdictSupportLane(plan)
	if len(lane.Entries) == 0 {
		return plan
	}
	out := *plan
	out.Lanes = append(append([]AnswerSupportLane(nil), plan.Lanes...), lane)
	return &out
}

func compileCurrentStatusVerdictSupportLane(plan *AnswerSupportPlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneCurrentVerdict,
		Title:         "Current status verdict synthesis",
		AllowedBlocks: []string{"decision"},
		Guidance: "Use this lane only for the bounded verdict block. It may cite the historical " +
			"observation, current code verification, and boundary evidence together, but it must not " +
			"be rendered as path steps, diagram nodes, or a standalone mechanism story.",
	}
	if plan == nil {
		return lane
	}
	seen := make(map[string]struct{})
	for _, sourceLane := range plan.Lanes {
		if sourceLane.Kind == SupportLaneCurrentVerdict {
			continue
		}
		title := strings.TrimSpace(sourceLane.Title)
		if title == "" {
			title = string(sourceLane.Kind)
		}
		for _, entry := range sourceLane.Entries {
			location := strings.TrimSpace(entry.Location)
			if location == "" {
				continue
			}
			key := strings.ToLower(strings.ReplaceAll(location, `\`, `/`))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			text := strings.TrimSpace(entry.Text)
			if text == "" {
				text = location
			}
			lane.Entries = append(lane.Entries, AnswerSupportEntry{
				Text:     fmt.Sprintf("%s verdict support: %s", title, text),
				Detail:   entry.Detail,
				Location: location,
			})
			if len(lane.Entries) >= 8 {
				return lane
			}
		}
	}
	return lane
}

func compileObservedArtifactSupportLane(rm RequestModel, plan *AnswerSurfacePlan) AnswerSupportLane {
	allowedBlocks := []string{"summary", "caveat"}
	if runtimeObservationOnly(plan) {
		allowedBlocks = []string{"summary", "ordered_list", "bullet_list", "caveat"}
		if diagramPreferredByEvidence(plan) {
			allowedBlocks = append(allowedBlocks, "diagram")
		}
	}
	lane := AnswerSupportLane{
		Kind:          SupportLaneObservedArtifact,
		Title:         "Observed artifact facts",
		AllowedBlocks: allowedBlocks,
		Guidance: "Use this lane only for facts that came from the attached runtime artifact " +
			"(log / perf trace / external observation). The system populates this lane " +
			"from items the typed evidence projector tagged Origin=log or Origin=perf, " +
			"so every entry here is structurally an observation, not current-code " +
			"mechanism. These facts can explain what was observed (which frame fired, " +
			"which signal triggered) but they do not prove caller-side provenance, " +
			"source-parameter mapping, or exact downstream branch execution unless " +
			"a separately-cited current-code line establishes that mapping.",
	}
	if runtimeObservationOnly(plan) {
		lane.Guidance += " For an external-only runtime artifact with no current-repo intersection, this lane is allowed to carry the principal answer list itself: each item should be an observed frame / event / span from the artifact with citation_ref=-1. Do not substitute current-repo analysis helpers, resolver functions, or nearby implementation details for the artifact facts the user asked about."
		if len(principalObservationTargets(rm)) > 0 {
			lane.Guidance += " This dispatch has analyzer-resolved principal artifact targets; only frame entries listed in this lane are principal-safe. Other frames from the full log are supporting context unless the user explicitly asked for the full stack."
		}
	}
	for _, seed := range selectSupportExternalObservationSeeds(rm, plan.ExternalObservationSeeds, ExternalObservationPromptSeedLimit) {
		text, location := renderExternalObservationSupportEntry(seed)
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Detail:   externalObservationSupportDetail(seed, text),
			Location: location,
		})
	}
	return lane
}

func selectSupportExternalObservationSeeds(rm RequestModel, seeds []ExternalObservationSeed, limit int) []ExternalObservationSeed {
	if limit <= 0 || len(seeds) == 0 {
		return nil
	}
	targets := principalObservationTargets(rm)
	if len(targets) == 0 {
		return SelectExternalObservationSeedsForPrompt(seeds, limit)
	}
	out := make([]ExternalObservationSeed, 0, limit)
	seen := make(map[string]bool, limit)
	add := func(seed ExternalObservationSeed) bool {
		if len(out) >= limit {
			return false
		}
		key := externalObservationSeedKey(seed)
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
		out = append(out, seed)
		return true
	}

	for _, seed := range SelectExternalObservationSeedsForPrompt(seeds, limit) {
		if externalObservationSeedIsFrame(seed) {
			continue
		}
		add(seed)
		if len(out) >= 2 {
			break
		}
	}
	addTargetFrames := func(headOnly bool) int {
		added := 0
		for _, target := range targets {
			for _, seed := range seeds {
				if !externalObservationSeedIsFrame(seed) {
					continue
				}
				if headOnly && !externalObservationSeedIsErrorHeadFrame(seed) {
					continue
				}
				if externalObservationSeedMatchesTarget(seed, target) && add(seed) {
					added++
					break
				}
			}
			if len(out) >= limit {
				return added
			}
		}
		return added
	}
	if addTargetFrames(true) > 0 {
		return out
	}
	addTargetFrames(false)
	if len(out) > 0 {
		return out
	}
	return SelectExternalObservationSeedsForPrompt(seeds, limit)
}

func principalObservationTargets(rm RequestModel) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, topic := range rm.SubTopics {
		for _, entity := range topic.Entities {
			add(entity)
		}
	}
	return out
}

func externalObservationSeedMatchesTarget(seed ExternalObservationSeed, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, candidate := range []string{
		seed.Func,
		normalizedSurfaceSymbolTail(seed.Func),
		seed.File,
		fileBaseNoExt(seed.File),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.EqualFold(candidate, target) ||
			strings.EqualFold(normalizedSurfaceSymbolTail(candidate), normalizedSurfaceSymbolTail(target)) {
			return true
		}
	}
	return false
}

func fileBaseNoExt(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	if path == "" {
		return ""
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndex(path, "."); i > 0 {
		path = path[:i]
	}
	return path
}

func renderExternalObservationSupportEntry(seed ExternalObservationSeed) (string, string) {
	raw := strings.TrimSpace(seed.Raw)
	switch strings.TrimSpace(seed.Kind) {
	case "error_type":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("structured runtime error type %q", raw), ""
	case "error_message":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("structured runtime error message %q", raw), ""
	case "signal":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("structured runtime signal %q", raw), ""
	case "log_observation":
		if raw == "" {
			return "", ""
		}
		if subject := strings.TrimSpace(seed.Func); subject != "" {
			return fmt.Sprintf("structured runtime observation about %q: %s", subject, raw), ""
		}
		return fmt.Sprintf("structured runtime observation: %s", raw), ""
	case "perf_jank":
		if subject := strings.TrimSpace(seed.Func); subject != "" {
			return fmt.Sprintf("performance trace observed jank around %q: %s", subject, raw), ""
		}
		return fmt.Sprintf("performance trace observed jank: %s", raw), ""
	case "perf_stall":
		loc := ""
		if file := strings.TrimSpace(strings.ReplaceAll(seed.File, `\`, `/`)); file != "" && seed.Line > 0 {
			loc = fmt.Sprintf("%s:%d", file, seed.Line)
		}
		if subject := strings.TrimSpace(seed.Func); subject != "" {
			return fmt.Sprintf("performance trace observed stall at %q: %s", subject, raw), loc
		}
		return fmt.Sprintf("performance trace observed stall: %s", raw), loc
	case "perf_frame", "perf_startup":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("performance trace observation: %s", raw), ""
	}
	if funcLabel := strings.TrimSpace(seed.Func); funcLabel != "" {
		observedLoc := ""
		if file := strings.TrimSpace(strings.ReplaceAll(seed.File, `\`, `/`)); file != "" && seed.Line > 0 {
			observedLoc = fmt.Sprintf("%s:%d", file, seed.Line)
		}
		rolePrefix := "runtime artifact includes stack frame"
		switch strings.TrimSpace(seed.Role) {
		case "error_head_frame":
			rolePrefix = "runtime artifact identifies error head frame"
		case "caller_frame":
			rolePrefix = "runtime artifact includes caller/context frame"
		}
		if observedLoc != "" {
			return fmt.Sprintf("%s %q at observed %s", rolePrefix, funcLabel, observedLoc), ""
		}
		return fmt.Sprintf("%s %q", rolePrefix, funcLabel), ""
	}
	if raw == "" {
		raw = strings.TrimSpace(seed.Func)
	}
	if raw == "" {
		return "", ""
	}
	location := ""
	switch {
	case strings.TrimSpace(seed.AnchoredFile) != "" && seed.AnchoredLine > 0:
		location = fmt.Sprintf("%s:%d", strings.TrimSpace(seed.AnchoredFile), seed.AnchoredLine)
	case strings.TrimSpace(seed.File) != "" && seed.Line > 0:
		location = fmt.Sprintf("%s:%d", strings.TrimSpace(seed.File), seed.Line)
	}
	if location != "" {
		return fmt.Sprintf("runtime observation %q aligns to %s", raw, location), location
	}
	return fmt.Sprintf("runtime observation %q", raw), ""
}

func externalObservationSupportDetail(seed ExternalObservationSeed, text string) string {
	var parts []string
	for _, raw := range []string{seed.Raw} {
		if detail := answerSupportEntryDetail(raw, text); detail != "" {
			parts = append(parts, detail)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

func compileCurrentCodePathSupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneCurrentCodePath,
		Title:         "Current grounded code path",
		AllowedBlocks: []string{"ordered_list", "diagram", "summary"},
		Guidance: "Use this lane for the principal ordered call / path chain. Keep each hop at " +
			"the abstraction literally supported by its own citation or grounded snippet. " +
			"These entries prove today's code structure, not necessarily that the older runtime artifact executed every downstream hop exactly as shown. " +
			"For drift-bounded runtime traces, prefer hops that align to the observed stack-frame transition itself; do not elevate ordinary intra-function helper calls into the principal path unless the observed artifact explicitly names that hop.",
	}
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if !rootCausePathItemEligible(plan, item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Detail:   callChainEvidenceSupportDetail(item, text),
			Location: supportEntryLocation(item),
		})
		if len(lane.Entries) >= 4 {
			break
		}
	}
	return lane
}

func compileNearestMechanismSupportLane(plan *AnswerSurfacePlan, strength rootCauseMechanismStrength) AnswerSupportLane {
	if strength != rootCauseMechanismStrong {
		return AnswerSupportLane{}
	}
	lane := AnswerSupportLane{
		Kind:          SupportLaneNearestMechanism,
		Title:         "Nearest grounded mechanism",
		AllowedBlocks: []string{"summary", "ordered_list"},
		Guidance: "Use this lane for the closest current-code guard / assignment / return / " +
			"definition that helps explain the failure path. Do not promote this lane into " +
			"caller-side provenance or old-build internals unless current citations explicitly prove it. " +
			"A current guard condition plus a later dereference proves the code contains both sites; by itself it does NOT prove the runtime artifact actually passed that guard and reached the dereference path.",
	}
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if !rootCauseMechanismItemEligible(plan, item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Detail:   callChainEvidenceSupportDetail(item, text),
			Location: supportEntryLocation(item),
		})
		if len(lane.Entries) >= 3 {
			break
		}
	}
	return lane
}

func compileUncertaintyBoundarySupportLane(plan *AnswerSurfacePlan, strength rootCauseMechanismStrength) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneUncertaintyBound,
		Title:         "Boundary / uncertainty disclosures",
		AllowedBlocks: []string{"caveat", "summary"},
		Guidance: "Use this lane for drift and proof-boundary caveats. It can narrow or hedge the " +
			"principal explanation, but it must not be turned into a speculative mechanism story. " +
			"Do not turn entries from this lane into principal ordered-list hops, diagram edges, or candidate nil-source / caller-provenance claims.",
	}
	if strength == rootCauseMechanismWeakGuardOnly {
		lane.Guidance += " When this is the strongest current-code mechanism support available, do not identify a specific variable, receiver field, or caller-provided value as the likely cause from this lane alone; state only that the exact internal trigger remains unrecovered in the current checkout."
	}
	for _, anchor := range plan.LogSourceDriftAnchors {
		file := strings.TrimSpace(anchor.File)
		if file == "" || anchor.ObservedLine <= 0 || anchor.AnchoredLine <= 0 {
			continue
		}
		funcLabel := strings.TrimSpace(firstNonEmptySurfaceString(anchor.Func, anchor.OriginalFunc))
		text := fmt.Sprintf("observed frame %s:%d", file, anchor.ObservedLine)
		if funcLabel != "" {
			text += fmt.Sprintf(" in %s", funcLabel)
		}
		text += fmt.Sprintf(" now maps to current grounded anchor %s:%d", file, anchor.AnchoredLine)
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Location: fmt.Sprintf("%s:%d", file, anchor.AnchoredLine),
		})
		if len(lane.Entries) >= 3 {
			break
		}
	}
	if strength == rootCauseMechanismWeakGuardOnly {
		if entry, ok := rootCauseWeakMechanismBoundaryEntry(plan); ok {
			lane.Entries = append(lane.Entries, entry)
		}
	}
	return lane
}

func rootCausePathItemEligible(plan *AnswerSurfacePlan, item EvidenceItem) bool {
	if driftBoundedIsCallItem(item) {
		return rootCauseObservedFrameCallEligible(plan, item)
	}
	// When no explicit call edge survives, a grounded definition is the
	// next-best path anchor; keep it in the path lane rather than forcing
	// the answer to jump straight into mechanism-only prose.
	return item.AnchorKind == AnchorDefinition
}

func rootCauseObservedFrameCallEligible(plan *AnswerSurfacePlan, item EvidenceItem) bool {
	subjectTail := normalizedSurfaceSymbolTail(item.Subject)
	objectTail := normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(item.Object, item.AnchorSymbol))
	if subjectTail == "" || objectTail == "" {
		return false
	}
	innerTail, outerTail := rootCauseObservedFrameTails(plan)
	switch {
	case outerTail != "" && innerTail != "":
		return subjectTail == outerTail && objectTail == innerTail
	case innerTail != "":
		// If we only recovered the innermost current anchor, keep the
		// incoming call into that frame, not arbitrary intra-function
		// calls that originate from it.
		return objectTail == innerTail
	default:
		return true
	}
}

func rootCauseObservedFrameTails(plan *AnswerSurfacePlan) (innerTail, outerTail string) {
	if plan == nil {
		return "", ""
	}
	if len(plan.LogObservedAnchors) > 0 {
		innerTail = normalizedSurfaceSymbolTail(plan.LogObservedAnchors[0].Func)
	}
	if innerTail == "" && len(plan.LogSourceDriftAnchors) > 0 {
		innerTail = normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(
			plan.LogSourceDriftAnchors[0].OriginalFunc,
			plan.LogSourceDriftAnchors[0].Func,
		))
	}
	if len(plan.LogObservedAnchors) > 1 {
		outerTail = normalizedSurfaceSymbolTail(plan.LogObservedAnchors[1].Func)
	}
	if outerTail == "" && len(plan.LogSourceDriftAnchors) > 1 {
		outerTail = normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(
			plan.LogSourceDriftAnchors[1].OriginalFunc,
			plan.LogSourceDriftAnchors[1].Func,
		))
	}
	return innerTail, outerTail
}

type rootCauseMechanismStrength int

const (
	rootCauseMechanismNone rootCauseMechanismStrength = iota
	rootCauseMechanismWeakGuardOnly
	rootCauseMechanismStrong
)

func rootCauseMechanismSupportStrength(plan *AnswerSurfacePlan) rootCauseMechanismStrength {
	if plan == nil {
		return rootCauseMechanismNone
	}
	count := 0
	hasNonGuard := false
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if !rootCauseMechanismItemEligible(plan, item) {
			continue
		}
		count++
		if item.AnchorKind != AnchorCondition {
			hasNonGuard = true
		}
	}
	switch {
	case hasNonGuard || count >= 2:
		return rootCauseMechanismStrong
	case count == 1:
		return rootCauseMechanismWeakGuardOnly
	default:
		return rootCauseMechanismNone
	}
}

func rootCauseWeakMechanismBoundaryEntry(plan *AnswerSurfacePlan) (AnswerSupportEntry, bool) {
	if plan == nil {
		return AnswerSupportEntry{}, false
	}
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if !rootCauseMechanismItemEligible(plan, item) || item.AnchorKind != AnchorCondition {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		return AnswerSupportEntry{
			Text:     "current grounded code exposes only a protective guard near the observed site; no additional grounded inner statement in the same path was recovered to prove a closer current crash mechanism",
			Location: supportEntryLocation(item),
		}, true
	}
	return AnswerSupportEntry{}, false
}

var nilGuardPathRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_\.]*)\s*(?:==|!=)\s*nil`)

func rootCauseMechanismItemEligible(plan *AnswerSurfacePlan, item EvidenceItem) bool {
	switch item.AnchorKind {
	case AnchorCondition:
		return true
	case AnchorReturn:
		// A return statement can expose a current fail-fast / error-return
		// path. For a CRASH-sourced root-cause trace (panic / exception /
		// sanitizer) it is not, by itself, proof of the runtime crash
		// instruction that actually executed — keeping return anchors out
		// of the principal mechanism lane prevents the finalizer from
		// turning "current checkout returns an error here" into "older
		// runtime panicked because this return path fired".
		//
		// For NON-crash root-cause questions ("Foo returns nil — why?",
		// "Why did config X get the default?") an early-return statement
		// frequently IS the mechanism the user is asking about. Excluding
		// returns unconditionally would hollow out the mechanism lane on
		// these questions and force the finalizer onto the weak-mechanism
		// fallback prose path. So the exclusion is gated on whether the
		// surface plan was actually sourced from a crash artifact.
		if plan.IsCrashSourcedRootCause() {
			return false
		}
		if rootCauseControlHeaderCompanion(item) {
			return false
		}
		if !rootCauseMechanismCompanionKindEligible(item) {
			return false
		}
		return !rootCauseGuardProtectedCompanion(plan, item)
	case AnchorAssignment:
		if rootCauseControlHeaderCompanion(item) {
			return false
		}
		if !rootCauseMechanismCompanionKindEligible(item) {
			return false
		}
		return !rootCauseGuardProtectedCompanion(plan, item)
	default:
		return false
	}
}

func rootCauseMechanismCompanionKindEligible(item EvidenceItem) bool {
	switch item.Kind {
	case EvidenceDirect, EvidenceMechanism, EvidenceConcrete, EvidenceDataflowPath:
		return true
	default:
		return false
	}
}

func rootCauseControlHeaderCompanion(item EvidenceItem) bool {
	snippet := strings.TrimSpace(item.Snippet)
	if snippet == "" {
		snippet = strings.TrimSpace(EvidenceStructuredSemanticLine(item, false))
	}
	if snippet == "" {
		return false
	}
	lower := strings.ToLower(snippet)
	return strings.HasPrefix(lower, "if ") ||
		strings.HasPrefix(lower, "switch ") ||
		strings.HasPrefix(lower, "for ") ||
		strings.HasPrefix(lower, "select ")
}

func rootCauseGuardProtectedCompanion(plan *AnswerSurfacePlan, candidate EvidenceItem) bool {
	if plan == nil || candidate.LineStart <= 0 {
		return false
	}
	if candidate.AnchorKind != AnchorAssignment && candidate.AnchorKind != AnchorReturn {
		return false
	}
	expr := strings.TrimSpace(candidate.Snippet)
	if expr == "" {
		expr = strings.TrimSpace(EvidenceStructuredSemanticLine(candidate, false))
	}
	if expr == "" {
		return false
	}
	targetFunc := normalizedSurfaceSymbolTail(firstNonEmptySurfaceString(
		candidate.OwnerSymbol,
		candidate.AnchorSymbol,
		candidate.Subject,
	))
	candidateFile := strings.TrimSpace(strings.ReplaceAll(candidate.Source, `\`, `/`))
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if item.AnchorKind != AnchorCondition || item.LineStart <= 0 || item.LineStart >= candidate.LineStart {
			continue
		}
		if strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)) != candidateFile {
			continue
		}
		if targetFunc != "" && !driftBoundedMentionsFunc(item, targetFunc) {
			continue
		}
		guardExpr := strings.TrimSpace(item.Condition)
		if guardExpr == "" {
			guardExpr = strings.TrimSpace(item.Snippet)
		}
		for _, path := range extractNilGuardedPaths(guardExpr) {
			if path != "" && strings.Contains(expr, path) {
				return true
			}
		}
	}
	return false
}

func extractNilGuardedPaths(cond string) []string {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return nil
	}
	matches := nilGuardPathRe.FindAllStringSubmatch(cond, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := strings.TrimSpace(match[1])
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func supportEntryLocation(item EvidenceItem) string {
	src := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if src == "" && item.Scope == ScopeNegative && item.NegativeQuery != nil {
		src = strings.TrimSpace(strings.ReplaceAll(item.NegativeQuery.File, `\`, `/`))
	}
	if src == "" && item.Scope == ScopeCrossfile && item.CrossfileQuery != nil && len(item.CrossfileQuery.Files) > 0 {
		files := append([]string(nil), item.CrossfileQuery.Files...)
		for i := range files {
			files[i] = strings.TrimSpace(strings.ReplaceAll(files[i], `\`, `/`))
		}
		sort.Strings(files)
		src = strings.Join(files, ",")
	}
	if src == "" {
		return ""
	}
	if item.LineStart > 0 {
		return fmt.Sprintf("%s:%d", src, item.LineStart)
	}
	return src
}

func stepSurfaceAnchorLocation(anchor StepSurfaceAnchor) string {
	file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
	if file == "" {
		return ""
	}
	if anchor.Line > 0 {
		return fmt.Sprintf("%s:%d", file, anchor.Line)
	}
	return file
}
