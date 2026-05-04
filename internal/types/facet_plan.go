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

	// QFComparison covers questions that PARTITION the answer
	// across 2+ user-named buckets ("compare A vs B", "X for A
	// and Y for B", "differences between A and B"). Triggered
	// when QuestionStructure.Buckets has 2+ entries — the
	// resolver promotes the question out of QFCallChain /
	// QFEnumeration / QFGeneric (where the bucket partition
	// would be lost) into a dedicated family that scaffolds one
	// principal section block per bucket, preserving the
	// user's mental partition end-to-end.
	//
	// Compile template (compile_comparison.go): one
	// BlockSummary (lead-in describing what's being compared),
	// 2+ BlockSection (one per bucket — verbatim Label as Title),
	// optional BlockTable (side-by-side rendering when the
	// comparison is multi-axis). FacetCoverage Required uses
	// FacetBucketLabel HARD per bucket so the renderer / oracle
	// can verify each Label appears verbatim in some block.
	QFComparison QuestionFamily = "comparison"

	// QFGeneric is the fallback when no specific family matches.
	// Compiled facet plan stays minimal (resolved-literal +
	// uncertainty-boundary only).
	QFGeneric QuestionFamily = "generic"
)

// AllQuestionFamilies returns every declared QuestionFamily in a
// stable enumeration order. Mirrors the AllAnchorKinds /
// AllViolationKinds / AllAnswerBlockKinds pattern. Used by:
//   - structural tests (e.g. TestAllQuestionFamiliesHaveCompiler in
//     internal/types/answer_semantic_view_test.go) to enforce that
//     every family has a compile entry-point in
//     answer_semantic_view_compile.go's switch.
//   - downstream telemetry that wants to iterate the canonical
//     family set without re-listing it.
func AllQuestionFamilies() []QuestionFamily {
	return []QuestionFamily{
		QFRootCauseTrace,
		QFConfigPrecedence,
		QFRoleLookup,
		QFCallChain,
		QFEnumeration,
		QFArchitecture,
		QFComparison,
		QFGeneric,
	}
}

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

// RichnessTier (Phase 5 of Semantic Surface Contract, 2026-05-02)
// names how the facet contributes to answer "richness" — the
// difference between a technically-correct answer and one that
// surfaces useful supplemental context. The tier is a 1:1 derived
// view of FacetRequiredness; declared separately so Phase 5
// telemetry-only paths can read tier semantics without coupling to
// the gate-classification semantics.
//
//   - TierEssential   == FacetHardRequired: gate-required content
//   - TierExpected    == FacetSoftRequired: soft-drift content,
//     skill prompt encourages but does not gate
//   - TierEnrichment  == FacetOptional: richness signal — when
//     LLM omits, Phase 5 records SOFT ViolRichnessRegression for
//     end-of-Run telemetry. NEVER promotes to STRICT.
type RichnessTier string

const (
	TierEssential  RichnessTier = "essential"
	TierExpected   RichnessTier = "expected"
	TierEnrichment RichnessTier = "enrichment"
)

// TierFromRequiredness derives the canonical Tier from a
// FacetRequiredness value. Pure 1:1 projection — used by the
// CompileFacetCoverage compiler so every FacetRequirement carries
// both fields after compile.
func TierFromRequiredness(r FacetRequiredness) RichnessTier {
	switch r {
	case FacetHardRequired:
		return TierEssential
	case FacetSoftRequired:
		return TierExpected
	case FacetOptional:
		return TierEnrichment
	}
	return TierEnrichment
}

// FacetPromotionPolicy (G6 post_v2_runtime_gap_remediation, 2026-05-04)
// classifies how a facet's coverage demand is enforced when typed
// evidence is available to support it. Phase 5-E1 introduced the
// "evidence-sufficient skip" gate: TierExpected facets without
// SourceCandidate were skipped to avoid forcing the LLM to invent
// unsupported claims. G6 generalises that into an explicit
// promotion policy so consumers (validator severity, reviewer,
// accept policy) can read a single typed enum instead of replicating
// the (Tier, len(SourceCandidate)) joint logic at every site.
//
// R3 invariant: every promotion decision derives from the typed
// (PromotionPolicy, MinEvidenceForPromotion, len(SourceCandidate))
// triple — no heuristic, no similarity, no frequency.
//
// CompileFacetCoverage maps Requiredness → PromotionPolicy 1:1:
//
//	FacetHardRequired → PromotionAlwaysHard
//	FacetSoftRequired → PromotionWhenEvidenceSufficient
//	FacetOptional     → PromotionAdvisoryOnly
type FacetPromotionPolicy string

