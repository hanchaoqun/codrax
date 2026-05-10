package types

// Violation, ViolationKind, and SuspectedRoot live in the types
// package (not internal/analysis/contract) so EvidenceClosure can
// embed a []Violation ledger without creating the circular import
// contract → types → contract. The contract package re-exports these
// via Go type aliases (see internal/analysis/contract/checker.go), so
// call sites continue to write contract.Violation / contract.ViolFamilyMismatch.
//
// Session 11 F1 (2026-04-18) added the Stage / DispatchID /
// EvidenceRefs / SuspectedRoot fields so every enforcer write can
// carry structured self-diagnosis. The F2 aggregator groups events
// by SuspectedRoot.IRField and weights by Confidence so the F3
// IRPatchEngine can decide whether to reconcile upstream IR.

// ViolationKind classifies a contract or enforcer breach.
type ViolationKind string

const (
	// Original contract-checker kinds (pre-session-11).
	ViolFamilyMismatch   ViolationKind = "shape"
	ViolCitation         ViolationKind = "citation"
	ViolMustInclude      ViolationKind = "must_include"
	ViolMustExclude      ViolationKind = "must_exclude"
	ViolAcceptance       ViolationKind = "acceptance"
	ViolSuccessCriterion ViolationKind = "success_criterion"

	// Session 11 F1 — enforcer kinds beyond the classic
	// contract.Check path. Each has a dedicated SuspectedRoot
	// template filled at the enforcer hookup (see the full design
	// doc §1.4 table).

	// ViolGhostAnchor: D2 chain promotion skipped a chain whose
	// anchor file is outside ScannedSet. SuspectedRoot: ScannedSet.
	ViolGhostAnchor ViolationKind = "ghost_anchor"

	// ViolChainDemoted: ERM L0-1 terminal-predicate demote. Only the
	// self-ref subset (terminal literal == primary entity name) is
	// written to the ledger to avoid noise from routine demotes that
	// carry no root-cause signal.
	ViolChainDemoted ViolationKind = "chain_demoted"

	// ViolSelfRefLiteral: emit_evidence / emit_answer_* saw a
	// literal equal to the question's primary entity — self-reference
	// trap. SuspectedRoot: answer_subject.kind.
	ViolSelfRefLiteral ViolationKind = "self_ref_literal"

	// ViolPreCompleteDowngrade: emit_investigation_complete CGEC I3
	// pre-complete simulator rejected a complete-claim because the
	// closure snapshot would not pass the contract checker.
	ViolPreCompleteDowngrade ViolationKind = "pre_complete_downgrade"

	// ViolLiteralFormFailed: C5 G1 literal form check rejected a
	// value-shape literal that does not match the AnswerSubject.Kind
	// regex rule (e.g. skill_name must end with -skill).
	ViolLiteralFormFailed ViolationKind = "literal_form_failed"

	// ViolViewSwap: B2a/B2b shape swap fired. Kept in the ledger
	// for audit even after C4 removes the can_correct=true rescue —
	// operators still want to observe how often the LLM picks the
	// wrong shape.
	ViolViewSwap ViolationKind = "shape_swap"

	// Commit 53 P2 — read-mode answer Shape Oracle violations.
	// Defaults are SOFT (advisory only, mirrored to closure but
	// don't trigger finalize retry) so the new gates can ship
	// without breaking edge-case answers; operators can promote
	// to strict via gate_contract_strict_kinds yaml.

	// ViolViewIntentMismatch: the resolved AnswerSemanticView's
	// family contradicts Intent / Scenario from RequestModel (e.g.
	// a scalar BlockScalar answer for an "explain how X works"
	// intent). Produced by the finalize-stage view oracle in
	// orchestrator/contract_check.go. SuspectedRoot: question_kind.
	ViolViewIntentMismatch ViolationKind = "view_intent_mismatch"

	// ViolSubTopicCountMismatch: doc.AnswerSymbols' distinct
	// SubTopic-bucket count diverges from len(IR.SubTopics).
	// Catches multi-topic answers that under-cover or over-cover.
	// Produced by the finalize-stage view oracle in
	// orchestrator/contract_check.go. SuspectedRoot: sub_topics.
	ViolSubTopicCountMismatch ViolationKind = "sub_topic_count_mismatch"

	// ViolDiagramIdentifier: a bare CamelCase / snake_case
	// identifier inside a mermaid block does not resolve to a
	// real symbol via the SymbolOracle. Caught by P4 diagram
	// validator. SuspectedRoot: diagram.
	ViolDiagramIdentifier ViolationKind = "diagram_identifier_unverified"

	// Commit 55 Batch A.3 — declared-count drift between extractor
	// and finalizer. emit_answer_symbol stamps the LLM's self-
	// declared count on Mutable; if the finalizer's rendered
	// doc.Symbols length differs (items dropped by mid-loop
	// grounding etc.), the count claim is no longer accurate.
	// Soft-by-default. SuspectedRoot: answer_symbol.
	ViolDeclaredCountDrift ViolationKind = "declared_count_drift"

	// Commit 62 — answer-prose internal self-contradiction.
	// Independent reviewer LLM compared the answer's summary
	// section vs its body bullets and detected a factual claim
	// asserted with opposite values across the two parts. Default
	// classification is STRICT when
	// pipeline_self_consistency_rewrite_on_contradiction=true
	// (operator opts in to retry-on-contradiction); when false,
	// the kind moves to soft (telemetry-only). SuspectedRoot:
	// answer_summary_body_consistency.
	ViolSelfContradiction ViolationKind = "self_contradiction"

	// ViolExternalArtifactUnderdecoded fires when the user attached
	// an external artifact (runtime log via --log / --log-text /
	// REPL /log; perf trace via --htrace / --atrace; future
	// attached config dumps), the system successfully triaged it
	// into a structured bundle (LogBundle / PerfBundle on
	// MutableState), but the final answer references too few of
	// the bundle-extracted fields. The triage layer's job is to
	// turn opaque text into a typed payload (Errors[].Type, Frame
	// .Symbol, Signal name, Stall.symbol, Jank.trigger_span,
	// Startup.mode, etc.); the answer's job is to DECODE / EXPLAIN
	// these fields for the operator. Without this gate, the
	// finalizer can ship an answer that names a file:line from
	// repo code while completely ignoring the panic signal address,
	// the goroutine context, or the parameter values literally
	// printed in the trace — i.e. all the per-incident
	// diagnostic content the user pasted in.
	//
	// Trigger: structural (Mutable.LogTriage() != nil OR
	// Mutable.PerfTrace() != nil), NOT keyword/intent. Generalises
	// to any future "extract structured payload from user-attached
	// artifact" surface; new triage kinds plug in by appending to
	// the bundle-token-collection helper, not by editing skill
	// prompts. SuspectedRoot: external_artifact_decoded.
	ViolExternalArtifactUnderdecoded ViolationKind = "external_artifact_underdecoded"

	// ViolAuthorityOverreach fires when the rendered answer cites
	// drift-bounded evidence (Authority ∈ {conditional, historical,
	// illustrative}) but the rendered draft text lacks the system-
	// private Authority caveat tag that render.ApplyAuthorityHedging
	// embeds in every system-generated caveat. The check is
	// structural, not keyword-based: it greps for a single
	// well-defined token (render.AuthorityCaveatTag()), not for
	// per-shape hedge markers. Triggering this violation means the
	// rendered draft did NOT flow through ApplyAuthorityHedging —
	// the renderer was bypassed entirely (typically the IsZero
	// raw-prose fallback when emit_answer_document failed) or some
	// downstream transformation stripped the caveat.
	//
	// Strict by classification — wired into extraStrict in
	// cmd/root.go so the finalizer retries via the structured
	// emit channel. The repair hint speaks in LLM-actionable terms
	// (call emit_answer_document with required fields), not
	// internal plumbing names. SuspectedRoot: answer_authority.
	//
	// NOT collected by answer_taxonomy: this is a plumbing failure
	// signal (renderer bypass), not an answer-quality lesson the
	// LLM should learn across runs. See orchestrator's
	// learnFromContractViolations filter.
	ViolAuthorityOverreach ViolationKind = "authority_overreach"

	// Block 1 (architecture overhaul 2026-05-02) — reviewer-side
	// kinds. plan_critic, reflector, and answer_reviewer all run as
	// independent LLMs and were previously isolated from the
	// EvidenceClosure ledger (their output flowed into ChangePlan
	// .PlanCritique, Mutable.PlanningHint, and the cross-Run
	// failure_taxonomy disk cache respectively). Block 1 wires them
	// to also append to the ledger so a single read API
	// (StageHealthSnapshot) sees every reviewer's findings and
	// Block 3's selective fallback policy can route on the kind.
	//
	// All three are SOFT by default — they are observational signals
	// that should NOT immediately retry-loop the pipeline. Operators
	// who want strict behaviour add the kind to
	// pipeline_contract_strict_kinds.

	// ViolPlanCritic carries one risk emitted by plan_critic for the
	// just-emitted ChangePlan. Stage="plan". Confidence reflects the
	// reviewer's self-rated overall confidence (passed through to
	// SuspectedRoot.Confidence).
	ViolPlanCritic ViolationKind = "plan_critic_risk"

	// ViolReflectorObservation carries one diagnostic observation the
	// reflector LLM emitted between verify failure and plan re-dispatch.
	// Stage="verify". Distinct from emit_failure_pattern (cross-Run
	// taxonomy entry) — Observation is the per-iteration critique that
	// stays in this Run.
	ViolReflectorObservation ViolationKind = "reflector_observation"

	// ViolAnswerReviewerDistilled carries one distilled answer-pitfall
	// the answer_reviewer found in the just-finalized AnswerDocument.
	// Stage="finalize". Mirrors the pattern persisted to the cross-Run
	// answer_taxonomy cache (commit 51) so the same signal feeds both
	// in-Run health snapshots and cross-Run learning.
	ViolAnswerReviewerDistilled ViolationKind = "answer_reviewer_distilled"

	// Block 2 (architecture overhaul 2026-05-02) — Intent → Answer
	// axis-coverage violations. Each fires when the analyzer's
	// emit_analysis output declared one shape of question but the
	// finalizer / extractor produced an answer that does not satisfy
	// the structural expectation for that shape. SOFT-by-default —
	// the answer ships with telemetry, operators promote to strict
	// via pipeline_contract_strict_kinds.

	// ViolIntentTraceShallow fires when rm.Intent == IntentTrace
	// (LLM said "trace this from X to Y") but the answer has fewer
	// than 2 hops in steps[] AND no sequenceDiagram fenced block in
	// the summary. A trace answer with one bullet or no diagram
	// failed to walk the chain — almost always a regression where
	// the explorer found one anchor and the finalizer wrapped it
	// in prose without crossing the actual intermediate functions.
	ViolIntentTraceShallow ViolationKind = "intent_trace_shallow"

	// ViolIntentEnumerateNotList fires when rm.Intent ==
	// IntentEnumerate (LLM said "list all X") but the answer's
	// shape is neither list_of_symbols NOR a step_list with ≥2
	// items. Enumeration questions need a list-shaped surface;
	// a single explanation paragraph is structurally wrong.
	ViolIntentEnumerateNotList ViolationKind = "intent_enumerate_not_list"

	// ViolIntentRootCauseNoCause fires when rm.Intent ==
	// IntentRootCause (LLM said "why does X happen") but the
	// AnswerDocument has zero citations naming concrete causal
	// anchors. Root-cause answers must rest on at least one
	// grounded site — a hand-wave explanation without anchors is
	// a regression.
	ViolIntentRootCauseNoCause ViolationKind = "intent_root_cause_no_cause"

	// ViolIntentConfigNoTrail fires when rm.Intent ==
	// IntentConfigQuery (LLM said "what's the value / where is X
	// configured") but the answer has neither shape=value /
	// config_value nor a citation pointing at a config-shaped
	// path (yaml / json / toml / ini / .env / shell rc). The
	// canonical config-query answer needs at least one of these
	// signals; an Explanation with no config citation is wrong.
	ViolIntentConfigNoTrail ViolationKind = "intent_config_no_trail"

	// ViolSubjectAnchorMissing fires when the analyzer declared a
	// concrete AnswerSubject (Kind ∈ {SubjectFunctionName,
	// SubjectTypeName, SubjectHandlerRoute, SubjectConfigKey,
	// SubjectStructField, SubjectInterface}) at confidence ≥
	// coherenceSubjectConfidenceFloor (0.6), but the rendered
	// AnswerDocument neither exposes this subject in doc.Symbols /
	// doc.Steps (their .anchor fields) nor mentions it in the
	// summary by inline backtick code. The subject the LLM said
	// the question was about is missing from the answer surface.
	ViolSubjectAnchorMissing ViolationKind = "subject_anchor_missing"

	// ViolPredicateAxisMissing fires when rm.PredicateAxis is set
	// (LLM declared the action verb) but the closure carries no
	// EvidenceItem whose AnchorKind belongs to the axis's allowed
	// set (per types/axis_anchor_map.go::PredicateAxisToAnchorKinds).
	// E.g. PredicateAxis = AxisCall but no evidence with
	// AnchorKind = AnchorCall — the answer is grounded but in the
	// wrong shape (definition / assignment instead of the call).
	ViolPredicateAxisMissing ViolationKind = "predicate_axis_missing"

	// ViolFacetUncovered fires when a FacetCoverageContract.Required
	// entry whose Required==FacetHardRequired has zero corresponding
	// payload coverage in the rendered AnswerDocument. "Coverage"
	// means at least one AnswerStep / AnswerSymbol / AnswerValue /
	// AnswerBoolean carries a RenderedClaimUse whose FacetID matches
	// the facet's Kind, OR (when LLM omitted ClaimUse) a payload
	// whose referenced citation has a ClaimFormOf in the facet's
	// AcceptableForms.
	//
	// Phase 4 default classification: SOFT — facet gaps usually
	// require additional evidence collection (BackToExplore) rather
	// than a finalizer rewrite, and the evaluation oracle for
	// "did the LLM cover facet X" is itself fuzzy enough that hard-
	// rejecting on a miss risks false positives. Operators promote
	// to STRICT via pipeline_contract_strict_kinds for repos where
	// all answers must be facet-complete.
	ViolFacetUncovered ViolationKind = "facet_uncovered"

	// ViolClaimFormUnsupported fires when a payload's RenderedClaimUse
	// names a ClaimForm that is INCOMPATIBLE with the underlying
	// evidence's actual ClaimForm projection. Example: a step's
	// ClaimUse declares claim_form=call_edge but the cited
	// EvidenceItem has AnchorKind=Definition (ClaimFormOf returns
	// ClaimDefinitionFact, not ClaimCallEdge).
	//
	// Phase 4 default classification: STRICT — this is an explicit
	// LLM-emitted self-contradiction. The LLM either named the wrong
	// facet or cited the wrong evidence for it; either way the
	// finalizer can fix it without new evidence (FinalizerOnly).
	ViolClaimFormUnsupported ViolationKind = "claim_form_unsupported"

	// ViolAbsenceScopeExceeded fires when the answer prose claims a
	// broader absence than its citation can support. Example: the
	// summary says "X is not used anywhere in the codebase" but the
	// only Scope=Negative citation is a per-package grep (NegativePattern
	// scoped to internal/foo/). The repo has not been searched
	// exhaustively; the absence claim oversteps.
	//
	// Phase 4 default classification: STRICT — safety-critical for
	// config-trace / absence-question shapes where downstream
	// consumers act on the negative finding (operator removes a
	// config knob, etc.). Fallback target is BackToExtract: the
	// extractor must re-emit a tighter absence framing or surface
	// the bounded scope verbatim.
	ViolAbsenceScopeExceeded ViolationKind = "absence_scope_exceeded"

	// ViolMissingRequestedRoleUndisclosed fires when a
	// config-precedence exact-absence answer has one or more
	// user-requested precedence layers that remained unbound in the
	// grounded evidence surface, but the emitted V2 document does
	// not disclose them via document-level missing_requested_roles[].
	//
	// This is a finalizer-local surface failure: the semantic view
	// already compiled the missing role set and the renderer already
	// knows how to materialise it. No new evidence is required; the
	// fix is to populate the typed field correctly.
	ViolMissingRequestedRoleUndisclosed ViolationKind = "missing_requested_role_undisclosed"

	// ViolStepIdentifierUnverified (Phase 4 extension, 2026-05-02)
	// fires when an AnswerStep's prose contains a backtick-quoted
	// identifier (a load-bearing inline-code token) that does NOT
	// appear in the typed evidence pool's Subject / Object /
	// AnchorSymbol / AnswerSymbol.Name fields.
	//
	// Pure new-world signal: the verified-identifier set is built
	// from typed EvidenceItem fields the explorer / extractor
	// emitted during the Run, NOT from raw file-content scraping.
	// A step.description that uses an identifier the typed evidence
	// pool never structurally observed is by definition introducing
	// a name without provenance — typically a hallucination where
	// the LLM remembered a position (file:line) but invented the
	// name at that position.
	//
	// Default classification: SOFT (typical Phase 4 default; the
	// signal is precise but missing-from-typed-pool COULD reflect
	// a legitimate identifier the explorer skipped, so soft fail
	// + retry hint is the right escalation). Operators promote to
	// STRICT via pipeline_contract_strict_kinds when their
	// pipeline guarantees full-coverage emit_evidence. Fallback
	// target: BackToExplore — the missing identifier means the
	// explorer's emit_evidence missed structurally capturing it.
	ViolStepIdentifierUnverified ViolationKind = "step_identifier_unverified"

	// ViolRichnessRegression (Phase 5 of Semantic Surface Contract,
	// 2026-05-02) is a TELEMETRY-ONLY signal — the answer covered
	// every Hard / Soft facet but skipped one or more
	// FacetOptional (TierEnrichment) entries the evidence pool
	// could have supported. The closure ledger records the gap so
	// end-of-Run [CGEC] summary surfaces optional_facets_covered=N/M
	// for cross-Run trend tracking; the finalizer NEVER retries on
	// this kind. SOFT-by-default and explicitly NOT promotable to
	// STRICT — it is a richness-coverage observation, not a
	// correctness gate. Phase 4 hard kinds catch the correctness
	// half of the contract; this kind only watches "did we leave
	// useful supplemental context on the table".
	ViolRichnessRegression ViolationKind = "richness_regression"

	// ViolValueSecondaryCitationOffFocus (Phase 6 stage 7,
	// 2026-05-03) fires when a shape=value answer carries 2+
	// citations and at least one secondary citation does not
	// directly support the emitted scalar literal under the
	// AnswerSubject.Kind's lookup discipline. The finalizer renders
	// scalar answers with one defining citation; secondary
	// citations must each have their matched evidence pool entry
	// (Subject / AnchorSymbol / Object) name the same literal —
	// otherwise the citation is broader background and belongs in
	// summary prose without an extra cite. SOFT-by-default; the
	// pre-Phase-6 emit-time gate (validateValueCitationFocus) used
	// citation file/line plus token-overlap fallbacks; this oracle
	// reads only the typed evidence pool so generic-token false
	// negatives are eliminated. Fallback target: BackToExtract —
	// the extractor selects citations.
	ViolValueSecondaryCitationOffFocus ViolationKind = "value_secondary_citation_off_focus"

	// ── B4 V2 block-only carrier validators ─────────────────────
	// 4 SOFT-by-default violation kinds raised by the V2 oracle
	// dispatch path (block_only_carrier.md §5.4). They never affect
	// Result.Passed during B4-B5 (telemetry-only); B6 promotes them
	// to STRICT once V2 becomes the default carrier.

	// ViolBlockCoverageMissing fires when the rendered V2 doc fails
	// to satisfy a BlockRequirement.Required=true entry from the
	// AnswerSemanticView — required block kind absent OR present
	// but below MinCount. Diagnoses "LLM emitted V2 but skipped a
	// required block kind". Stage="finalize". Default fallback
	// FallbackFinalizerOnly during telemetry; B6 promotes to
	// FallbackBackToExtract.
	ViolBlockCoverageMissing ViolationKind = "block_coverage_missing"

	// ViolPrincipalClaimUseMissing fires when a SurfaceRole=principal
	// block carries no RenderedClaimUse on its Items[] (or block-
	// level ClaimUses[]) AND the BlockRequirement's
	// AcceptableClaimForms list is non-empty (i.e. the LLM was
	// supposed to declare what claim form backs the principal
	// payload). Stage="finalize". SOFT-by-default.
	ViolPrincipalClaimUseMissing ViolationKind = "principal_claim_use_missing"

	// ViolDiagramEdgeUnsupported fires when the V2 doc carries a
	// BlockDiagram whose body lists nodes/edges that don't have
	// matching ClaimUses (or whose declared Diagram.Kind disagrees
	// with the AnswerSemanticView's DiagramFacetGraph.Kind). Catches
	// "LLM drew a diagram but didn't declare which facets each
	// edge represents". Stage="finalize". SOFT-by-default.
	ViolDiagramEdgeUnsupported ViolationKind = "diagram_edge_unsupported"

	// ViolDiagramEdgeLabelMismatch fires when an edge carries a typed
	// RelationKind on its EdgeAnchor entry AND the rendered label
	// resolves (via InferRelationFromLabel) to a DIFFERENT non-Unknown
	// RelationKind. Surfaces a consistency drift — the typed
	// declaration is the authority for legality checks, but a
	// mismatched label confuses readers. SOFT-by-default and never
	// promotable to STRICT (label inference is a noisy signal — R3
	// invariant: noisy signals only drive soft guidance).
	// Stage="finalize". G3 (post_v2_runtime_gap_remediation,
	// 2026-05-04).
	ViolDiagramEdgeLabelMismatch ViolationKind = "diagram_edge_label_mismatch"

	// ViolAnswerSemanticUnderfilled fires when the SemanticQualityReviewer
	// (G5 post_v2_runtime_gap_remediation, 2026-05-04) judges the
	// answer thin — promoted facets uncovered, diagram edge minimums
	// short, or richness candidates with available typed evidence
	// not surfaced. SOFT-by-default; operators may promote to STRICT
	// via pipeline_contract_strict_kinds when their pipeline
	// guarantees richer answers. Stage="finalize".
	ViolAnswerSemanticUnderfilled ViolationKind = "answer_semantic_underfilled"

	// ViolAnswerTopicMismatch fires when the SemanticQualityReviewer
	// specifically reports kind=topic_mismatch: the final answer's
	// main subject diverged from the current user request. Unlike
	// ordinary semantic underfill/richness concerns, this is HIGH
	// severity by default and retry-eligible: a wrong-topic answer
	// must not ship just because its prose is otherwise grounded.
	// Stage="finalize".
	ViolAnswerTopicMismatch ViolationKind = "answer_topic_mismatch"

	// ViolRichnessGlaringGap fires when an Optional facet is
	// flagged as EnrichmentGlaring AND has typed-evidence support
	// (len(SourceCandidate) >= family threshold) AND the rendered V2
	// doc does NOT cover it. Severity = Medium (retry-eligible) —
	// the LLM gets an actionable prompt to re-emit with the missing
	// facet declared. Per-family glaring marking lives in
	// compile_<family>.go via markGlaringFacets.
	//
	// SOFT-by-default; promotable via pipeline_contract_strict_kinds.
	// Stage = "finalize". Layer = "v2_oracle". B2 v3 (2026-05-04).
	ViolRichnessGlaringGap ViolationKind = "richness_glaring_gap"

	// ViolPrincipalProseUnderfilled fires when a principal block
	// declares ≥ 3 typed claim_use annotations but the rendered
	// prose contains ZERO Markdown inline-code references — the
	// surrounding text was abstracted away from the cited evidence.
	// Per-block-kind dispatch (Summary / Section / Caveat use
	// block.Text; OrderedList / BulletList aggregate items[].Text;
	// Scalar / Decision / Diagram / Table skip).
	//
	// Severity = Medium (retry-eligible). The LLM gets a pointer to
	// the offending block id + a request to anchor at least one
	// inline identifier so prose density returns to evidence-grounded.
	// SOFT-by-default; promotable via pipeline_contract_strict_kinds.
	// Stage = "finalize". Layer = "v2_oracle". B2 v3 (2026-05-04).
	ViolPrincipalProseUnderfilled ViolationKind = "principal_prose_underfilled"

	// ViolDiagramRelationLabelOnly fires when EdgeRelations.Min is
	// satisfied for some relation kind ONLY because label-inferred
	// edges fill the gap (i.e. typed RelationKind on edge_anchors[]
	// is below Min on its own; total typed + label-only counts
	// reach Min). The contract is satisfied so the answer ships,
	// but the validator emits a SOFT advisory encouraging the LLM
	// to declare `relation_kind` directly on edge_anchors[] for the
	// label-only edges so future readers see the typed authority.
	//
	// SOFT-by-default; promotable via pipeline_contract_strict_kinds
	// when the operator wants typed declarations enforced. Stage =
	// "finalize". Layer = "v2_oracle". B3 (v3 runtime consolidation,
	// 2026-05-04).
	ViolDiagramRelationLabelOnly ViolationKind = "diagram_relation_label_only"

	// ViolEnumerationEvidenceUnderspecified fires when the user's
	// question carries an explicit enumeration count N (via the
	// analyzer's EnumerationBoundary) AND the family is enumeration-
	// shaped (QFEnumeration / QFRootCauseTrace / QFCallChain) AND the
	// evidence pool has fewer than N distinct typed anchor_symbol
	// values. Diagnoses the s1a-class failure where the explorer
	// aggregates N parallel callsites into a single line_range item
	// with anchor_symbol pointing at the container — leaving the
	// pool with too few typed names for the renderer to enumerate.
	//
	// SOFT-by-default. Routes to FallbackBackToExplore on retry —
	// the only honest fix is gathering more evidence, not finalizer
	// rewrite. Operators may promote to STRICT via
	// pipeline_contract_strict_kinds once eval confirms low
	// false-positive rate; the typed signal (DeclaredCount integer
	// + len(unique anchor_symbol) integer) supports promotion.
	// Stage="extract" (fires after evidence pool is finalised).
	// 修 B (post_v2_runtime_gap_remediation, 2026-05-04).
	ViolEnumerationEvidenceUnderspecified ViolationKind = "enumeration_evidence_underspecified"

	// ViolUncertaintyBlockMissing fires when an UncertaintyRule's
	// trigger fired (e.g. FacetObservedArtifactFact present in
	// FacetCoverage.Required) but no matching BlockCaveat exists
	// in doc.Blocks. Per the AnswerSemanticView contract, the LLM
	// must disclose log-source drift / scope absence / external-
	// observation provenance via a caveat block. Stage="finalize".
	// SOFT-by-default.
	ViolUncertaintyBlockMissing ViolationKind = "uncertainty_block_missing"

	// ViolStructuralEnumerationDivergence (P3 #6 precise variant,
	// 2026-05-03) fires when the answer's emitted set diverges from
	// the typed Symbol.Implements relation AND the omitted items
	// are silent — neither listed (with caveat or otherwise) in
	// doc.Symbols nor named verbatim in doc.Summary. The defining
	// case: the user asks "list all implementers of <Iface>",
	// Graph.ImplementersOf reports 8 concrete types via method-set
	// match, but the LLM emitted only 7 because some narrative cue
	// (a comment, doc fragment, naming heuristic) made the LLM
	// quietly drop one. Both signals (typed structural relation +
	// LLM prose reasoning) are legitimate; the bug is the silent
	// drop, not the disagreement itself.
	//
	// Repair contract (mirrored in change-plan-skill / answer-document
	// -skill prompt teaching): when typed and emitted sets diverge,
	// the LLM MUST either (a) include the divergent item with a
	// rationale starting with "[caveat]" that names the disagreeing
	// signal, OR (b) name the item verbatim in summary as an
	// exception case so the user can see the divergence and judge.
	// Silently selecting one side of the disagreement is the
	// transparency failure the oracle catches.
	//
	// Stage="finalize". SOFT-by-default — the answer ships with the
	// divergence noted in the closure ledger; operators promote via
	// pipeline_contract_strict_kinds when they want
	// answer-rewrite-on-divergence behaviour. Fallback target (when
	// promoted to STRICT): BackToExtract — the extractor re-emits
	// emit_answer_symbol with the missing names included, the
	// finalizer then re-renders with the divergence acknowledged.
	//
	// Red-line discipline: the gate reads (a) typed Graph.ImplementersOf
	// output (deterministic method-set match), (b) LLM-emitted
	// doc.Symbols[i].Name (verbatim string), (c) doc.Summary
	// substring match (verbatim). Three precise signals; zero
	// keyword tables or fuzzy heuristics. Per the precise-signals
	// -for-hard-gates principle this oracle CAN drive a structural
	// fallback, but defaults soft so the user keeps an answer even
	// when transparency is partial.
	ViolStructuralEnumerationDivergence ViolationKind = "structural_enumeration_divergence"

	// ViolCrossCitationConflict (B6-F1, 2026-05-04) fires when the
	// rendered AnswerDocument cites the same SYMBOL across multiple
	// citations whose (file, line) tuples disagree. The single-locus
	// rule: if symbol X is cited in two payloads and the citations
	// resolve to different files OR different lines (>2-line drift),
	// at most one of the cites can refer to X's actual definition —
	// the other points at a fragment that mentions X without being
	// the locus of the claim. Real-world case (m1a-...): one cite
	// referenced extractor.go:114, another extractor.go:135, both
	// labelled "the same Turn-B handoff function" — the user reads
	// the answer and cannot tell which line is the actual handoff.
	//
	// Detection: walk doc.Symbols + every AnswerStep's Anchor and
	// CitationRef pairs; group cites by canonical symbol name
	// (Symbols[i].Name OR an anchor.Symbol field); within each
	// group flag any pair whose File OR (|line_a - line_b| > 2)
	// disagree. The 2-line tolerance covers receiver-decl vs body-
	// signature situations where two cites legitimately point at
	// adjacent lines of the same function.
	//
	// Stage="finalize". SOFT-by-default — operators may want
	// retain-and-caveat behaviour rather than auto-rewrite. Fallback
	// target (when promoted to STRICT via pipeline_contract_strict_kinds):
	// BackToExtract — the extractor must pick consistent loci before
	// the finalizer re-renders. Repair text is LLM-actionable:
	// names the conflicting symbol + lists the disagreeing cites.
	ViolCrossCitationConflict ViolationKind = "cross_citation_conflict"

	// ViolDemotionStorm fires when the per-Run ClosureStats.ChainsDemoted
	// counter exceeds the configured threshold. Each demotion event
	// individually is a normal CGEC enforcement signal (a hallucinated
	// chain or terminal-predicate self-ref correctly stripped from the
	// prompt); collectively a high-frequency stream signals the
	// explorer is over-producing low-quality chains, which the
	// operator should see in the Run summary as a single SOFT
	// violation rather than buried in the [CGEC] summary INFO line.
	//
	// SOFT-by-default — telemetry-only. Defaulted threshold is 10
	// per-Run; configurable via pipeline_demotion_storm_threshold.
	// Stage="finalize" (recorded at end-of-Run after all stages have
	// closed). Never blocks shipping.
	ViolDemotionStorm ViolationKind = "demotion_storm"

	// ViolForcedReadStorm fires when ClosureStats.ForcedReads exceeds
	// the configured threshold. Each forced read individually is a
	// recovery action (the orchestrator paged a file the LLM should
	// have read but skipped); a high count signals the explorer's
	// keyword_search is consistently leaving primary anchors
	// unread. Same SOFT-by-default + per-Run + Stage="finalize"
	// semantics as ViolDemotionStorm.
	ViolForcedReadStorm ViolationKind = "forced_read_storm"

	// ViolSymbolAnchorMismatch (P1 #3, 2026-05-03) fires when the
	// extractor accumulated ≥ symbolAnchorMismatchThreshold (default 3)
	// emit_answer_symbol rejections in a single Run AND the rendered
	// AnswerDocument carries strictly fewer Symbols than the analyzer's
	// declared count for the question (or zero when no count was
	// declared but the answer shape is list_of_symbols). Diagnostic:
	// the explorer either never read the def-regions of the user's
	// principal entities, or read them but the lines surfaced through
	// keyword_search were the call-site lines rather than the def lines.
	// Either way the right escalation is BackToExplore so the next
	// iteration can re-investigate with a hint pointing at the missing
	// def-region symbols.
	//
	// Stage="extract" (the rejections are logged from emit_answer_symbol
	// inside Turn B). SOFT-by-default (telemetry signal first; the
	// orchestrator's existing iteration-cap escape and pre-existing
	// soft fail-safe paths must observe the gap before retrying or
	// shipping). Operators promote via pipeline_contract_strict_kinds.
	// Fallback target (when promoted to STRICT): BackToExplore — the
	// rejected lines mean keyword_search did not cover def-region.
	ViolSymbolAnchorMismatch ViolationKind = "symbol_anchor_mismatch"

	// ViolEnumerationLabelUngrounded (post-shape s1a-20260504-064754
	// hallucination forensic, 2026-05-04) fires when an
	// ordered_list / bullet_list block in the V2 carrier carries
	// items[i].label values that do NOT appear as a substring of any
	// EvidenceItem.AnchorSymbol / Subject / Object in the dispatch's
	// evidence pool. Detected case: explorer emitted 28 grounded
	// items naming the 9 real gate.Run checks (checkCoverage /
	// checkContractComplete / etc.); finalizer emitted 9 fabricated
	// labels (checkCrossSignalCoherence / checkAnswerSubjectKindIsValid
	// / etc.) that no evidence item supported. self_consistency
	// reviewer compared SUMMARY vs BODY (consistent=true) but no
	// existing oracle compared BODY vs evidence pool, so the
	// hallucination shipped. Default classification: STRICT — silent
	// hallucination is the worst-case answer-quality regression.
	// Fallback target: BackToExtract — the extraction stage owns
	// item composition; replaying explore wastes budget.
	ViolEnumerationLabelUngrounded ViolationKind = "enumeration_label_ungrounded"

	// ViolInlineIdentifierHallucinated (Fix I, 2026-05-07, post-Fix-D
	// post-batch eval forensic) fires when a block's prose text
	// (Title / Text / Items[].Text fields on Summary / Section /
	// Scalar / Decision / Caveat blocks) contains an inline
	// backtick-delimited identifier like `\`checkAnswerContract\`` that
	// the typed graph SymbolOracle reports as Tier 0 — i.e., the
	// LLM rendered a fabricated name within prose where the existing
	// list-block-only Fix C / diagram-only Fix D validators could
	// not see it.
	//
	// Detected case (s1a r1, batch eval 2026-05-07): finalizer LLM
	// emitted a 9-item enumeration as markdown numbered prose inside
	// a BlockSection.Text field, with each item using inline
	// backtick to quote a check name. Five of the nine names were
	// "semantically right but lexically fabricated" partial
	// hallucinations: `checkAnswerContract` (real:
	// checkContractComplete), `checkSubtopicConsistency` (real:
	// checkSubtopicCoherence), `checkShapeSubjectConsistency` (real:
	// checkShapeSubjectCoherence), `checkCriterionResolution`
	// (real: checkCriterionResolvable), `checkCoherence` (real:
	// checkPendingFieldsWellformed — fully fabricated). All five
	// resolved to Tier 0 in the graph's flat-form index, but
	// because the answer was rendered in prose-text rather than
	// ordered_list items[].label, neither Fix C nor any other
	// validator caught the drift, and the answer shipped to the
	// user with mis-attributed file:line citations.
	//
	// Distinct from the sister hallucination kinds:
	//
	//   - ViolEnumerationLabelHallucinated — list-block items[].label
	//   - ViolDiagramEdgeEndpointHallucinated — mermaid edge endpoints
	//   - ViolInlineIdentifierHallucinated   — prose-text inline backticks
	//
	// Together these three cover every surface where the LLM may
	// render a symbol-shape token; collectively they enforce the
	// "every visible identifier MUST be a real codebase symbol"
	// contract across all V2 block kinds.
	//
	// Default classification: Medium severity, retry-eligible
	// finalizer-only. Operators promote via
	// pipeline_contract_strict_kinds. Recovery: finalizer re-emit
	// the block's prose with real names from the existing slate.
	ViolInlineIdentifierHallucinated ViolationKind = "inline_identifier_hallucinated"

	// ViolDiagramEdgeEndpointHallucinated (Fix D, 2026-05-07,
	// post-Fix-C diagram audit) fires when a mermaid edge's `from`
	// or `to` node identifier in a BlockDiagram is identifier-
	// shaped (≥10 chars), passes the existing
	// validateDiagramEdgeSupport substring vouch (because some
	// support-pool prose token happens to be a substring of the
	// fabricated name), but the typed graph SymbolOracle reports
	// no Tier 1-2 codebase declaration for the leading identifier.
	//
	// Distinct from ViolDiagramEdgeUnsupported:
	//
	//   - Unsupported = "edge endpoint not vouched by any item /
	//     title / claim_use / declared node label" (the LLM cited
	//     a name that's not in the answer's grounding pool).
	//   - EndpointHallucinated = "the typed graph does not contain
	//     the named symbol at all" (the LLM fabricated a node
	//     name; the substring vouch was prose noise).
	//
	// Detected risk profile (mirrors s1a r1 enumeration case
	// extended to diagrams): finalizer emits a call_dag /
	// class_diagram with a fabricated compound symbol name like
	// `validateFakeCoherenceCheck` and the support pool's prose
	// (e.g. "coherence" / "validate") substring-vouches the bad
	// name. Without this gate, the user receives a mermaid
	// diagram referencing identifiers no source file declares.
	//
	// Default classification: Medium severity, retry-eligible
	// finalizer-only. Operators promote via
	// pipeline_contract_strict_kinds.
	ViolDiagramEdgeEndpointHallucinated ViolationKind = "diagram_edge_endpoint_hallucinated"

	// ViolEnumerationLabelHallucinated (Fix C, s1a-20260507
	// hallucination forensic, 2026-05-07) fires when finalizer's
	// ordered_list / bullet_list / table block carries items whose
	// leading identifier-shape token does NOT resolve to any Tier
	// 1-2 codebase symbol via the typed-graph SymbolOracle.
	//
	// Distinct from ViolEnumerationLabelUngrounded:
	//
	//   - Ungrounded = "explorer evidence pool didn't see this name"
	//     (the label might be a real symbol explorer missed —
	//     recovery: BackToExtract / re-investigation).
	//   - Hallucinated = "the typed graph does not contain this
	//     name at all" (the label is a fabrication — recovery:
	//     finalizer re-emit with a name that exists).
	//
	// Detected case (s1a r1, 2026-05-07): finalizer fabricated
	// `checkNamingConvention` and `checkSignalSufficiency` for items
	// 8-9 of the gate.Run check enumeration; the real names at
	// gate.go:171/172 are `checkCriterionResolvable` and
	// `checkPendingFieldsWellformed`. Existing ungrounded oracle
	// caught only c8 because evidence pool prose contained "signal"
	// substring vouching for c9. The graph oracle catches BOTH
	// because neither fabricated name has a Tier 1-2 definition.
	//
	// Default classification: Medium severity, retry-eligible
	// finalizer-only. Operators promote to STRICT via
	// pipeline_contract_strict_kinds when answer-quality regressions
	// are intolerable. Fallback target: BackToFinalize — the
	// extractor's slate is fine; only the finalizer rendered
	// fabricated names.
	ViolEnumerationLabelHallucinated ViolationKind = "enumeration_label_hallucinated"

	// ViolEnumerationItemLabelExtractorDrift (s1a-20260504-130143
	// abstraction-drift forensic, 2026-05-04) fires when finalizer's
	// ordered_list / bullet_list items[i].label values do NOT
	// preserve the verbatim names from the extractor's
	// emit_answer_symbol output. Detected case: extractor emitted
	// 9 verbatim method names (checkCoverage, checkDAGClosure,
	// checkContractComplete, etc.) but finalizer rendered them as
	// abstract placeholders ("check 1 (gate.go:148)", "check 2
	// (gate.go:149)", ...) — every existing oracle (label
	// grounding / facet coverage / claim form) passed because
	// "check" appears in evidence prose, but the user reading the
	// answer cannot recover the real identifiers. Default
	// classification: STRICT — verbatim identifier preservation is
	// the contract between extractor (selects) and finalizer
	// (renders). Fallback target: FinalizerOnly — the extraction
	// signal is intact; only finalizer rendering needs rewrite.
	ViolEnumerationItemLabelExtractorDrift ViolationKind = "enumeration_item_label_extractor_drift"

	// ViolLaneBlockKindMismatch (high-priority gap from the
	// 2026-05-07 lane-discipline audit) fires when a principal
	// AnswerBlock cites evidence whose location is sourced from a
	// support lane whose AllowedBlocks set does not include the
	// block's kind.
	//
	// Example failure mode this gates: the root_cause family's
	// observed_artifact lane is restricted to {summary, caveat} —
	// runtime stack frames are observation facts, not call-chain
	// hops. When the LLM ignores that surface boundary and renders
	// a principal ordered_list whose item citations all map to
	// observed_artifact entries, the answer silently promotes
	// "runtime hit frame X" to "the call chain begins at X". Prior
	// to this oracle the boundary was advisory prompt prose only;
	// upgrading it to a typed validator keeps the lane → block-kind
	// table load-bearing instead of LLM-discretionary.
	//
	// Default classification: STRICT — silently mis-attributing a
	// support lane's content to a forbidden block kind is a hard
	// answer-quality regression that no other oracle catches.
	// Fallback target: FinalizerOnly — re-rendering the same
	// citations under the correct block kind is finalizer-local;
	// upstream stages have already supplied the right evidence.
	ViolLaneBlockKindMismatch ViolationKind = "lane_block_kind_mismatch"

	// ViolWriteCrossSubRepoForbidden fires when write-mode plan / apply
	// emits a ChangePlan that touches files belonging to MORE THAN ONE
	// discovered sub-repo (multigraph topology). Multi-repo write is
	// banned by design §4.5.5 — task.scope=micro forces kind=patch
	// which must lock the worktree to a single sub-repo; violating
	// this contract risks dirtying multiple parent-tree git roots in
	// one apply, which the worktree cleanup defer cannot reliably
	// undo.
	//
	// Fires at plan emission time (write_analyzer / planner stage).
	// Strict by default — there is no graceful degradation; the user
	// MUST split the change into one ChangePlan per sub-repo. Retry
	// hint surfaces "split into separate runs" pointer. Single-repo
	// posture (topology.IsSingle()) trivially never triggers (all
	// files share one slug).
	//
	// Stage = "plan". Layer = "write_contract". SuspectedRoot field
	// = "change_plan.sub_repo_scope".
	ViolWriteCrossSubRepoForbidden ViolationKind = "write_cross_sub_repo_forbidden"

	// ViolDeniedTokenUndeclared fires when the finalize-stage answer
	// prose names a token (file path / symbol identifier) that an
	// upstream typed gate already determined to be unverifiable
	// against the current repository (external-source log frame
	// whose function is absent / perf stall whose symbol is absent /
	// oracle existence miss / drift relocation), AND the answer
	// does NOT explicitly disclose the token as
	// "unverified / external / not in this repository".
	//
	// L3 of the negative-knowledge enforcement pyramid (R3 second-
	// axis): L1 (tool-call gate) prevents the LLM from grounding
	// the token via read_file/grep/repo_map; L2 (prompt sanitiser)
	// strips verbatim mentions from raw input fields before they
	// reach the LLM; L3 (this) catches the case where the LLM still
	// emits the token in answer prose (e.g., quoted from the user's
	// own question or paraphrased from a non-sanitised channel).
	//
	// SOFT-by-default — the answer ships, but the closure ledger
	// records the violation for telemetry / future strict-promote
	// (operators who run heavy attached-log workflows can promote
	// to retry-eligible via pipeline_contract_strict_kinds yaml).
	// Stage="finalize". Layer="answer_validator". Detail names the
	// offending token + the upstream denial class.
	ViolDeniedTokenUndeclared ViolationKind = "denied_token_undeclared"

	// --- Phase 2.B Tier 2 ERM completeness violations
	// (post-finalize answer-aware hard gate, 2026-05-09).
	//
	// These four kinds replace the original Phase 2 advisory-only
	// pre-finalize Tier 2 check with full violation+retry+caveat
	// integration. Each is emitted by a per-dimension answer-aware
	// validator that reads the composed AnswerDocumentV2 (typed
	// blocks first, prose fallback) instead of just the evidence
	// pool. Cross-project and cross-language portable — see
	// docs/design/commercial_grade_3_pattern_remediation.md
	// Phase 2.B for the full rationale, including why
	// LayerDepthValidator was dropped (codrax-specific 3-layer
	// assumption) in favour of the LLM's MissingRequestedRoles
	// typed slot.
	//
	// All four are HARD-by-default: Severity=hard so the FixHint
	// surfaces into the retry directive; bounded retry budget is
	// the existing per-kind machinery (state.retryUsedForKind).

	// ViolScalarCountUnsourced fires when the answer to a count /
	// measurement question (IsCountQuestion=true) surfaces a
	// numeric scalar that is NOT linked to a deterministic counting
	// tool result (exec_command output containing an integer
	// literal). LLM visual-counting from a read_file output produces
	// off-by-one errors on long files / multi-block declarations;
	// this gate forces the count to come from a tool whose output
	// is reproducible.
	ViolScalarCountUnsourced ViolationKind = "scalar_count_unsourced"

	// ViolPathDepthInsufficient fires when a call-chain question
	// (QFCallChain) produces an answer whose distinct function-
	// symbol mentions are fewer than (named_entities + 3). The
	// rule catches the s8a-class failure where the LLM stops at the
	// first 2-3 functions found and misses load-bearing
	// intermediate steps in a longer pipeline. Cross-language —
	// validator detects function symbols regardless of naming
	// convention (CamelCase / snake_case / camelCase / qualified).
	ViolPathDepthInsufficient ViolationKind = "path_depth_insufficient"

	// ViolCardinalityShort fires when an enumeration question
	// (QFEnumeration) carries an explicit declared count
	// (EnumerationBoundary.DeclaredCount > 0) but the answer's
	// enumerated items (BlockBulletList / BlockOrderedList Items
	// totals) come up short. The user stated "the 7 stages" or
	// similar; the answer must surface 7 items or explicitly
	// disclose the count gap.
	ViolCardinalityShort ViolationKind = "cardinality_short"

	// ViolEntityParityImbalanced fires when a comparison question
	// (QFComparison with ≥2 buckets) produces an answer where the
	// per-bucket evidence count is heavily skewed — smallest bucket
	// has fewer than half the evidence of the largest. Heavy skew
	// makes the comparison's weaker side speculative.
	ViolEntityParityImbalanced ViolationKind = "entity_parity_imbalanced"
)

