package types

import (
	"fmt"
	"regexp"
	"strings"
)

// compileRootCauseSupportPlan is the family-specific compiler for
// QFRootCauseTrace. It threads observation / current-code-path /
// nearest-mechanism / uncertainty lanes through a typed
// rootCauseMechanismStrength dial so weak mechanism evidence promotes
// the boundary lane and full mechanism evidence promotes the
// dedicated mechanism lane. Returns nil when no lane has entries so
// the renderer can skip the section entirely.
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
		lane.Entries = append(lane.Entries,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
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
		lane.Entries = append(lane.Entries,
			answerSupportEntryForEvidence(item, text, callChainEvidenceSupportDetail(item, text)))
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
