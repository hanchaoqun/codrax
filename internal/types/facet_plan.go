package types

import "strings"

// facet_plan.go — Phase 1 of the Semantic Surface Contract rollout
// (docs/design/semantic_surface_contract_phases.md §1).
//
// FacetCoverageContract describes the SET of answer facets the user's
// question requires to be covered, along with each facet's allowed
// ClaimForm whitelist. It is COMPILED deterministically by
// CompileFacetCoverage from existing typed analyzer outputs (Intent /
// Scenario / AnswerSubject / PredicateAxis / QuestionStructure /
// surface evidence). LLM never reads or emits FacetCoverageContract.
//
// Phase 1 contract: this layer is OBSERVATIONAL ONLY. Compiled plan
// lands on AnswerSurfacePlan.FacetCoverage and surfaces in trace
// debug logs for Phase 4+ validators to consume. NO gate, NO prompt
// effect, NO behaviour change in this Phase.
//
// ── Phase 0 trace data informs three design decisions ──
// (see docs/design/semantic_surface_contract_p0_trace.md)
//
//  1. Tolerate ClaimUnknown: 34% global unknown rate (concrete_values
//     producer dominates). HARD facets MUST not require ClaimForm-set
//     to exclude Unknown — they validate via direct
//     EvidenceItem.Source / AnchorSymbol match instead.
//
//  2. Question shape correlates with ClaimForm density: mechanism
//     questions have ~9% unknown, list_of_symbols / explanation
//     have ~95%. Facet plan must weight SourceCandidate-direct-match
//     equally with ClaimForm-aware match for low-ClaimForm shapes.
//
//  3. Five priority rules in ClaimFormOf all fire on real data;
//     projection matrix is stable and serves as Phase 1+ contract
//     base without adjustment.

// QuestionFamily is the deterministic question-shape grouping used
// to select a FacetCoverageContract template. Resolved by
// ResolveQuestionFamily from RequestModel — pure function, no LLM
// input.
//
// The 7 families are intentionally COARSE — the goal is to map the
// long tail of (Intent × Scenario × AnswerSubject × PredicateAxis)
// combinations onto a tractable plan space. Phase 0 trace data
// (s1a / s5a / m1a / s3a / logtri_go) maps cleanly to existing
// families; logtri_go is QFRootCauseTrace because the question
// asked "why did this panic" with an attached log artifact.
type QuestionFamily string

const (
	// QFRootCauseTrace covers root-cause + log/perf attached
	// questions ("why did X fail" / "trace the panic").
	QFRootCauseTrace QuestionFamily = "root_cause_trace"

	// QFConfigPrecedence covers config-trace questions where the
	// answer must show layered overrides ("how is X computed
	// across N config layers").
	QFConfigPrecedence QuestionFamily = "config_precedence"

	// QFRoleLookup covers "what is the X for Y" questions where
	// the answer is a specific named entity playing a role
	// (handler / config-key / function-by-purpose).
	QFRoleLookup QuestionFamily = "role_lookup"

	// QFCallChain covers structural-trace questions ("how does X
	// reach Y", "what is the dispatch path for Z"). Distinct from
	// QFRootCauseTrace by lack of failure framing.
	QFCallChain QuestionFamily = "call_chain"

	// QFEnumeration covers list / count / exhaustive-set questions
	// ("list all X" / "the 7 X" / "X for A and Y for B").
	QFEnumeration QuestionFamily = "enumeration"

	// QFArchitecture covers high-level explanation questions
	// ("how does the X subsystem work overall").
	QFArchitecture QuestionFamily = "architecture"

	// QFGeneric is the fallback when no specific family matches.
	// Compiled facet plan stays minimal (resolved-literal +
	// uncertainty-boundary only).
	QFGeneric QuestionFamily = "generic"
)

// AnswerFacetKind enumerates the semantic surfaces an answer may
// need to cover. NOT a 1:1 mirror of ClaimForm — facets are answer-
// shape requirements, ClaimForms are evidence capabilities.
//
// Phase 1 starts with the 12 facets the input design doc proposed;
// Phase 4+ may extend after trace data identifies new gaps.
type AnswerFacetKind string