// AllViolationKinds returns every declared ViolationKind in a stable
// enumeration order. Mirrors the AllRepairKinds pattern (see
// internal/types/repair.go) and is consumed by the Session 11
// TestAllViolationKindsHaveProducer structural gate — when a new
// Kind is added here it MUST also be wired to at least one producer
// call site. The enumeration list is the single source of truth for
// kind coverage tooling.
func AllViolationKinds() []ViolationKind {
	return []ViolationKind{
		ViolFamilyMismatch,
		ViolCitation,
		ViolMustInclude,
		ViolMustExclude,
		ViolAcceptance,
		ViolSuccessCriterion,
		ViolGhostAnchor,
		ViolChainDemoted,
		ViolSelfRefLiteral,
		ViolPreCompleteDowngrade,
		ViolLiteralFormFailed,
		ViolViewSwap,
		ViolViewIntentMismatch,
		ViolSubTopicCountMismatch,
		ViolDiagramIdentifier,
		ViolDeclaredCountDrift,
		ViolSelfContradiction,
		ViolExternalArtifactUnderdecoded,
		ViolAuthorityOverreach,
		ViolPlanCritic,
		ViolReflectorObservation,
		ViolAnswerReviewerDistilled,
		ViolIntentTraceShallow,
		ViolIntentEnumerateNotList,
		ViolIntentRootCauseNoCause,
		ViolIntentConfigNoTrail,
		ViolSubjectAnchorMissing,
		ViolPredicateAxisMissing,
		ViolFacetUncovered,
		ViolClaimFormUnsupported,
		ViolAbsenceScopeExceeded,
		ViolMissingRequestedRoleUndisclosed,
		ViolStepIdentifierUnverified,
		ViolRichnessRegression,
		ViolValueSecondaryCitationOffFocus,
		ViolSymbolAnchorMismatch,
		ViolEnumerationLabelUngrounded,
		ViolEnumerationLabelHallucinated,
		ViolInlineIdentifierHallucinated,
		ViolDiagramEdgeEndpointHallucinated,
		ViolEnumerationItemLabelExtractorDrift,
		ViolLaneBlockKindMismatch,
		ViolStructuralEnumerationDivergence,
		ViolCrossCitationConflict,
		// CGEC frequency bridges (R10).
		ViolDemotionStorm,
		ViolForcedReadStorm,
		// V2 block-only carrier validators.
		ViolBlockCoverageMissing,
		ViolPrincipalClaimUseMissing,
		ViolDiagramEdgeUnsupported,
		ViolDiagramEdgeLabelMismatch,
		ViolUncertaintyBlockMissing,
		// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic-
		// quality reviewer thinness / wrong-topic signals.
		ViolAnswerSemanticUnderfilled,
		ViolAnswerTopicMismatch,
		// 修 B (post_v2_runtime_gap_remediation, 2026-05-04) —
		// enumeration evidence underspecification structural gate.
		ViolEnumerationEvidenceUnderspecified,
		// B3 v3 (2026-05-04) — diagram relation typed-first label-only
		// advisory.
		ViolDiagramRelationLabelOnly,
		// B2 v3 (2026-05-04) — richness glaring gap + principal prose
		// underfilled.
		ViolRichnessGlaringGap,
		ViolPrincipalProseUnderfilled,
		// Multi-repo write fail-loud (2026-05-08, design §4.5.5).
		ViolWriteCrossSubRepoForbidden,
		// L3 negative-knowledge answer validator (2026-05-08,
		// TypedDenials Phase F).
		ViolDeniedTokenUndeclared,
		// Phase 2.B Tier 2 ERM completeness (2026-05-09,
		// docs/design/commercial_grade_3_pattern_remediation.md).
		ViolScalarCountUnsourced,
		ViolPathDepthInsufficient,
		ViolCardinalityShort,
		ViolEntityParityImbalanced,
	}
}