const (
	// PromotionAlwaysHard names a facet whose coverage demand is
	// hard regardless of evidence (analyzer template pinned it
	// essential; the answer must surface it). Mirrors TierEssential.
	PromotionAlwaysHard FacetPromotionPolicy = "always_hard"

	// PromotionWhenEvidenceSufficient names a facet that promotes to
	// HARD when len(SourceCandidate) >= MinEvidenceForPromotion. When
	// the evidence pool has nothing typed to surface, the demand is
	// SOFT (advisory) so the LLM is not forced to invent. Mirrors
	// TierExpected.
	PromotionWhenEvidenceSufficient FacetPromotionPolicy = "when_evidence_sufficient"

	// PromotionAdvisoryOnly names a facet that NEVER promotes — its
	// signal is permanently SOFT (telemetry surface for
	// validateRichnessRegression). Mirrors TierEnrichment.
	PromotionAdvisoryOnly FacetPromotionPolicy = "advisory_only"
)

// IsValid reports whether p is a known FacetPromotionPolicy.
func (p FacetPromotionPolicy) IsValid() bool {
	switch p {
	case PromotionAlwaysHard, PromotionWhenEvidenceSufficient, PromotionAdvisoryOnly:
		return true
	}
	return false
}

// PromotionPolicyFromRequiredness derives the canonical promotion
// policy from a FacetRequiredness value. Pure 1:1 projection,
// mirrors TierFromRequiredness — keeps Tier and PromotionPolicy
// in lock-step.
func PromotionPolicyFromRequiredness(r FacetRequiredness) FacetPromotionPolicy {
	switch r {
	case FacetHardRequired:
		return PromotionAlwaysHard
	case FacetSoftRequired:
		return PromotionWhenEvidenceSufficient
	case FacetOptional:
		return PromotionAdvisoryOnly
	}
	return PromotionAdvisoryOnly
}

// DefaultMinEvidenceForPromotion is the minimum SourceCandidate
// count that triggers PromotionWhenEvidenceSufficient → HARD.
// 1 is the conservative default: any single typed evidence row
// supporting the facet promotes the demand. Operators (or future
// per-facet overrides) can raise the bar via MinEvidenceForPromotion.
const DefaultMinEvidenceForPromotion = 1

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
//
// Phase 5 (2026-05-02): Tier is the derived richness classification
// (1:1 from Required). Filled by CompileFacetCoverage so consumers
// reading Tier do not need to call TierFromRequiredness inline.
// Empty Tier on a hand-constructed FacetRequirement decodes via
// EffectiveTier() so back-compat callers stay working.
//
// SourceCandidate provenance invariant (Phase 5-E0, 2026-05-04):
// SourceCandidate entries MUST be typed EvidenceItem.ID values. The
// only writer is bindSourceCandidates, which filters surface
// evidence through (a) claimFormMatches over the ClaimForm typed
// enum and (b) optional LogPerfSubKind typed enum equality. NO
// similarity scoring, NO frequency weighting, NO LLM-authored ID
// strings, NO heuristic boosts. EvidenceItem.ID itself is
// StableEvidenceID(item) — an FNV hash over typed fields (Kind /
// Scope / Origin / Authority / scope-specific typed fields). This
// invariant is what lets validators consume len(SourceCandidate) >
// 0 (or specific membership) as a HARD gate signal without
// violating R3 (precise signals → hard gates;noisy signals → soft
// guidance only). Adding a non-typed source — frequency, ranker
// score, similarity match — would corrupt the gate; route those
// through soft guidance instead.
type FacetRequirement struct {
	Kind            AnswerFacetKind
	Required        FacetRequiredness
	AcceptableForms []ClaimForm
	SourceCandidate []string
	Tier            RichnessTier

	// RequiredSubKind (Phase 4 extension, 2026-05-02): when set,
	// CompileFacetCoverage's candidate-binding step ALSO requires
	// the evidence's LogPerfSubKind to equal this value. Used by
	// QFRootCauseTrace and sibling families to refine the generic
	// FacetObservedArtifactFact facet into a sub-kind-specific
	// requirement (panic-only, oom-only, etc.) when the attached
	// log/perf bundle declares the corresponding severity signal.
	//
	// Empty = no sub-kind constraint (the existing AcceptableForms
	// + ClaimFormOf gate is the only filter). When set, the
	// binder narrows the SourceCandidate set to evidence whose
	// LogPerfSubKind == this value, in addition to passing the
	// AcceptableForms whitelist.
	RequiredSubKind LogPerfSubKind

	// PromotionPolicy + MinEvidenceForPromotion (G6
	// post_v2_runtime_gap_remediation, 2026-05-04): explicit promotion
	// classification for this facet. PromotionPolicy is filled by
	// CompileFacetCoverage 1:1 from Requiredness; MinEvidenceForPromotion
	// defaults to DefaultMinEvidenceForPromotion when zero. Together
	// they let validators / reviewers read a single typed signal
	// instead of replicating (Tier, len(SourceCandidate)) joint
	// logic at every consumer.
	PromotionPolicy         FacetPromotionPolicy
	MinEvidenceForPromotion int
}