const (
	// FacetObservedArtifactFact: "the log/perf trace observed X at
	// frame N". Acceptable forms: ClaimExternalObservation only.
	// Required when Origin includes Log or Perf.
	FacetObservedArtifactFact AnswerFacetKind = "observed_artifact_fact"

	// FacetCurrentCodePath: "the current code does X at file:line".
	// Acceptable forms: ClaimDefinitionFact, ClaimCallEdge,
	// ClaimGuardCondition, ClaimAssignmentFact, ClaimReturnFact.
	FacetCurrentCodePath AnswerFacetKind = "current_code_path"

	// FacetNearestMechanism: "the closest existing mechanism that
	// approximates the user's named X". Used for role-lookup
	// questions where the exact target is absent but a near-match
	// exists.
	FacetNearestMechanism AnswerFacetKind = "nearest_mechanism"

	// FacetUncertaintyBoundary: "what was searched, what remains
	// unverified". Required SOFT for any question with mixed
	// authority (some claims factual, some illustrative).
	FacetUncertaintyBoundary AnswerFacetKind = "uncertainty_boundary"

	// FacetConfigPrecedenceRole: "this X is the layer-N override of
	// Y". Acceptable forms: ClaimPrecedenceRole. Required HARD for
	// QFConfigPrecedence questions.
	FacetConfigPrecedenceRole AnswerFacetKind = "config_precedence_role"

	// FacetResolvedLiteralOrSymbol: "the exact answer is N" / "is
	// foo at file:line". Required HARD when QuestionStructure has
	// ExactResolution contract.
	FacetResolvedLiteralOrSymbol AnswerFacetKind = "resolved_literal_or_symbol"

	// FacetEnumerationItem: "one item in the user-asked enumeration".
	// Required HARD per item when EnumerationBoundary is set.
	FacetEnumerationItem AnswerFacetKind = "enumeration_item"

	// FacetBucketLabel: "this section answers the user-named bucket
	// X". Required HARD per bucket when Buckets[] is set.
	FacetBucketLabel AnswerFacetKind = "bucket_label"

	// FacetPrincipalPathEdge: "X calls Y" / "Y dispatches Z".
	// Acceptable forms: ClaimCallEdge.
	FacetPrincipalPathEdge AnswerFacetKind = "principal_path_edge"

	// FacetBranchGuard: "X runs only when condition C".
	// Acceptable forms: ClaimGuardCondition.
	FacetBranchGuard AnswerFacetKind = "branch_guard"

	// FacetComponentRelation: "X depends on Y" / "X imports Y".
	// Acceptable forms: ClaimImportEdge, ClaimCallEdge.
	FacetComponentRelation AnswerFacetKind = "component_relation"

	// FacetDiagramSpine: the structural backbone of a rendered
	// diagram (call graph / dispatch chain / config layers).
	// Required HARD when DiagramContract requires a diagram AND the
	// question is non-trivial enough that prose alone is unclear.
	FacetDiagramSpine AnswerFacetKind = "diagram_spine"
)

// FacetRequiredness classifies how strictly a facet must be
// covered. Phase 4 hard gates only fire on FacetHardRequired
// missing; FacetSoftRequired surfaces as soft drift signal;
// FacetOptional drives Phase 5 richness telemetry.
type FacetRequiredness string

const (
	FacetHardRequired FacetRequiredness = "hard"
	FacetSoftRequired FacetRequiredness = "soft"
	FacetOptional     FacetRequiredness = "optional"
)

// FacetRequirement describes one row in a FacetCoverageContract.
// AcceptableForms whitelists which ClaimForm values can validly
// support this facet; SourceCandidate carries EvidenceItem.IDs
// the planner believes could fill the slot (looser binding,
// resolved at validation time).
//
// Phase 0 trace finding: 34% of evidence items are ClaimUnknown
// (from concrete_values producer). FacetHardRequired entries
// MUST list SourceCandidate (direct EvidenceItem.ID match) so
// validation has a fallback that does not depend on ClaimForm
// being non-Unknown.
type FacetRequirement struct {
	Kind            AnswerFacetKind
	Required        FacetRequiredness
	AcceptableForms []ClaimForm
	SourceCandidate []string
}

// FacetCoverageContract is the compiled per-question facet plan.
// Lives on AnswerSurfacePlan.FacetCoverage. Phase 1: observation
// only; Phase 2 surfaces in finalizer prompt; Phase 4 drives gate
// validation.
type FacetCoverageContract struct {
	Family   QuestionFamily
	Required []FacetRequirement
	Optional []FacetRequirement
}

