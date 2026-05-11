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
const facetSupportEntryLimitDefault = 18
const facetSupportEntryLimitEnumerationMax = 64
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
