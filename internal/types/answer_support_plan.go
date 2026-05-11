package types

import (
	"fmt"
	"regexp"
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
	SupportLaneCurrentVerdict   AnswerSupportLaneKind = "current_status_verdict"
)

type AnswerSupportLane struct {
	Kind          AnswerSupportLaneKind
	Title         string
	Guidance      string
	AllowedBlocks []string
	Entries       []AnswerSupportEntry
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
			return augmentCurrentStatusVerdictLane(out, view.CurrentStatusDiagnostic)
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		return augmentCurrentStatusVerdictLane(
			buildAnswerSupportPlanForFamily(QFRootCauseTrace, plan),
			currentStatusDiagnosticContractFromIR(ctx.AnalysisIR),
		)
	}
	return augmentCurrentStatusVerdictLane(
		BuildAnswerSupportPlan(ctx.AnalysisIR.RequestModel, plan),
		currentStatusDiagnosticContractFromIR(ctx.AnalysisIR),
	)
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
		if out := buildAnswerSupportPlanForFamily(view.Family, plan); out != nil {
			return augmentCurrentStatusVerdictLane(out, view.CurrentStatusDiagnostic)
		}
	}
	if plan.SummarySurfaceMode == AnswerSummarySurfaceDriftBoundedRootCause ||
		len(plan.LogObservedAnchors) > 0 ||
		len(plan.LogSourceDriftAnchors) > 0 {
		return augmentCurrentStatusVerdictLane(
			buildAnswerSupportPlanForFamily(QFRootCauseTrace, plan),
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
	mechanismStrength := rootCauseMechanismSupportStrength(plan)
	out := &AnswerSupportPlan{Family: QFRootCauseTrace}

	if lane := compileObservedArtifactSupportLane(plan); len(lane.Entries) > 0 {
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
				Location: location,
			})
			if len(lane.Entries) >= 8 {
				return lane
			}
		}
	}
	return lane
}

func compileObservedArtifactSupportLane(plan *AnswerSurfacePlan) AnswerSupportLane {
	lane := AnswerSupportLane{
		Kind:          SupportLaneObservedArtifact,
		Title:         "Observed artifact facts",
		AllowedBlocks: []string{"summary", "caveat"},
		Guidance: "Use this lane only for facts that came from the attached runtime artifact " +
			"(log / perf trace / external observation). The system populates this lane " +
			"from items the typed evidence projector tagged Origin=log or Origin=perf, " +
			"so every entry here is structurally an observation, not current-code " +
			"mechanism. These facts can explain what was observed (which frame fired, " +
			"which signal triggered) but they do not prove caller-side provenance, " +
			"source-parameter mapping, or exact downstream branch execution unless " +
			"a separately-cited current-code line establishes that mapping.",
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
		if observedLoc != "" {
			return fmt.Sprintf("runtime artifact includes stack frame %q at observed %s", funcLabel, observedLoc), ""
		}
		return fmt.Sprintf("runtime artifact includes stack frame %q", funcLabel), ""
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
	if src == "" {
		return ""
	}
	if item.LineStart > 0 {
		return fmt.Sprintf("%s:%d", src, item.LineStart)
	}
	return src
}
