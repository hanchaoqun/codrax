package types

import (
	"fmt"
	"sort"
	"strings"
)

// AnswerSupportPlan is the typed support-lane contract between the
// compiled answer surface and the finalizer prompt. Unlike free-form
// closure prose, support lanes describe what kind of user-visible
// claims are safe to build and which grounded evidence entries belong
// to each lane.
type AnswerSupportPlan struct {
	Family              QuestionFamily
	ChangeImpactProfile *ChangeImpactProfile
	// PrincipalMemberCoverage tells downstream gates whether entries in
	// the principal evidence lane are a hard member slate or enrichment
	// anchors for a broader prose/table answer. The zero value preserves
	// the historical QFEnumeration behavior for hand-built plans; compiled
	// plans set it explicitly from typed request structure.
	PrincipalMemberCoverage PrincipalMemberCoveragePolicy
	Lanes                   []AnswerSupportLane
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

type PrincipalMemberCoveragePolicy string

const (
	PrincipalMemberCoveragePolicyDefault        PrincipalMemberCoveragePolicy = ""
	PrincipalMemberCoveragePolicyRequired       PrincipalMemberCoveragePolicy = "required"
	PrincipalMemberCoveragePolicyEnrichmentOnly PrincipalMemberCoveragePolicy = "enrichment_only"
)

func (p PrincipalMemberCoveragePolicy) RequiresMemberCoverage() bool {
	return p != PrincipalMemberCoveragePolicyEnrichmentOnly
}

const callChainSupportEntryLimit = 24
const facetSupportEntryLimitDefault = 18
const facetUncertaintySupportEntryLimit = 4

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

	// EquivalentLocations holds additional file:line anchors that
	// describe the same principal support entry. A common cross-language
	// shape is "member label + implementation/proof support_ref" plus a
	// separate typed definition evidence item for that same member. Keeping
	// both anchors on one entry prevents downstream citation repair from
	// treating declaration and implementation evidence as competing members.
	EquivalentLocations []string

	// Typed evidence projection fields are copied from the source
	// EvidenceItem when the entry originates from structured
	// evidence. They let downstream validators reason about the
	// evidence member itself instead of parsing the human-readable
	// Text / Detail strings.
	EvidenceID    string
	ClaimForm     ClaimForm
	LabelSurface  ClaimLabelSurfaceKind
	Subject       string
	Object        string
	AnchorSymbol  string
	OwnerSymbol   string
	SurfaceTerms  []string
	Source        string
	LineStart     int
	LineEnd       int
	AnchorKind    AnchorKind
	MemberSurface AnswerPrincipalMemberSurface
	Producer      string
	GroundingTier GroundingTier
}

// BuildAnswerSupportPlanForAgentContext compiles the current typed
// support lanes for the active family. Returning nil means the family
// currently uses no additional support-lane contract beyond the base
// AnswerSurfacePlan / AnswerSemanticView.
func BuildAnswerSupportPlanForAgentContext(ctx *AgentContext) *AnswerSupportPlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	if cached := ctx.cachedAnswerSupportPlan(); cached != nil {
		return cached
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
			out = augmentCurrentStatusVerdictLane(out, view.CurrentStatusDiagnostic)
			ctx.storeAnswerSupportPlan(out)
			return cloneAnswerSupportPlan(out)
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		out := augmentCurrentStatusVerdictLane(
			buildAnswerSupportPlanForFamily(QFRootCauseTrace, ctx.AnalysisIR.RequestModel, plan),
			currentStatusDiagnosticContractFromIR(ctx.AnalysisIR),
		)
		ctx.storeAnswerSupportPlan(out)
		return cloneAnswerSupportPlan(out)
	}
	out := augmentCurrentStatusVerdictLane(
		BuildAnswerSupportPlan(ctx.AnalysisIR.RequestModel, plan),
		currentStatusDiagnosticContractFromIR(ctx.AnalysisIR),
	)
	ctx.storeAnswerSupportPlan(out)
	return cloneAnswerSupportPlan(out)
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

// AnswerSupportPlanPrincipalSurfaceCounts counts model-authored
// principal entries by answer-visible member surface. It gives
// extractor/tool validators one shared typed signal for whether an
// enumeration can be rendered from support lanes without synthesising an
// AnswerSymbol slate. The inputs are already structured support-lane
// entries; no request-text keywords or search scores participate.
func AnswerSupportPlanPrincipalSurfaceCounts(plan *AnswerSupportPlan) (nonSymbol int, symbolLike int) {
	if plan == nil {
		return 0, 0
	}
	for _, lane := range plan.Lanes {
		if lane.Kind != SupportLanePrincipalEvidence {
			continue
		}
		for _, entry := range lane.Entries {
			if !answerSupportEntryIsModelAuthoredPrincipal(entry) {
				continue
			}
			surface := entry.MemberSurface
			if surface == PrincipalMemberSurfaceUnknown {
				if entry.ClaimForm.UsesNonSymbolLabelSurface() {
					surface = PrincipalMemberSurfaceDisplayLabel
				} else if entry.ClaimForm.LabelSurfaceKind() == ClaimLabelSurfaceSymbolLike {
					surface = PrincipalMemberSurfaceSymbolLike
				}
			}
			if surface.IsNonSymbol() {
				nonSymbol++
				continue
			}
			if surface.IsSymbolLike() {
				symbolLike++
			}
		}
	}
	return nonSymbol, symbolLike
}