// EffectiveTier returns Tier when set, otherwise derives from
// Required. Back-compat-safe accessor for any FacetRequirement
// that bypassed CompileFacetCoverage.
func (r FacetRequirement) EffectiveTier() RichnessTier {
	if r.Tier != "" {
		return r.Tier
	}
	return TierFromRequiredness(r.Required)
}

// EffectivePromotionPolicy returns PromotionPolicy when set,
// otherwise derives from Required. Back-compat accessor for any
// FacetRequirement that bypassed CompileFacetCoverage.
func (r FacetRequirement) EffectivePromotionPolicy() FacetPromotionPolicy {
	if r.PromotionPolicy != "" {
		return r.PromotionPolicy
	}
	return PromotionPolicyFromRequiredness(r.Required)
}

// EffectiveMinEvidenceForPromotion returns MinEvidenceForPromotion
// when > 0, otherwise DefaultMinEvidenceForPromotion. Zero is
// indistinguishable from "field not initialised", so the
// effective getter applies the default.
func (r FacetRequirement) EffectiveMinEvidenceForPromotion() int {
	if r.MinEvidenceForPromotion > 0 {
		return r.MinEvidenceForPromotion
	}
	return DefaultMinEvidenceForPromotion
}

// IsPromoted reports whether this facet's coverage demand is
// currently HARD given the typed evidence available. Pure typed-
// signal computation (R3 red line):
//
//   - PromotionAlwaysHard       → always HARD (regardless of evidence)
//   - PromotionWhenEvidenceSufficient → HARD iff
//     len(SourceCandidate) >= EffectiveMinEvidenceForPromotion()
//   - PromotionAdvisoryOnly     → never HARD
//
// The (Tier, len(SourceCandidate)) joint logic that used to live in
// validateFacetCoverage is centralised here so reviewer / accept-
// policy / future consumers all read the same answer.
func (r FacetRequirement) IsPromoted() bool {
	switch r.EffectivePromotionPolicy() {
	case PromotionAlwaysHard:
		return true
	case PromotionWhenEvidenceSufficient:
		return len(r.SourceCandidate) >= r.EffectiveMinEvidenceForPromotion()
	}
	return false
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
func ResolveQuestionFamily(rm RequestModel, sinks ...RichnessTelemetrySink) QuestionFamily {
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
	bucketCount := len(view.Buckets)

	// Rule 3 (R4.4): comparison family. When the question carries
	// 2+ user-named buckets (verbatim labels in QuestionStructure
	// .Buckets), a flat family (QFCallChain / QFEnumeration /
	// QFGeneric) would lose the partition. QFComparison scaffolds
	// one principal section per bucket so the user's mental
	// partition survives. Driven by the typed bucket-count signal,
	// not by question-text matching (§9.5 red line preserved).
	if bucketCount >= 2 {
		return QFComparison
	}

	// Rule 4: role-lookup detection (named-entity subject + no
	// enumeration obligation).
	if !hasObligation {
		switch rm.AnswerSubject.Kind {
		case SubjectFunctionName, SubjectHandlerRoute,
			SubjectConfigKey, SubjectStructField, SubjectInterface:
			return QFRoleLookup
		}
	}

	// Rule 5: trace intent without obligation.
	if rm.Intent == IntentTrace && !hasObligation {
		// bucketCount < 2 here (QFComparison handled above);
		// no telemetry needed — single-bucket trace cleanly fits
		// QFCallChain.
		return QFCallChain
	}

	// Rule 6: enumeration / obligation-bearing.
	if hasObligation || rm.Intent == IntentEnumerate {
		// bucketCount < 2 by construction (QFComparison absorbed
		// the multi-bucket case above).
		return QFEnumeration
	}

	// Rule 7: architecture explain.
	if rm.Intent == IntentExplain && rm.Scenario == ScenarioArchitectureExplain {
		return QFArchitecture
	}

	return QFGeneric
}

// emitFamilyUnderrepresentedIfBucketed records a "family did not
// model the question's bucket partition" telemetry signal. B5-F3
// 2026-05-03 — fires for QFCallChain / QFEnumeration / QFGeneric
// when QuestionStructure.Buckets has 2+ entries, the three families
// whose templates do NOT carry per-bucket structure. QFRootCauseTrace
// / QFConfigPrecedence / QFRoleLookup / QFArchitecture are exempted
// because their templates are bucket-orthogonal (one root cause vs
// many can still be cleanly expressed without per-bucket facets).
func emitFamilyUnderrepresentedIfBucketed(sinks []RichnessTelemetrySink, family QuestionFamily, bucketCount int) {
	if bucketCount < 2 {
		return
	}
	for _, sink := range sinks {
		if sink == nil {
			continue
		}
		sink.AppendRichnessTelemetry(RichnessTelemetrySignal{
			Kind:        "family_underrepresented",
			Family:      string(family),
			BucketCount: bucketCount,
			Reason:      "QuestionStructure.Buckets is multi-valued but resolved family does not preserve per-bucket structure",
		})
	}
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
// RichnessTelemetrySink names the (optional) channel
// CompileFacetCoverage / ResolveQuestionFamily use to record silent
// softening / family-fit fallback events. MutableState satisfies
// this interface via AppendRichnessTelemetry; nil sinks are a
// supported no-op so test callers and the answer_surface_plan path
// (which has no MutableState in scope at deep helpers) can stay
// silent.
type RichnessTelemetrySink interface {
	AppendRichnessTelemetry(sig RichnessTelemetrySignal)
}

func CompileFacetCoverage(rm RequestModel, surface []EvidenceItem, sinks ...RichnessTelemetrySink) *FacetCoverageContract {
	family := ResolveQuestionFamily(rm, sinks...)
	template := familyTemplate(family, rm)
	if template == nil {
		return nil
	}

	// R3.1 (post_shape_residual_audit.md, 2026-05-04): when the
	// caller invokes CompileFacetCoverage BEFORE any emit_evidence
	// has run (e.g. analyzer-time BuildAnswerSurfacePlan call),
	// surface is empty by construction — every facet looks
	// "uncovered" but only because evidence collection hasn't
	// started yet. Treat this as INCONCLUSIVE: skip both the
	// HARD→SOFT downgrade and the facet_softened telemetry, so
	// the false-positive signal eval baseline saw (4/4 runs
	// silently softening) goes away. The Required tier reaches
	// downstream untouched — later in the Run a re-compile with
	// real surface evidence either covers the facet OR keeps it
	// HARD for a real validator to act on.
	emptySurface := len(surface) == 0

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
		//
		// B5-F2 (2026-05-03): record a richness-telemetry signal so
		// the silent downgrade is observable. Callers that supply a
		// sink (orchestrator hot path via mut) accumulate signals;
		// callers that don't (test cases, deep helpers without
		// MutableState in scope) stay byte-identical to pre-B5-F2
		// behaviour.
		//
		// R3.1: emptySurface short-circuits the downgrade — no
		// evidence yet ≠ HARD facet uncoverable. Skip telemetry too.
		if !emptySurface && bound.Required == FacetHardRequired && len(bound.SourceCandidate) == 0 {
			bound.Required = FacetSoftRequired
			for _, sink := range sinks {
				if sink == nil {
					continue
				}
				sink.AppendRichnessTelemetry(RichnessTelemetrySignal{
					Kind:      "facet_softened",
					FacetKind: string(bound.Kind),
					Family:    string(family),
					Reason:    "hard requirement had no surface evidence matching any AcceptableForm; downgraded to SOFT to avoid false-fail",
				})
			}
		}
		// Phase 5: derive RichnessTier from the (post-fallback)
		// Required value so consumers reading Tier never need to
		// re-run the fallback rules.
		bound.Tier = TierFromRequiredness(bound.Required)
		// G6 (post_v2_runtime_gap_remediation, 2026-05-04): mirror
		// the same 1:1 derivation onto PromotionPolicy. Both fields
		// are filled here so downstream consumers (validators,
		// reviewers, accept policies) can read either surface
		// without re-running the projection.
		bound.PromotionPolicy = PromotionPolicyFromRequiredness(bound.Required)
		if bound.MinEvidenceForPromotion <= 0 {
			bound.MinEvidenceForPromotion = DefaultMinEvidenceForPromotion
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
		if matches := claimFormMatches(req.AcceptableForms, ClaimFormOf(item)); !matches {
			continue
		}
		// Sub-kind narrowing (2026-05-02): when RequiredSubKind is
		// set, the candidate evidence must additionally carry a
		// matching LogPerfSubKind. Empty RequiredSubKind disables
		// this filter (existing behaviour preserved for facets that
		// don't care about sub-kind).
		if req.RequiredSubKind != "" && item.LogPerfSubKind != req.RequiredSubKind {
			continue
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			out.SourceCandidate = append(out.SourceCandidate, id)
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
		// Sub-kind-aware refinement (Phase 4 extension, 2026-05-02):
		// the generic FacetObservedArtifactFact is narrowed to a
		// specific LogPerfSubKind when the attached log/perf bundle
		// declares a severity signal. A panic-trace question's
		// answer should anchor on a LogPanicFrame-classed evidence
		// item, not just any ClaimExternalObservation; without
		// RequiredSubKind, an answer that only cites LogNoiseFrame
		// evidence would technically pass the form check but miss
		// the failure shape entirely.
		artifactFacet := FacetRequirement{
			Kind: FacetObservedArtifactFact, Required: FacetHardRequired,
			AcceptableForms: []ClaimForm{ClaimExternalObservation},
			RequiredSubKind: rootCauseRequiredSubKind(rm),
		}
		return append([]FacetRequirement{
			artifactFacet,
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
	case QFComparison:
		// R4.4 (post_shape_residual_audit.md, 2026-05-04): the
		// shared commonFacets() helper already appends a
		// FacetBucketLabel HARD entry whenever Buckets >= 2 (the
		// canonical bucket-alignment signal); QFComparison's
		// dedicated facets are the *content* corroborators —
		// FacetCurrentCodePath SOFT for grounded code refs in
		// each bucket's section, FacetUncertaintyBoundary SOFT
		// for disclosing axis asymmetry between buckets.
		return append([]FacetRequirement{
			{Kind: FacetCurrentCodePath, Required: FacetSoftRequired,
				AcceptableForms: []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}},
			{Kind: FacetUncertaintyBoundary, Required: FacetSoftRequired},
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

// rootCauseRequiredSubKind picks the LogPerfSubKind a
// QFRootCauseTrace question's FacetObservedArtifactFact should
// require, based on the attached bundle's typed signals. When
// no specific severity / perf signal fires, returns
// LogPerfSubKindUnknown (no narrowing — generic
// ClaimExternalObservation acceptance).
//
// Priority: perf-side signals win over log-side (a perf-trace
// question is more specific than a log-trace question). Within
// log side: panic > crash > oom > timeout. Returns the FIRST
// matching sub-kind so the facet narrows to the strongest
// available evidence.
//
// Read-only on rm — pure projection.
func rootCauseRequiredSubKind(rm RequestModel) LogPerfSubKind {
	// Perf-side: prefer the strongest perf signal available.
	if rm.PerfTrace != nil {
		if len(rm.PerfTrace.Stalls) > 0 {
			return PerfStallFrame
		}
		if len(rm.PerfTrace.Janks) > 0 {
			return PerfJankFrame
		}
		if rm.PerfTrace.Startup != nil &&
			rm.PerfTrace.Startup.AppLaunchMs > PerfStartupSlowColdMs {
			return PerfStartupFrame
		}
	}
	// Log-side: severity ladder.
	if rm.LogTriage != nil {
		for _, s := range rm.LogTriage.Meta.Signals {
			if s == SignalPanic || s == SignalCrash {
				return LogPanicFrame
			}
		}
		for _, s := range rm.LogTriage.Meta.Signals {
			if s == SignalOOM {
				return LogOOMFrame
			}
		}
		for _, s := range rm.LogTriage.Meta.Signals {
			if s == SignalTimeout {
				return LogTimeoutFrame
			}
		}
	}
	return LogPerfSubKindUnknown
}