// ResolveQuestionFamily maps a RequestModel onto one of the seven
// families. Pure deterministic function over typed analyzer fields:
//   - Intent (primary signal)
//   - Scenario (refinement)
//   - AnswerSubject.Kind (role_lookup detection)
//   - QuestionStructure (count / completeness / buckets)
//   - LogBundle / PerfBundle presence (root_cause_trace gate)
//
// Priority order (first match wins):
//
//  1. LogTriage / PerfTrace attached + Intent in {RootCause, Trace}
//     → QFRootCauseTrace (logtri_go falls here)
//
//  2. Intent == ConfigQuery OR Scenario == ConfigTrace
//     → QFConfigPrecedence (s3a falls here)
//
//  3. AnswerSubject.Kind ∈ {SubjectFunctionName, SubjectHandlerRoute,
//     SubjectConfigKey, SubjectStructField, SubjectInterface}
//     AND QuestionStructure.HasAnyObligation()=false
//     → QFRoleLookup (typical "what's the X for Y" questions)
//
//  4. Intent == Trace AND no obligation
//     → QFCallChain (s1a falls here when 9-checks counted as trace)
//
//  5. QuestionStructure.HasAnyObligation()=true
//     OR Intent == Enumerate
//     → QFEnumeration (s5a / m1a fall here)
//
//  6. Intent == Explain AND Scenario == ArchitectureExplain
//     → QFArchitecture
//
//  7. fallthrough → QFGeneric
//
// Phase 0 trace data on s1a / s5a / m1a / s3a / logtri_go
// confirms each branch hits at least one case.
func ResolveQuestionFamily(rm RequestModel) QuestionFamily {
	hasLog := rm.LogTriage != nil
	hasPerf := rm.PerfTrace != nil

	// Rule 1: log/perf + cause/trace intent.
	if (hasLog || hasPerf) && (rm.Intent == IntentRootCause || rm.Intent == IntentTrace) {
		return QFRootCauseTrace
	}

	// Rule 2: config-shaped intent or scenario.
	if rm.Intent == IntentConfigQuery || rm.Scenario == ScenarioConfigTrace {
		return QFConfigPrecedence
	}

	view := rm.QuestionStructure()
	hasObligation := view.HasAnyObligation()

	// Rule 3: role-lookup detection (named-entity subject + no
	// enumeration obligation).
	if !hasObligation {
		switch rm.AnswerSubject.Kind {
		case SubjectFunctionName, SubjectHandlerRoute,
			SubjectConfigKey, SubjectStructField, SubjectInterface:
			return QFRoleLookup
		}
	}

	// Rule 4: trace intent without obligation.
	if rm.Intent == IntentTrace && !hasObligation {
		return QFCallChain
	}

	// Rule 5: enumeration / obligation-bearing.
	if hasObligation || rm.Intent == IntentEnumerate {
		return QFEnumeration
	}

	// Rule 6: architecture explain.
	if rm.Intent == IntentExplain && rm.Scenario == ScenarioArchitectureExplain {
		return QFArchitecture
	}

	return QFGeneric
}

// CompileFacetCoverage produces the FacetCoverageContract for a
// RequestModel + already-compiled surface evidence. Each family has
// a fixed template; the function fills SourceCandidate with
// EvidenceItem.IDs that match the facet's AcceptableForms.
//
// Phase 1 templates are intentionally MINIMAL — every Required
// facet must have at least one SourceCandidate (compiled from the
// surface evidence pool); otherwise it falls into Optional.
//
// This separation reflects the Phase 0 trace finding: concrete_values
// produces a lot of ClaimUnknown evidence. A facet should not be
// declared HARD-required just because the family template says so;
// it must also have grounded supporting evidence.
func CompileFacetCoverage(rm RequestModel, surface []EvidenceItem) *FacetCoverageContract {
	family := ResolveQuestionFamily(rm)
	template := familyTemplate(family, rm)
	if template == nil {
		return nil
	}

	// Bind SourceCandidate: walk surface evidence, match each
	// requirement's AcceptableForms, attach matching IDs.
	plan := &FacetCoverageContract{
		Family: family,
	}
	for _, req := range template {
		bound := bindSourceCandidates(req, surface)
		// Phase 1 fallback: if a HARD requirement has no candidate,
		// degrade to SOFT so we don't report false-fail in trace
		// observation. Phase 4 may revisit this rule with stricter
		// semantics once richer ClaimForm coverage lands.
		if bound.Required == FacetHardRequired && len(bound.SourceCandidate) == 0 {
			bound.Required = FacetSoftRequired
		}
		switch bound.Required {
		case FacetHardRequired, FacetSoftRequired:
			plan.Required = append(plan.Required, bound)
		case FacetOptional:
			plan.Optional = append(plan.Optional, bound)
		}
	}
	return plan
}

// bindSourceCandidates walks the surface evidence and attaches the
// IDs of items whose ClaimForm is in the requirement's
// AcceptableForms slice. Empty AcceptableForms means "any
// non-Unknown ClaimForm is acceptable" (lenient match for facets
// that just need ANY grounded evidence).
func bindSourceCandidates(req FacetRequirement, surface []EvidenceItem) FacetRequirement {
	out := req
	out.SourceCandidate = nil
	for _, item := range surface {
		if matches := claimFormMatches(req.AcceptableForms, ClaimFormOf(item)); matches {
			if id := strings.TrimSpace(item.ID); id != "" {
				out.SourceCandidate = append(out.SourceCandidate, id)
			}
		}
	}
	return out
}