func answerSupportEntryIsModelAuthoredPrincipal(entry AnswerSupportEntry) bool {
	producer := strings.TrimSpace(entry.Producer)
	if producer == "explorer.emit_investigation_complete.aggregate_facts" {
		return true
	}
	if producer == "explorer.emit_evidence" {
		return true
	}
	return producer != "" && strings.Contains(producer, "emit_")
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
	if cached := bus.cachedAnswerSupportPlan(); cached != nil {
		return cached
	}
	plan := BuildAnswerSurfacePlanForBusContext(bus)
	if plan == nil {
		return nil
	}
	view := BuildAnswerSemanticViewForBusContext(bus)
	if view != nil {
		if out := buildAnswerSupportPlanForFamily(view.Family, bus.AnalysisIR.RequestModel, plan); out != nil {
			out = augmentCurrentStatusVerdictLane(out, view.CurrentStatusDiagnostic)
			bus.storeAnswerSupportPlan(out)
			return cloneAnswerSupportPlan(out)
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		out := augmentCurrentStatusVerdictLane(
			buildAnswerSupportPlanForFamily(QFRootCauseTrace, bus.AnalysisIR.RequestModel, plan),
			currentStatusDiagnosticContractFromIR(bus.AnalysisIR),
		)
		bus.storeAnswerSupportPlan(out)
		return cloneAnswerSupportPlan(out)
	}
	out := augmentCurrentStatusVerdictLane(
		BuildAnswerSupportPlan(bus.AnalysisIR.RequestModel, plan),
		currentStatusDiagnosticContractFromIR(bus.AnalysisIR),
	)
	bus.storeAnswerSupportPlan(out)
	return cloneAnswerSupportPlan(out)
}

// AnswerRequiresObservedArtifactCarrier is the shared typed predicate for the
// observed-artifact answer lane. Emit-time normalizers and post-emit contract
// checks both use this helper so the system does not drift into two subtly
// different definitions of when runtime/log/trace observations must be declared
// in the AnswerDocument.
func AnswerRequiresObservedArtifactCarrier(bus *BusContext) bool {
	if bus == nil {
		return false
	}
	if plan := BuildAnswerSupportPlanForBusContext(bus); plan != nil {
		for _, lane := range plan.Lanes {
			if lane.Kind == SupportLaneObservedArtifact && len(lane.Entries) > 0 {
				return true
			}
		}
	}
	if view := BuildAnswerSemanticViewForBusContext(bus); view != nil && view.FacetCoverage != nil {
		for _, req := range view.FacetCoverage.Required {
			if req.Kind == FacetObservedArtifactFact && req.IsPromoted() {
				return true
			}
		}
	}
	return false
}

func currentStatusDiagnosticContractFromIR(ir *AnalysisIR) *CurrentStatusDiagnosticContract {
	if ir == nil || ir.AnswerContract.CurrentStatusDiagnostic == nil ||
		!ir.AnswerContract.CurrentStatusDiagnostic.Required {
		return nil
	}
	return ir.AnswerContract.CurrentStatusDiagnostic
}

func buildAnswerSupportPlanForFamily(family QuestionFamily, rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	withProfile := func(out *AnswerSupportPlan) *AnswerSupportPlan {
		if out != nil {
			out.ChangeImpactProfile = rm.ChangeImpactProfile
		}
		return out
	}
	switch family {
	case QFRootCauseTrace:
		return withProfile(compileRootCauseSupportPlan(rm, plan))
	case QFCallChain:
		return withProfile(compileCallChainSupportPlan(rm, plan))
	case QFConfigPrecedence, QFRoleLookup, QFEnumeration, QFArchitecture, QFComparison, QFGeneric:
		return withProfile(compileFacetEvidenceSupportPlan(family, rm, plan))
	default:
		return nil
	}
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

func answerSupportEntryForEvidence(item EvidenceItem, text, detail string) AnswerSupportEntry {
	form := ClaimFormOf(item)
	entry := AnswerSupportEntry{
		Text:          strings.TrimSpace(text),
		Detail:        strings.TrimSpace(detail),
		Location:      supportEntryLocation(item),
		EvidenceID:    strings.TrimSpace(item.ID),
		ClaimForm:     form,
		LabelSurface:  form.LabelSurfaceKind(),
		Subject:       strings.TrimSpace(item.Subject),
		Object:        strings.TrimSpace(item.Object),
		AnchorSymbol:  strings.TrimSpace(item.AnchorSymbol),
		OwnerSymbol:   strings.TrimSpace(item.OwnerSymbol),
		Source:        strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`)),
		LineStart:     item.LineStart,
		LineEnd:       item.LineEnd,
		AnchorKind:    item.AnchorKind,
		MemberSurface: PrincipalMemberSurfaceForEvidenceSet(item, nil),
		Producer:      strings.TrimSpace(item.Producer),
		GroundingTier: item.GroundingTier,
	}
	if entry.Source != "" && entry.LineEnd > entry.LineStart {
		entry.EquivalentLocations = appendAnswerSupportEquivalentLocation(
			entry.EquivalentLocations,
			fmt.Sprintf("%s:%d", entry.Source, entry.LineEnd),
		)
	}
	for _, term := range item.SurfaceTerms {
		term = strings.TrimSpace(term)
		if term != "" {
			entry.SurfaceTerms = append(entry.SurfaceTerms, term)
		}
	}
	return entry
}

func appendAnswerSupportEquivalentLocation(in []string, location string) []string {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return in
	}
	key := normalizeAnswerSupportLocation(location)
	for _, existing := range in {
		if normalizeAnswerSupportLocation(existing) == key {
			return in
		}
	}
	return append(in, location)
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