// SuspectedRoot is the enforcer's structured self-diagnosis attached
// to every Session-11 ledger write. The F2 aggregator groups events
// by IRField and weights by Confidence so the F3 IRPatchEngine can
// decide whether to reconcile the upstream IR.
//
// IRField uses dotted-path notation matching the mutation API in
// internal/analysis/patcher (e.g. "answer_subject.kind",
// "question_kind", "entity_axes", "EvidencePlan.SourceMix",
// "ScannedSet", "CitationReq", "AcceptanceTests"). The pre-PR2
// patcher allowlist also accepted "answer_shape"; that field is
// retired with the AnswerShape migration. Reason is a ≤ 140-char
// hint the operator can
// skim; Confidence is a [0,1] score calibrated per enforcer (see
// full-design doc §1.4).
//
// A zero-value SuspectedRoot (IRField="" Confidence=0) signals
// "this enforcer did not self-diagnose"; F2 filters such events
// out of aggregation so only classified events drive IR patches.
type SuspectedRoot struct {
	IRField    string  // dotted-path target for F3 patcher
	Reason     string  // ≤ 140 chars
	Confidence float64 // [0, 1]
}

// Violation is one specific contract breach with a short reason and
// an optional repair hint the orchestrator can pass to the explorer
// when it reroutes the task.
//
// The three classic fields (Kind/Detail/Repair) remain the stable
// API. Session 11 F1 added four optional fields so every enforcer
// write can carry (a) which stage produced it, (b) which dispatch ID
// (trace-N-iter-M), (c) which chain / evidence / citation it blames,
// and (d) a structured self-diagnosis the F2 aggregator consumes.
// Legacy callers that only populate Kind/Detail/Repair produce
// zero-confidence events that F2 silently skips, preserving backward
// compat.
type Violation struct {
	Kind   ViolationKind
	Detail string
	Repair string // e.g. "collect evidence for <symbol>"

	// Session 11 F1 extensions — all optional.
	Stage         string        // PipelineStage name: "analyze" / "explore" / "extract" / "finalize"
	DispatchID    string        // trace-N-iter-M or equivalent correlation token
	ClusterKey    string        // producer-side typed repair-cluster identity; preferred over Detail parsing
	EvidenceRefs  []string      // chain_id / evidence_id / citation_idx references
	SuspectedRoot SuspectedRoot // F2-consumed root-cause hypothesis

	// W2.1 (2026-05-05) — root-cause closure axis. Lets the
	// reporter pre-collapse violations that share a root before the
	// retry hint is built so the LLM sees ONE root issue per
	// fingerprint, not 3 derived facets of the same problem.
	//
	// IsDerived = true means this violation describes a SYMPTOM of
	// another violation in the same set; UI / retry hints SHOULD
	// suppress it in favour of its RootKind. The Violation still
	// reaches the closure ledger (telemetry sees it), it's just
	// not surfaced as a separate fix instruction.
	//
	// RootKind names the upstream root-cause ViolKind when the
	// reporter knows it. Empty when this Violation IS its own root
	// (the common case) or when no Implies relation matched.
	//
	// Derivation is computed by ComputeRootCauseClosure (W2.3) per
	// emit batch using ViolKindSpec.Implies (W2.2). The fields are
	// purely advisory — downstream consumers that ignore them get
	// pre-W2.1 behaviour.
	IsDerived bool
	RootKind  ViolationKind
}
