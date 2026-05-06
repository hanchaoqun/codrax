package types

import (
	"fmt"
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
	SupportLaneObservedArtifact AnswerSupportLaneKind = "observed_artifact"
	SupportLaneCurrentCodePath  AnswerSupportLaneKind = "current_code_path"
	SupportLaneNearestMechanism AnswerSupportLaneKind = "nearest_mechanism"
	SupportLaneUncertaintyBound AnswerSupportLaneKind = "uncertainty_boundary"
)

type AnswerSupportLane struct {
	Kind     AnswerSupportLaneKind
	Title    string
	Guidance string
	Entries  []AnswerSupportEntry
}

type AnswerSupportEntry struct {
	Text     string
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
	view := BuildAnswerSemanticViewForAgentContext(ctx)
	if view != nil {
		if out := buildAnswerSupportPlanForFamily(view.Family, plan); out != nil {
			return out
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		return buildAnswerSupportPlanForFamily(QFRootCauseTrace, plan)
	}
	return BuildAnswerSupportPlan(ctx.AnalysisIR.RequestModel, plan)
}

// BuildAnswerSupportPlan compiles a family-aware support-lane view from
// the resolved RequestModel and current AnswerSurfacePlan. Phase 1 only
// materializes QFRootCauseTrace, where we need a hard boundary between
// observed artifact facts, current code path facts, current mechanism
// facts, and uncertainty disclosures.
func BuildAnswerSupportPlan(rm RequestModel, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	return buildAnswerSupportPlanForFamily(ResolveQuestionFamily(rm), plan)
}

func buildAnswerSupportPlanForFamily(family QuestionFamily, plan *AnswerSurfacePlan) *AnswerSupportPlan {
	switch family {
	case QFRootCauseTrace:
		return compileRootCauseSupportPlan(plan)
	default:
		return nil
	}
}

func compileRootCauseSupportPlan(plan *AnswerSurfacePlan) *AnswerSupportPlan {
	if plan == nil {
		return nil
	}
	out := &AnswerSupportPlan{Family: QFRootCauseTrace}

	if lane := compileObservedArtifactSupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileCurrentCodePathSupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileNearestMechanismSupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if lane := compileUncertaintyBoundarySupportLane(plan); len(lane.Entries) > 0 {
		out.Lanes = append(out.Lanes, lane)
	}
	if len(out.Lanes) == 0 {
		return nil
	}
	return out
}

func compileObservedArtifactSupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:  SupportLaneObservedArtifact,
		Title: "Observed artifact facts",
		Guidance: "Use this lane only for facts that came from the attached runtime artifact " +
			"(log / perf trace / external observation). These facts can explain what was " +
			"observed, but they are not by themselves current-code mechanism proofs.",
	}
	for _, seed := range plan.ExternalObservationSeeds {
		text, location := renderExternalObservationSupportEntry(seed)
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Location: location,
		})
		if len(lane.Entries) >= 6 {
			break
		}
	}
	return lane
}

func renderExternalObservationSupportEntry(seed ExternalObservationSeed) (string, string) {
	raw := strings.TrimSpace(seed.Raw)
	switch strings.TrimSpace(seed.Kind) {
	case "error_type":
		if raw == "" {
			return "", ""
		}
		return fmt.Sprintf("structured runtime error type %q", raw), ""
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

func compileCurrentCodePathSupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:  SupportLaneCurrentCodePath,
		Title: "Current grounded code path",
		Guidance: "Use this lane for the principal ordered call / path chain. Keep each hop at " +
			"the abstraction literally supported by its own citation or grounded snippet.",
	}
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if !rootCausePathItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Location: supportEntryLocation(item),
		})
		if len(lane.Entries) >= 4 {
			break
		}
	}
	return lane
}

func compileNearestMechanismSupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:  SupportLaneNearestMechanism,
		Title: "Nearest grounded mechanism",
		Guidance: "Use this lane for the closest current-code guard / assignment / return / " +
			"definition that helps explain the failure path. Do not promote this lane into " +
			"caller-side provenance or old-build internals unless current citations explicitly prove it.",
	}
	for _, item := range DriftBoundedRenderableSurfaceItems(plan.DriftBoundedSurfaceItems) {
		if !rootCauseMechanismItemEligible(item) {
			continue
		}
		text := strings.TrimSpace(EvidenceAuthoritativeSurfaceText(item, false))
		if text == "" {
			continue
		}
		lane.Entries = append(lane.Entries, AnswerSupportEntry{
			Text:     text,
			Location: supportEntryLocation(item),
		})
		if len(lane.Entries) >= 3 {
			break
		}
	}
	return lane
}

func compileUncertaintyBoundarySupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:  SupportLaneUncertaintyBound,
		Title: "Boundary / uncertainty disclosures",
		Guidance: "Use this lane for drift and proof-boundary caveats. It can narrow or hedge the " +
			"principal explanation, but it must not be turned into a speculative mechanism story.",
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
	return lane
}

func rootCausePathItemEligible(item EvidenceItem) bool {
	if driftBoundedIsCallItem(item) {
		return true
	}
	// When no explicit call edge survives, a grounded definition is the
	// next-best path anchor; keep it in the path lane rather than forcing
	// the answer to jump straight into mechanism-only prose.
	return item.AnchorKind == AnchorDefinition
}

func rootCauseMechanismItemEligible(item EvidenceItem) bool {
	switch item.AnchorKind {
	case AnchorCondition, AnchorAssignment, AnchorReturn:
		return true
	default:
		return false
	}
}

func supportEntryLocation(item EvidenceItem) string {
	src := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if src == "" {
		return ""
	}
	if item.LineStart > 0 {
		return fmt.Sprintf("%s:%d", src, item.LineStart)
	}
	return src
}