// claimFormMatches reports whether the projected ClaimForm satisfies
// the AcceptableForms whitelist. Empty whitelist matches any
// non-Unknown form (Phase-1 lenient semantic — see CompileFacetCoverage
// docstring on Phase-0 ClaimUnknown tolerance).
func claimFormMatches(allowed []ClaimForm, projected ClaimForm) bool {
	if len(allowed) == 0 {
		return projected != ClaimUnknown
	}
	for _, c := range allowed {
		if c == projected {
			return true
		}
	}
	return false
}

// familyTemplate returns the template list of FacetRequirements for
// a given family. Templates are static; SourceCandidate binding
// happens in CompileFacetCoverage.
func familyTemplate(family QuestionFamily, rm RequestModel) []FacetRequirement {
	view := rm.QuestionStructure()
	common := commonFacets(view)
	switch family {
	case QFRootCauseTrace:
		return append([]FacetRequirement{
			{Kind: FacetObservedArtifactFact, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimExternalObservation}},
			{Kind: FacetCurrentCodePath, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge,
					ClaimGuardCondition, ClaimAssignmentFact, ClaimReturnFact}},
			{Kind: FacetNearestMechanism, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}},
			{Kind: FacetUncertaintyBoundary, Required: FacetSoftRequired},
			{Kind: FacetDiagramSpine, Required: FacetOptional,
				AcceptableForms: []ClaimForm{ClaimCallEdge, ClaimGuardCondition}},
		}, common...)
	case QFConfigPrecedence:
		return append([]FacetRequirement{
			{Kind: FacetConfigPrecedenceRole, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimPrecedenceRole}},
			{Kind: FacetResolvedLiteralOrSymbol, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimAssignmentFact,
					ClaimAbsenceFact}},
			{Kind: FacetUncertaintyBoundary, Required: FacetSoftRequired},
			{Kind: FacetDiagramSpine, Required: FacetOptional,
				AcceptableForms: []ClaimForm{ClaimPrecedenceRole}},
		}, common...)
	case QFRoleLookup:
		return append([]FacetRequirement{
			{Kind: FacetResolvedLiteralOrSymbol, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimAssignmentFact,
					ClaimReturnFact}},
			{Kind: FacetCurrentCodePath, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}},
			{Kind: FacetUncertaintyBoundary, Required: FacetOptional},
		}, common...)
	case QFCallChain:
		return append([]FacetRequirement{
			{Kind: FacetPrincipalPathEdge, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimCallEdge}},
			{Kind: FacetBranchGuard, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimGuardCondition}},
			{Kind: FacetCurrentCodePath, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact}},
			{Kind: FacetDiagramSpine, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimCallEdge, ClaimGuardCondition}},
		}, common...)
	case QFEnumeration:
		return append([]FacetRequirement{
			{Kind: FacetEnumerationItem, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimAssignmentFact,
					ClaimReturnFact, ClaimImportEdge}},
			{Kind: FacetUncertaintyBoundary, Required: FacetSoftRequired},
		}, common...)
	case QFArchitecture:
		return append([]FacetRequirement{
			{Kind: FacetCurrentCodePath, Required: FacetHardRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}},
			{Kind: FacetComponentRelation, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimImportEdge, ClaimCallEdge}},
			{Kind: FacetDiagramSpine, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimCallEdge, ClaimImportEdge}},
			{Kind: FacetUncertaintyBoundary, Required: FacetOptional},
		}, common...)
	case QFGeneric:
		return append([]FacetRequirement{
			{Kind: FacetCurrentCodePath, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}},
			{Kind: FacetUncertaintyBoundary, Required: FacetOptional},
		}, common...)
	}
	return nil
}

// commonFacets adds bucket / enumeration-item facets that EVERY
// family inherits when QuestionStructure declares the obligation.
// These are conditionally injected based on the user's structural
// asks rather than being family-specific.
func commonFacets(view QuestionStructureView) []FacetRequirement {
	var out []FacetRequirement
	if view.EnumerationBoundary != nil && view.EnumerationBoundary.DeclaredCount > 0 {
		// One conceptual facet for the count-bounded set; bound to
		// principal-eligible forms.
		out = append(out, FacetRequirement{
			Kind:     FacetEnumerationItem,
			Required: FacetHardRequired,
			AcceptableForms: []ClaimForm{
				ClaimDefinitionFact, ClaimAssignmentFact, ClaimReturnFact,
				ClaimImportEdge, ClaimCallEdge,
			},
		})
	}
	if len(view.Buckets) >= 2 {
		out = append(out, FacetRequirement{
			Kind:     FacetBucketLabel,
			Required: FacetHardRequired,
		})
	}
	return out
}
