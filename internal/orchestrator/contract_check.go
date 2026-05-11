package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/hint"
	"github.com/hanchaoqun/codrax/internal/authority"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// contract_check.go is the orchestrator-side hook that runs the
// Analyzer-v3 AnswerContract checker after the finalizer produces its
// draft answer and before runTaskGraph marks the finalize node done.
//
// The checker itself lives in internal/analysis/contract; this file
// only handles the orchestrator-facing glue: extracting citations
// from the rendered answer text and translating contract.Result into
// the orchestrator's backtrack signal.
//
// P1.3 design notes:
//
//   - Citations are extracted by a structural file:line regex, NOT
//     a curated list of valid citations. A "valid citation" is any
//     `path/to/file.ext:NNN` token in the answer body. This is a
//     universal Go/Python/JS code-locator format with no repo-
//     specific hardcoding, satisfying the no-stopword guardrail.
//
//   - The checker's own `containsSymbol` helper is a substring match;
//     for must_include / must_exclude, that's intentionally permissive
//     — we want "TaskList" to match "TaskListSize" rather than reject
//     valid superstrings.
//
//   - When AnswerContract is empty (every field zero) the checker
//     short-circuits to Passed=true. So a nil-IR or unwired-shape
//     run never sees a spurious violation.

// runContractCheck runs the AnswerContract validator over a finalizer
// StageOutput and returns the violations slice. nil means no
// violations OR no contract was declared. The caller is expected to
// inspect both the returned slice length and the IR's contract
// presence to decide what to do.
//
// Returns the typed contract.Result so callers can render a
// per-violation diagnostic for the explorer's retry hint.
func runContractCheck(out *agent.StageOutput, c types.AnswerContract, mut *types.MutableState, o *Orchestrator) contract.Result {
	if out == nil {
		return contract.Result{Passed: true}
	}
	draft := contract.Answer{
		Text:      out.FinalAnswer,
		Citations: extractCitationsFromAnswer(out.FinalAnswer),
		IsAbsence: isJustifiedAbsenceAnswer(mut),
	}
	if mut != nil {
		// Read V2 carrier's typed citation pool when present.
		if docV2 := mut.AnswerDocumentV2(); docV2 != nil && len(docV2.Citations) > 0 {
			draft.Citations = make([]contract.Citation, 0, len(docV2.Citations))
			for _, c := range docV2.Citations {
				draft.Citations = append(draft.Citations, contract.Citation{
					File: c.File,
					Line: c.Line,
				})
			}
		}
	}
	// Commit 55 Batch C — wire SymbolOracle through to the contract
	// checker so must_include / must_exclude / acceptance(contains_symbol)
	// substring matches get supplementary oracle validation. nil
	// oracle (no graph) preserves pre-commit-55 behaviour.
	//
	// P4-cross-sub-repo (2026-05-08): when BusContext.MultiGraph is
	// populated (multi_repo_enabled=true), prefer the multigraph
	// fan-out oracle so SymbolExists / SymbolExistsFlat queries hit
	// every active sub-repo. This eliminates the false-positive class
	// where a real symbol defined in a non-routed sub-repo was
	// rejected by hallucination gates (validateInlineIdentifierHallucination
	// / validateDiagramEdgeEndpointHallucination /
	// validateEnumerationItemLabelHallucination), which all consume
	// this same oracle. Single-repo IsSingle() posture is byte-
	// identical because mg.Oracle() forwards through one graph oracle.
	var oracle types.SymbolOracle
	if o != nil && o.busCtx != nil {
		if mg := repomap.MultiGraphFromContext(o.busCtx); mg != nil {
			oracle = mg.Oracle()
		}
	}
	if oracle == nil && mut != nil {
		if g, ok := mut.SearchGraph().(*repotypes.Graph); ok && g != nil {
			oracle = repomap.NewSymbolOracle(g)
		}
	}
	result := contract.CheckWithOracle(draft, c, oracle)

	// Commit 53 P2 — Answer Shape Oracle. After the contract.Check
	// suite, run additional read-mode-only coherence checks that need
	// the full IR (not just the contract). These produce additional
	// ViolationKind values (Intent / Subject / PredicateAxis /
	// SemanticView coherence) that downstream sees as ordinary
	// violations. Soft by default at the gate-layer (see
	// softViolationKinds below); the Append-to-closure path is
	// unchanged.
	//
	// V2 carrier dispatch (the only carrier supported at runtime —
	// AnswerDocumentV2 is the sole emit shape; legacy carriers are
	// retired and rejected at the tool-call boundary). V2 oracle
	// coverage:
	//   - runV2BlockOracles: block coverage / principal claim use /
	//     diagram edge support / uncertainty block / facet coverage /
	//     richness regression / claim form support / absence scope
	//   - runStructuralEnumerationDivergenceOracleV2 + symbol-anchor
	//     mismatch + cross-citation conflict
	// Validator inputs are typed BlockRequirement.FacetIDs /
	// AcceptableClaimForms / SurfaceRoleHint surfaces.
	if mut != nil {

		// V2 block-carrier validators. When a block-only answer has
		// been persisted, AnswerDocumentV2 is non-nil on Mutable; we
		// run the V2 oracle suite against it. Default
		// classification leans SOFT for telemetry-only kinds and
		// STRICT for structural correctness kinds (single source of
		// truth: types.ViolationProfileFor / DeriveSeverity);
		// operators flip individual kinds via
		// pipeline_contract_strict_kinds yaml.
		if docV2 := mut.AnswerDocumentV2(); docV2 != nil && o != nil && o.busCtx != nil {
			view := types.BuildAnswerSemanticViewForBusContext(o.busCtx)
			if view != nil {
				// Oracle-aware variant so validators that opt in
				// (today: validateEnumerationItemLabelExtractorMatch,
				// Fix B 2026-05-07) verify finalizer-emitted labels
				// against the typed graph as a precise fallback when
				// the (noisy) extractor slate misses or typos a real
				// symbol. The same `oracle` constructed above for
				// must_include / must_exclude / acceptance is reused —
				// single truth source for all symbol-existence
				// checks across the contract layer.
				result.Violations = append(result.Violations,
					runV2BlockOraclesWithOracle(docV2, view, mut, oracle)...)
			}
			// validateLaneBlockKindCompliance — typed-signal hard
			// gate on AnswerSupportLane.AllowedBlocks. Compiled here
			// instead of inside runV2BlockOraclesWithMut because the
			// support plan needs the BusContext (RequestModel +
			// AnswerSurfacePlan), not just the Mutable handle.
			// Returns no violations when the active family has no
			// support-lane plan or no lane declares AllowedBlocks.
			if supportPlan := types.BuildAnswerSupportPlanForBusContext(o.busCtx); supportPlan != nil {
				result.Violations = append(result.Violations,
					validateLaneBlockKindCompliance(docV2, supportPlan)...)
			}
			// 修 B (post_v2_runtime_gap_remediation, 2026-05-04) —
			// enumeration evidence pool structural gate. Fires when
			// the user's question states an explicit count N AND the
			// evidence pool has fewer than N distinct named anchors.
			// Routes to BackToExplore (the only honest fix is more
			// evidence, not finalizer rewrite).
			if view != nil {
				result.Violations = append(result.Violations,
					validateEnumerationEvidenceCoverage(mut, view)...)
			}
			// V2 structural-enumeration divergence oracle. Reads typed
			// V2 blocks (block.facet_ids + block.claim_uses) and the
			// graph's ImplementersOf / Subclasses relations.
			if rmFull := mut.RequestModel(); rmFull != nil {
				result.Violations = append(result.Violations,
					runStructuralEnumerationDivergenceOracleV2(docV2, rmFull, mut, o.busCtx)...)
				// B5-T5: V2 adaptation of the symbol-anchor-mismatch
				// oracle. Reads the same per-Run rejection counter
				// + V2 block item counts to detect "explorer never
				// surfaced def-region lines for the user's
				// principal entities".
				if view != nil {
					result.Violations = append(result.Violations,
						runSymbolAnchorTrackOracleV2(docV2, rmFull, view, mut)...)
				}
			}
			// B6-F1 (post-shape consolidated audit, 2026-05-04):
			// cross-citation single-locus oracle. Independent of the
			// AnswerSemanticView — operates purely on V2 block items
			// + citation pool to flag the same symbol cited at
			// conflicting (file, line) tuples.
			result.Violations = append(result.Violations,
				runCrossCitationConflictOracleV2(docV2)...)

			// L3 negative-knowledge answer validator (TypedDenials
			// Phase F, 2026-05-08). Final answer prose must NOT name
			// a denied token (path / symbol that an upstream typed
			// gate marked as unverifiable against the current
			// repository) WITHOUT an explicit unverified disclosure.
			// SOFT — ledger-only by default; promotable via
			// pipeline_contract_strict_kinds yaml.
			if o != nil && o.busCtx != nil {
				result.Violations = append(result.Violations,
					runDeniedTokenAnswerCheck(docV2, &o.busCtx.TypedDenials)...)
			}
		}
	}

	// 2026-05-02 — CritExternalArtifactDecoded oracle. Triggers
	// structurally on (Mutable.LogTriage() != nil OR PerfTrace() !=
	// nil); fails when draft.Text references fewer than the
	// configured floor of bundle-extracted non-path tokens. The
	// rationale lists the missing tokens verbatim so the finalizer
	// retry can directly pick them up and decode them in summary /
	// body. Soft by default (see softViolationKinds in cmd/root.go);
	// operators flip to strict via
	// pipeline_contract_strict_kinds: [external_artifact_underdecoded].
	if mut != nil {
		result.Violations = append(result.Violations,
			runExternalArtifactDecodedCheck(mut, draft.Text)...)
	}
	if o != nil && o.busCtx != nil {
		result.Violations = append(result.Violations,
			runLogErrorMessageLiteralCheck(o.busCtx, draft.Text)...)
	}

	// AuthorityCeiling axis: walk the rendered prose looking for
	// citations whose underlying evidence carries non-factual
	// AuthorityCeiling AND the prose lacks the system-injected
	// hedge sentinel for that ceiling. Triggering here means either
	// (a) the LLM emitted a retry that stripped a previously-applied
	// hedge, or (b) the doc bypassed the renderer. Strict by
	// classification — finalizer retries with hedge repair.
	if mut != nil {
		result.Violations = append(result.Violations,
			runAuthorityOverreachCheck(mut, draft.Text)...)
	}

	// Self-consistency reviewer dispatch. Reviewer reads V2
	// AnswerDocumentV2 — independent LLM detects FACTUAL
	// contradictions between BlockSummary text and the body blocks
	// (BlockOrderedList / BlockBulletList / BlockSection / etc.);
	// gated by reviewer presence, dispatch-eligibility (sufficient
	// summary text + body items), and self-rated confidence floor.
	if o != nil && o.selfConsistencyReviewer != nil && mut != nil {
		if docV2 := mut.AnswerDocumentV2(); docV2 != nil && shouldReviewConsistencyV2(docV2) {
			result.Violations = append(result.Violations,
				o.runSelfConsistencyReviewV2(docV2, mut)...)
		}
	}

	// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
	// quality reviewer. Wired AFTER self-consistency so the reviewer
	// only sees answers that already passed prose-coherence; gated on
	// hard-facet-violation absence so we don't double-noise on
	// already-failing answers.
	if o != nil && o.semanticQualityReviewer != nil && mut != nil {
		if docV2 := mut.AnswerDocumentV2(); docV2 != nil &&
			shouldReviewSemanticQuality(docV2, result.Violations) {
			result.Violations = append(result.Violations,
				o.runSemanticQualityReview(docV2, mut)...)
		}
	}

	// Session 11 F1 ViolationLedger — mirror every violation into the
	// per-Run EvidenceClosure so the F2 aggregator and F4 HintComposer
	// can consume them the same way they consume enforcer-emitted
	// violations. The checker already filled SuspectedRoot on each
	// Violation (see internal/analysis/contract/checker.go), so this
	// is a straight batch append — no per-kind translation needed.
	// Reset Stage/DispatchID here because the checker is decoupled
	// from pipeline plumbing.
	if mut != nil && len(result.Violations) > 0 {
		closure := mut.EvidenceClosure()
		for i := range result.Violations {
			v := result.Violations[i]
			if v.Stage == "" {
				v.Stage = string(types.StageFinalize)
			}
			closure.AppendViolation(v)
		}
	}

	// Commit 53 P3 — soft/strict gate. Recompute Passed against the
	// configured strict-kinds set: if every violation is "soft" per
	// yaml, Passed flips back to true so the scheduler doesn't trigger
	// a hard finalize retry. Mirrored telemetry stays intact (the
	// Append above already happened). Default strict-kinds covers
	// every legacy kind so pre-commit-53 behaviour is byte-identical;
	// only the 3 new kinds (P2/P4) default to soft.
	result.Passed = !hasAnyStrictViolation(result.Violations)

	return result
}

func runExternalArtifactDecodedCheck(mut *types.MutableState, draftText string) []types.Violation {
	if mut == nil {
		return nil
	}
	logBundle := mut.LogTriage()
	perfBundle := mut.PerfTrace()
	if logBundle == nil && perfBundle == nil {
		return nil
	}
	// Strip system-injected authority artifacts before the token-match
	// scan so the hedge body's literal "log" / "perf" / "drift" tokens
	// don't get counted as proof the LLM decoded the bundle. Without
	// this, a drift-bounded answer would auto-pass the gate even when
	// the LLM ignored every Errors[].Type / Frame.Func / Signal in the
	// bundle.
	env := criterion.Env{
		DraftAnswer: render.StripAuthorityArtifacts(draftText),
		LogTriage:   logBundle,
		PerfTrace:   perfBundle,
	}
	res := criterion.Eval(types.Criterion{Kind: types.CritExternalArtifactDecoded}, env)
	if res.Satisfied {
		return nil
	}
	return []types.Violation{{
		Kind:       types.ViolExternalArtifactUnderdecoded,
		Detail:     res.Detail,
		Repair:     res.Detail,
		ClusterKey: "root:external_artifact_decoded",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "external_artifact_decoded",
			Reason:     "answer underdecodes triaged log / perf trace",
			Confidence: 0.7,
		},
		Stage: string(types.StageFinalize),
	}}
}

func runLogErrorMessageLiteralCheck(ctx *types.BusContext, draftText string) []types.Violation {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	bundle := ctx.Mutable.LogTriage()
	if bundle == nil || !shouldRequireLogErrorMessageLiterals(ctx) {
		return nil
	}
	answer := normalizeRuntimeMessageLiteral(draftText)
	var out []types.Violation
	for _, msg := range requiredLogErrorMessagesForContract(bundle) {
		if msg == "" {
			continue
		}
		if strings.Contains(answer, normalizeRuntimeMessageLiteral(msg)) {
			continue
		}
		out = append(out, types.Violation{
			Kind:       types.ViolMustInclude,
			Detail:     fmt.Sprintf("required runtime error message %q missing from answer", msg),
			Repair:     fmt.Sprintf("preserve the runtime error message %q verbatim in the summary or body; do not translate it away", msg),
			ClusterKey: types.IdentityClusterKey("runtime_error_message:"+msg, "must_include"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "log_error_message",
				Reason:     "diagnostic artifact answer omitted original runtime error wording",
				Confidence: 0.85,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

func shouldRequireLogErrorMessageLiterals(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.AnalysisIR != nil {
		return ctx.AnalysisIR.RequestModel.RequiresDiagnosticArtifactMessageSurface()
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.RequiresDiagnosticArtifactMessageSurface()
		}
	}
	return false
}

func requiredLogErrorMessagesForContract(bundle *types.LogBundle) []string {
	messages := types.LogBundleErrorMessages(bundle)
	if len(messages) > 4 {
		messages = messages[:4]
	}
	return messages
}

func normalizeRuntimeMessageLiteral(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// runAuthorityOverreachCheck guards against rendered drafts that
// LACK the system's authority signaling on log-derived / drift-
// bounded evidence. ApplyAuthorityHedging unconditionally injects
// the doc-level "Authority: " caveat into doc.Caveats[] whenever ANY
// emitted evidence is non-factual; the caveat travels through
// RenderAnswerDocument into the user-visible prose.
//
// The gate FIRES only when:
//   - cited evidence requires hedging (any conditional / historical /
//     illustrative in the cited anchor set), AND
//   - the rendered draft text DOES NOT carry the doc-level Authority
//     caveat (the system's primary user-visible hedging signal).
//
// In other words, this check catches drafts that bypassed the
// renderer entirely (the doc.IsZero raw-prose fallback in
// answer_document_evaluator) or drafts where some downstream
// transformation stripped the caveat. The normal render path always
// injects the caveat, so this check is silent on the dominant
// success path — we don't burn finalizer retry budget on a strict
// gate that fires on every drift-bounded answer.
//
// Strict by classification (cmd/root.go); the actionable retry hint
// is "the system's hedging signal didn't reach the user — re-render
// via the documented path".
func runAuthorityOverreachCheck(mut *types.MutableState, draftText string) []types.Violation {
	if mut == nil {
		return nil
	}
	// B8-T4: read V2 carrier's typed citations.
	docV2 := mut.AnswerDocumentV2()
	if docV2 == nil || len(docV2.Citations) == 0 {
		return nil
	}
	// Post block-only migration, the authority caveat is injected at
	// render time onto a defensive clone of the V2 carrier and then
	// stripped from the user-facing surface. A successfully structured
	// V2 doc therefore does NOT need to persist the private caveat tag
	// on Mutable itself. Keep this check dormant on the normal
	// structured-doc path; it only existed to catch renderer bypass /
	// raw-prose fallback before render-time caveat synthesis became the
	// source of truth.
	if len(docV2.Blocks) > 0 {
		return nil
	}
	evidence := mut.EmittedEvidence()
	if len(evidence) == 0 {
		return nil
	}

	// Any cited anchor whose strongest underlying evidence is non-
	// factual REQUIRES the doc-level caveat to surface. We don't
	// gate on per-step / per-symbol sentinels anymore — those are
	// inline annotations for readability, not the contract anchor.
	requiresHedging := false
	for _, cit := range docV2.Citations {
		ceiling := authority.HighestAuthorityFor(evidence, cit.File, cit.Line)
		switch ceiling {
		case types.AuthorityConditional, types.AuthorityHistorical, types.AuthorityIllustrative:
			requiresHedging = true
		}
		if requiresHedging {
			break
		}
	}
	if !requiresHedging {
		return nil
	}

	// The authoritative signal lives in the structured V2 caveat
	// block, not in the user-visible rendered prose. The renderer is
	// free to strip the private tag from the final UI surface as long
	// as the structured carrier still contains the system-generated
	// caveat block.
	if docV2CarriesAuthorityCaveat(docV2) {
		return nil
	}

	// Caveat absent and hedging required: the renderer was bypassed
	// (doc.IsZero raw-prose fallback) or some post-render mutation
	// stripped the signal. Strict violation — finalizer must
	// re-emit through the structured channel. Wording uses the
	// canonical user-facing term "drift-bounded" everywhere
	// (matches skill prompt + caveat + render docs); avoids
	// internal plumbing names (ApplyAuthorityHedging, doc.Caveats)
	// the LLM cannot act on.
	detail := "answer cites drift-bounded evidence (the underlying observations were derived from the attached log/perf trace and the current code has changed since) but the answer's bottom-of-page Authority disclosure is missing. This usually means the prior emit_answer_document call failed schema validation and the system fell back to raw prose."
	return []types.Violation{{
		Kind:   types.ViolAuthorityOverreach,
		Detail: detail,
		// Repair speaks in LLM-actionable terms: name the tool, name
		// the cause class, name the fix. Don't reference internal
		// component names ("ApplyAuthorityHedging", "render path")
		// the LLM has no schema for.
		Repair:     "Re-emit emit_answer_document with all required block fields populated (see the skill's Required-Block dispatch table). The system will re-attach the drift disclosure automatically once the structured emit succeeds — do not write the disclosure prose yourself.",
		ClusterKey: "root:answer_authority",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_authority",
			Reason:     "drift disclosure missing on rendered answer (likely structured-emit failure)",
			Confidence: 0.6,
		},
		Stage: string(types.StageFinalize),
	}}
}

func docV2CarriesAuthorityCaveat(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, blk := range doc.Blocks {
		if blk.Kind != types.BlockCaveat {
			continue
		}
		if strings.Contains(blk.Text, render.AuthorityCaveatTag()) {
			return true
		}
	}
	return false
}

// isPlumbingFailureViolation reports whether a ViolationKind
// represents a pipeline / plumbing failure (renderer bypass,
// structured-emit rejection, etc.) rather than an answer-content
// quality issue. These kinds are excluded from answer_reviewer
// pattern collection: feeding them to the cross-Run learning loop
// pollutes the analyzer's "Known answer pitfalls" section with
// plumbing noise the next Run's LLM has no actionable handle on,
// distracting from real answer-quality lessons.
//
// Update this list when adding ViolationKinds whose Detail/Reason
// describes pipeline state rather than answer prose state.
func isPlumbingFailureViolation(k types.ViolationKind) bool {
	switch k {
	case types.ViolAuthorityOverreach:
		// Fires only when the renderer was bypassed (IsZero raw-
		// prose fallback) — not an answer-quality issue.
		return true
	case types.ViolPlanCritic, types.ViolReflectorObservation:
		// Block 1 reviewer-side observational signals (commit
		// 2f76dac). These are operational reviewer outputs the
		// system uses internally — they describe "the
		// reviewer LLM had something to say" not "the answer
		// has a quality bug". Letting them flow to the
		// answer_taxonomy cross-Run learning pool would distract
		// future Runs' analyzer with reviewer plumbing noise.
		// ViolAnswerReviewerDistilled is deliberately NOT here
		// — it IS an answer-quality lesson (the reviewer's whole
		// purpose is to surface those for the taxonomy).
		return true
	}
	return false
}

// runIntentCoverageOracle (Block 2 architecture overhaul 2026-05-02)
// validates that the rendered AnswerDocument structurally satisfies
// the depth / shape / breadth implied by rm.Intent. Each Intent gets
// its own coverage rule:
//
//   - IntentTrace: ≥2 hops in steps[] OR a sequenceDiagram fenced
//     block in summary. A trace answer with one bullet failed to
//     walk the chain.
//   - IntentEnumerate: shape=list_of_symbols OR shape=step_list with
//     ≥2 items. Enumeration questions need a list-shaped surface.
//   - IntentRootCause: ≥1 citation. Root-cause needs at least one
//     grounded site; an unanchored hand-wave explanation is wrong.
//   - IntentConfigQuery: shape=value/config_value OR ≥1 citation
//     pointing at a config-shaped path. The canonical config-query
//     answer surfaces a literal or a config citation.
//
// Inputs: only IR fields (Intent, Shape) and AnswerDocument structure.
// No question-text matching, no system-derived Scenario reads. Each
// Intent's rule is a STRUCTURAL invariant on the LLM's emit.
//
// traceAnswerHasDepth reports whether a trace-class answer surface
// has structural depth — either >=2 PRINCIPAL step hops or a
// sequenceDiagram fenced block in summary.
//
// enumerateAnswerHasList reports whether an enumeration answer
// renders as a list surface.
//
// boolToHasOrNot is a tiny presentation helper for the
// IntentTraceShallow Detail string. Renders:
//
//	hasSequenceDiagram(summary) → "a sequenceDiagram block"
//	!hasSequenceDiagram(summary) → "no sequenceDiagram block"
//
// runSubjectAnchorOracle (Block 2 — 2026-05-02) validates that the
// rendered AnswerDocument exposes the analyzer's declared
// AnswerSubject. When rm.AnswerSubject.Kind names a concrete
// surface ({SubjectFunctionName, SubjectTypeName, SubjectHandlerRoute,
// SubjectConfigKey, SubjectStructField, SubjectInterface}) at
// confidence ≥ subjectFloor, the answer must surface that subject:
//
//   - via doc.Symbols[i].Name (any item)
//   - via doc.Steps[i].Anchor (any step)
//   - via inline backtick code in summary or any step description
//
// subjectAnchorOracleFloor is the AnswerSubject.Confidence floor
// below which the SubjectAnchor oracle abstains. Mirrors the
// gate's coherenceSubjectConfidenceFloor (0.6) used for shape /
// subject coherence — same noise-suppression rationale.
const subjectAnchorOracleFloor = 0.6

// anyEntityAxisOnAnswerSurface reports whether any entity in axes
// appears verbatim (case-sensitive) in:
//
//   - doc.Symbols[i].Name (any item)
//   - any step.Description's inline backtick `code`
//   - doc.Summary's inline backtick `code`
//
// AnswerStep does not carry an explicit Anchor field today (Index +
// Description + CitationRef only); the LLM-emitted load-bearing
// identifier per step lives inline in the Description prose, which
// is what extractInlineCodeTokens walks.
//
// runPredicateAxisOracle (Block 2 — 2026-05-02) validates that the
// closure carries at least one EvidenceItem whose AnchorKind belongs
// to the LLM-declared PredicateAxis's allowed-kind set (per
// types/axis_anchor_map.go::PredicateAxisToAnchorKinds). When
// PredicateAxis is set but no evidence has the matching shape, the
// answer is grounded in the wrong axis — e.g. the LLM said the
// question is "trace this call" (AxisCall) but every evidence item
// carries AnchorDefinition (the function bodies, not the call sites).
//
// Skipped when:
//   - rm.PredicateAxis == AxisUnknown (LLM didn't pin an axis)
//   - the closure has no evidence with a non-empty AnchorKind
//     (nothing to check; up-stream gates handle the empty case)
//
// symbolAnchorMismatchThreshold is the rejection floor at which
// runSymbolAnchorTrackOracle escalates from telemetry to a fallback
// signal. Three rejections in one Run means the LLM emitted three
// items that all failed line-anchor verification — strong evidence
// the explorer never read the def-region of the user's principal
// entities (the verifier is a precise per-line check; rejection on
// three independently-emitted items cannot be explained by a single
// off-by-one slip).
const symbolAnchorMismatchThreshold = 3

// runSymbolAnchorTrackOracle (P1 #3, 2026-05-03) reads the per-Run
// emit_answer_symbol rejection counter from the closure and emits
// ViolSymbolAnchorMismatch when (a) the rejection count crossed
// symbolAnchorMismatchThreshold AND (b) the rendered AnswerDocument
// has fewer Symbols than the analyzer-declared count for the
// question (or zero Symbols when the answer shape is list_of_symbols
// without a declared count). The signal is precise — it derives from
// a deterministic per-line verifier inside the tool, NOT from
// keyword scanning or fuzzy ranker output. Per the
// precise-signals-for-hard-gates red line this oracle therefore
// CAN drive a structural fallback decision; default classification
// remains SOFT (telemetry first, operators promote via
// pipeline_contract_strict_kinds for repos where it should
// auto-trigger BackToExplore).
//
// runSymbolAnchorTrackOracleV2 reads V2 blocks to compute the
// principal-anchor count and compares against the family's
// expected count.
//
// Counting:
//   - "Got symbols" = sum of items[] across BlockOrderedList,
//     BlockBulletList, BlockTable (each item is one principal
//     anchor).
//   - "Got steps" = items[] count of BlockOrderedList specifically.
//
// "Required shape" is derived from view.Family + the family's
// primary required block kind.
func runSymbolAnchorTrackOracleV2(docV2 *types.AnswerDocumentV2, rm *types.RequestModel, view *types.AnswerSemanticView, mut *types.MutableState) []types.Violation {
	if docV2 == nil || rm == nil || mut == nil {
		return nil
	}
	closure := mut.EvidenceClosure()
	if closure == nil {
		return nil
	}
	rejections := closure.SymbolEmitRejections()
	if rejections < symbolAnchorMismatchThreshold {
		return nil
	}
	// V2 family eligibility — only enumeration-shaped or principal-
	// list families need an anchor count check. Generic / role-
	// lookup / config-precedence answers don't carry an enumerated
	// list of symbols.
	if view == nil {
		return nil
	}
	switch view.Family {
	case types.QFEnumeration, types.QFCallChain, types.QFRootCauseTrace:
		// supported
	default:
		return nil
	}
	expected := 0
	if b := rm.EnumerationBoundary; b != nil && b.DeclaredCount > 0 {
		expected = b.DeclaredCount
	}
	gotSymbols := 0
	gotSteps := 0
	for _, blk := range docV2.Blocks {
		switch blk.Kind {
		case types.BlockOrderedList:
			n := len(blk.Items)
			gotSteps += n
			gotSymbols += n
		case types.BlockBulletList, types.BlockTable:
			gotSymbols += len(blk.Items)
		}
	}
	switch {
	case expected > 0 && gotSymbols >= expected:
		return nil
	case expected == 0 && view.Family == types.QFEnumeration && gotSymbols == 0:
		// fire
	case expected == 0 && view.Family == types.QFCallChain && gotSteps == 0:
		// fire
	case expected == 0 && view.Family == types.QFRootCauseTrace && gotSymbols == 0:
		// fire
	case expected > 0 && gotSymbols < expected:
		// fire
	default:
		return nil
	}
	return []types.Violation{{
		Kind: types.ViolSymbolAnchorMismatch,
		Detail: fmt.Sprintf(
			"emit_answer_symbol rejected %d items on line-anchor mismatch this Run; the rendered V2 answer carries %d enumerated item(s) for family=%s (declared count=%d). The rejections cluster around the def-line verifier.",
			rejections, gotSymbols, view.Family, expected),
		Repair:     "the next investigation pass should re-explore around the principal entities the rejected items pointed to, with read_file slices that cover each entity's definition line.",
		ClusterKey: familyClusterKey(view.Family, "explorer_def_region_coverage_v2"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "explorer_def_region_coverage_v2",
			Reason:     "repeated emit_answer_symbol rejections at line-anchor verifier (V2 carrier)",
			Confidence: 0.7,
		},
		Stage: string(types.StageExtract),
	}}
}

// crossCitationLineTolerance is the max line distance two cites may
// differ by while still being considered the same locus. Two cites
// at consecutive lines of a function body (signature + first body
// line) are not a conflict; cites that drift more than this are.
const crossCitationLineTolerance = 2

// runCrossCitationConflictOracleV2 (B6-F1, 2026-05-04;
// R2.4 selection-fix 2026-05-04) walks the V2 AnswerDocument's
// symbol-emitting items and flags symbols cited at more than one
// (file, line) locus. The single-locus rule: when the SAME
// underlying symbol is named in multiple block items + their
// CitationRef-resolved Citations point at different files OR
// drift >crossCitationLineTolerance lines apart, at most one cite
// can be the actual locus of the claim about that symbol — the
// others reference fragments that mention it without being its
// definition site. Real case (m1a-): one cite extractor.go:114 +
// another extractor.go:135 both labelled "the Turn-B handoff
// function"; user cannot tell which is the actual handoff.
//
// R2.4 fix (2026-05-04): the original implementation used
// item.Label as the symbol identity key, but Label is rendered
// PROSE (e.g. "checkCoverage — 覆盖度检查") which is wordy
// + nearly always unique per item even when the underlying symbol
// is the same. The fix prefers structured signals in priority
// order:
//
//  1. RenderedClaimUse.EvidenceID — precise typed signal: same
//     EvidenceID = same evidence item = canonical identity. Two
//     items with same EvidenceID but different Citations is the
//     direct contradiction shape we're catching.
//  2. item.ID — when LLM populates IDs (rarely in practice).
//  3. Label first 32 chars as a last-resort heuristic, gated by
//     a minimum-shape filter: token must be a single CamelCase /
//     snake_case identifier (no whitespace, no separator). This
//     skips wordy prose Labels like "checkCoverage — 覆盖度检查"
//     while still catching short verbatim-symbol Labels like
//     "TurnAArtifacts".
//
// Detection signals (precise — R2 red line):
//   - V2 RenderedClaimUse.EvidenceID (typed evidence handle), OR
//   - V2 AnswerBlockItem.ID (typed item handle), OR
//   - V2 AnswerBlockItem.Label (only when matches identifier shape).
//   - V2 docV2.Citations[item.CitationRef].File / Line (typed
//     resolution; CitationRef==-1 / out-of-range items skip).
//
// Skipped when:
//   - docV2 == nil OR no Citations in pool.
//   - Every symbol identity appears at most once in items[].
//
// Stage="finalize". SOFT-by-default (operators promote via
// pipeline_contract_strict_kinds when they want auto-rewrite).
func runCrossCitationConflictOracleV2(docV2 *types.AnswerDocumentV2) []types.Violation {
	if docV2 == nil || len(docV2.Blocks) == 0 || len(docV2.Citations) == 0 {
		return nil
	}
	type locus struct {
		file string
		line int
	}
	loci := make(map[string][]locus, 8)
	for _, blk := range docV2.Blocks {
		for _, item := range blk.Items {
			id := crossCitationIdentityKey(item)
			if id == "" {
				continue
			}
			ref := item.CitationRef
			if ref < 0 || ref >= len(docV2.Citations) {
				continue
			}
			cit := docV2.Citations[ref]
			if cit.File == "" {
				continue
			}
			loci[id] = append(loci[id], locus{file: cit.File, line: cit.Line})
		}
	}
	if len(loci) == 0 {
		return nil
	}
	var violations []types.Violation
	for name, lst := range loci {
		if len(lst) < 2 {
			continue
		}
		// Conflict iff any pair disagrees on file OR drifts >
		// crossCitationLineTolerance lines apart. Walk from index 0
		// outward; first disagreement records the violation and
		// breaks (one violation per symbol — multi-pair conflicts
		// for the same symbol surface as a single repair task).
		anchor := lst[0]
		for _, other := range lst[1:] {
			if other.file != anchor.file ||
				abs(other.line-anchor.line) > crossCitationLineTolerance {
				violations = append(violations, types.Violation{
					Kind: types.ViolCrossCitationConflict,
					Detail: fmt.Sprintf(
						"symbol %q cited at conflicting loci: %s:%d vs %s:%d",
						name, anchor.file, anchor.line, other.file, other.line),
					Repair: fmt.Sprintf(
						"resolve %q to a single canonical locus and re-emit emit_answer_document so every block item naming %q points at the same (file, line) cite. If two distinct sites legitimately co-exist (e.g. interface decl + impl), surface them as separate items with distinct labels.",
						name, name),
					ClusterKey: identityClusterKey(name, "answer_citations_locus"),
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "answer_citations_locus",
						Reason:     "single-locus consistency violated for cited symbol",
						Confidence: 0.7,
					},
					Stage: string(types.StageFinalize),
				})
				break
			}
		}
	}
	return violations
}

// crossCitationIdentityKey returns the symbol-identity grouping
// key for an AnswerBlockItem (R2.4 fix, 2026-05-04). Priority:
//
//  1. ClaimUse.EvidenceID — typed evidence handle (most precise).
//  2. item.ID — typed item handle, when LLM populated it.
//  3. item.Label — only if Label has identifier shape (single
//     CamelCase / snake_case token, ≤ 32 chars, no whitespace),
//     trimmed to first 32 chars to bound the key length.
//
// Returns "" when no signal is usable; caller skips the item so
// the oracle never groups by wordy prose.
func crossCitationIdentityKey(item types.AnswerBlockItem) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return "id:" + id
	}
	label := strings.TrimSpace(item.Label)
	if label == "" {
		return ""
	}
	if !looksLikeIdentifierShape(label) {
		return ""
	}
	if len([]rune(label)) > 32 {
		return ""
	}
	return "label:" + label
}

// looksLikeIdentifierShape reports whether s looks like a single
// CamelCase / snake_case identifier token. Whitespace, punctuation
// (other than `_`/`.`/`-` for dotted/dashed identifiers), and
// non-ASCII letters disqualify it. The check is structural only
// — no keyword table.
func looksLikeIdentifierShape(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == '-':
		default:
			return false
		}
	}
	return true
}

// runStructuralEnumerationDivergenceOracle (P3 #6 precise variant,
// 2026-05-03) catches the specific transparency failure where the
// answer's emitted set ⊊ Graph.ImplementersOf(IfaceName) AND the
// missing names are silent — neither emitted as caveat rows nor
// named verbatim in summary. The oracle's purpose is NOT to force
// the LLM to include every structural implementer (the LLM may
// have a legitimate prose-side reason to exclude one); it is to
// require that any divergence be DESCRIBED so the user sees both
// signals (typed code relation + author-narrative reasoning) and
// can judge.
//
// Three precise signals (zero keyword tables / ranker scores):
//   - typed: g.ImplementersOf(entityName) — deterministic method-set match.
//   - emitted: doc.Symbols[i].Name — exact verbatim string set.
//   - narrative: strings.Contains(doc.Summary, name) — verbatim substring.
//
// Skip when:
//   - rm.Predicates.IsCategoryEnumeration is false (not an
//     enumeration question)
//   - no analyzer entity resolves to an interface / trait / protocol
//     (Graph.ImplementersOf returns 0 for all entities)
//   - emitted set ⊇ typed set (no divergence)
//   - every missing name is mentioned verbatim in doc.Summary
//     (the LLM acknowledged the divergence in prose)
//
// runStructuralEnumerationDivergenceOracleV2 is the V2 carrier
// adaptation of runStructuralEnumerationDivergenceOracle (B5-T4
// 落地). Reads the same typed Graph.Implements relation but pulls
// emitted names from V2 blocks instead of V1's doc.Symbols and
// builds the summary haystack from BlockSummary text.
//
// Emitted-name set composition (typed signal only — R2 red line):
//   - All BlockOrderedList / BlockBulletList / BlockTable items'
//     Label fields (verbatim, no normalisation).
//   - All AnswerBlockItem.Text leading words when the item kind
//     conventionally carries the name there (Scalar / Decision).
//
// Summary haystack: concatenation of every BlockSummary.Text +
// any block.Title strings (verbatim substring match per R2 red
// line — no fuzzy matching).
func runStructuralEnumerationDivergenceOracleV2(docV2 *types.AnswerDocumentV2, rm *types.RequestModel, mut *types.MutableState, busCtx *types.BusContext) []types.Violation {
	if docV2 == nil || rm == nil || mut == nil {
		return nil
	}
	if !rm.Predicates.IsCategoryEnumeration {
		return nil
	}
	candidateEntities := append([]string(nil), rm.AnalyzerHints.PrimaryEntities...)
	candidateEntities = append(candidateEntities, rm.AnalyzerHints.Entities...)
	var ifaceName string
	var typedImpl []*repotypes.Symbol

	// P4-cross-sub-repo (Sc 1, 2026-05-08): when MultiGraph is wired,
	// scan implementers across every active sub-repo. The interface
	// declaration may live in sub-a while implementers span sub-b/c —
	// without fan-out the divergence oracle would silently agree the
	// emitted set ⊇ typed set when sub-b/c implementers were never
	// observed.
	if mg := repomap.MultiGraphFromContext(busCtx); mg != nil {
		for _, ent := range candidateEntities {
			ent = strings.TrimSpace(ent)
			if ent == "" {
				continue
			}
			hits := mg.ImplementersOf(ent)
			if len(hits) == 0 {
				continue
			}
			ifaceName = ent
			for _, hit := range hits {
				sym, _, ok := mg.LookupSymbolByID(hit.ID)
				if ok && sym != nil {
					typedImpl = append(typedImpl, sym)
				}
			}
			break
		}
		if len(typedImpl) == 0 {
			return nil
		}
		return enumerateStructuralDivergence(typedImpl, ifaceName, v2EmittedNameSet(docV2), v2SummaryHaystack(docV2))
	}

	graph, ok := mut.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil {
		return nil
	}
	for _, ent := range candidateEntities {
		ent = strings.TrimSpace(ent)
		if ent == "" {
			continue
		}
		ids := graph.ImplementersOf(ent)
		if len(ids) == 0 {
			continue
		}
		ifaceName = ent
		for _, id := range ids {
			if sym, ok := graph.SymbolByID[id]; ok && sym != nil {
				typedImpl = append(typedImpl, sym)
			}
		}
		break
	}
	if len(typedImpl) == 0 {
		return nil
	}
	return enumerateStructuralDivergence(typedImpl, ifaceName, v2EmittedNameSet(docV2), v2SummaryHaystack(docV2))
}

// mapV1Names converts a V1 AnswerSymbol slice into a name set the
// shared enumerateStructuralDivergence consumer expects.
func mapV1Names(syms []types.AnswerSymbol) map[string]bool {
	out := make(map[string]bool, len(syms))
	for _, s := range syms {
		if name := strings.TrimSpace(s.Name); name != "" {
			out[name] = true
		}
	}
	return out
}

// v2EmittedNameSet builds the emitted-name set from a V2 doc by
// walking enumeration-shaped blocks (OrderedList / BulletList /
// Table) and collecting Label values verbatim. Scalar / Decision
// blocks contribute their Text as a single name when non-empty.
func v2EmittedNameSet(doc *types.AnswerDocumentV2) map[string]bool {
	out := make(map[string]bool)
	for _, blk := range doc.Blocks {
		switch blk.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
			for _, it := range blk.Items {
				if name := strings.TrimSpace(it.Label); name != "" {
					out[name] = true
				}
			}
		case types.BlockScalar, types.BlockDecision:
			if name := strings.TrimSpace(blk.Text); name != "" {
				out[name] = true
			}
			for _, it := range blk.Items {
				if name := strings.TrimSpace(it.Label); name != "" {
					out[name] = true
				}
			}
		}
	}
	return out
}

// v2SummaryHaystack builds the substring-match haystack from a V2
// doc by concatenating every BlockSummary's Text + any block's
// Title field. The substring check in
// enumerateStructuralDivergence then runs verbatim against this
// haystack (strings.Contains).
func v2SummaryHaystack(doc *types.AnswerDocumentV2) string {
	var b strings.Builder
	for _, blk := range doc.Blocks {
		if blk.Kind == types.BlockSummary {
			if t := strings.TrimSpace(blk.Text); t != "" {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		}
		if title := strings.TrimSpace(blk.Title); title != "" {
			b.WriteString(title)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// enumerateStructuralDivergence is the shared core that both V1 and
// V2 oracles dispatch into. It computes which typed implementers
// the answer omitted (relative to emittedNames + summary haystack)
// and returns the canonical ViolStructuralEnumerationDivergence
// violation when divergence is silent.
func enumerateStructuralDivergence(typedImpl []*repotypes.Symbol, ifaceName string, emittedNames map[string]bool, summary string) []types.Violation {
	type missingEntry struct {
		name string
		file string
	}
	var missing []missingEntry
	for _, t := range typedImpl {
		if t == nil || t.Name == "" {
			continue
		}
		if emittedNames[t.Name] {
			continue
		}
		if strings.Contains(summary, t.Name) {
			continue
		}
		missing = append(missing, missingEntry{name: t.Name, file: t.File})
	}
	if len(missing) == 0 {
		return nil
	}
	names := make([]string, 0, len(missing))
	for _, m := range missing {
		if m.file != "" {
			names = append(names, fmt.Sprintf("%s (%s)", m.name, m.file))
		} else {
			names = append(names, m.name)
		}
	}
	return []types.Violation{{
		Kind: types.ViolStructuralEnumerationDivergence,
		Detail: fmt.Sprintf(
			"typed Symbol.Implements relation reports %d concrete implementers of %q; the rendered answer omits %d of them WITHOUT naming them in summary or marking them as caveats: %v",
			len(typedImpl), ifaceName, len(missing), names),
		Repair: fmt.Sprintf(
			"the typed code relation and your prose reasoning have diverged. Both are valid signals — typed = the method set structurally satisfies %q; prose = an author comment / narrative cue made you exclude these. Pick ONE of: (a) re-emit emit_answer_symbol with each omitted item included AND a rationale starting with \"[caveat]\" naming the disagreement (e.g. \"[caveat] method set satisfies %s but author comment at file:line excludes from semantic implementation\"); (b) re-emit emit_answer_document with the omitted names appearing verbatim in summary as exception cases (e.g. \"X also has the method set but author note excludes it because Y\"). Do NOT silently drop members — the user receives a partial set without knowing items were filtered.",
			ifaceName, ifaceName),
		ClusterKey: symbolClusterKey(ifaceName, "structural_enumeration_divergence"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "structural_enumeration_divergence",
			Reason:     "typed implementers vs emitted answer set diverge silently",
			Confidence: 0.85,
		},
		Stage: string(types.StageFinalize),
	}}
}

// FacetValidatorsEnabled returns whether the Phase 4 facet
// validators (FacetCoverage / ClaimForm / Absence-Scope) are
// active. Default true; flip to false via cmd/root.go's
// SetFacetValidatorsEnabled when codrax.yaml sets
// pipeline_facet_validators_enabled: false.
func FacetValidatorsEnabled() bool {
	facetValidatorsEnabledMu.RLock()
	defer facetValidatorsEnabledMu.RUnlock()
	return facetValidatorsEnabled
}

// SetFacetValidatorsEnabled flips the master switch. Called once
// from cmd/root.go at startup after codrax.yaml merge.
func SetFacetValidatorsEnabled(on bool) {
	facetValidatorsEnabledMu.Lock()
	defer facetValidatorsEnabledMu.Unlock()
	facetValidatorsEnabled = on
}

var (
	facetValidatorsEnabledMu sync.RWMutex
	facetValidatorsEnabled   = true
)

// runFacetCoverageOracle (Phase 4 of Semantic Surface Contract,
// 2026-05-02) walks the FacetCoverageContract.Required entries and
// flags any FacetHardRequired facet that the rendered AnswerDocument
// fails to cover. "Coverage" means at least one payload (step / symbol
// / value / boolean) carries either:
//
//   - An explicit RenderedClaimUse with FacetID matching the facet's
//     Kind — the LLM self-annotated the slot it filled.
//   - A citation whose underlying evidence has a ClaimFormOf result
//     in the facet's AcceptableForms — inferred coverage when the
//     LLM omitted ClaimUse but the evidence shape implies the facet.
//
// Phase 1's HARD-degrade-to-SOFT pre-compile rule guarantees that any
// facet remaining FacetHardRequired here had at least one bound
// SourceCandidate at compile time — so a "no coverage" verdict is
// not noise, it means the LLM had evidence available but did not
// surface it in the rendered answer.
//
// runRichnessTelemetryOracle (Phase 5 of Semantic Surface Contract,
// 2026-05-02) walks the FacetCoverageContract.Optional entries
// (TierEnrichment) and records SOFT ViolRichnessRegression for any
// optional facet the rendered AnswerDocument fails to cover.
// Coverage check is identical to runFacetCoverageOracle's: an
// explicit RenderedClaimUse.FacetID match OR an inferred-from-citation
// ClaimForm match against the facet's AcceptableForms.
//
// Phase 5 contract: TELEMETRY ONLY. The kind is permanently SOFT
// (defaultSoftKinds, never promotable to STRICT) and the fallback
// policy maps it to FailLoud as a safety net so an accidental
// promotion does not silently spin a finalize retry. The oracle's
// purpose is to feed end-of-Run [CGEC] summary's
// optional_facets_covered=N/M counter so cross-Run trend tracking
// shows whether prompt improvements lift richness coverage over
// time.
//
// runClaimFormSupportOracle (Phase 4, 2026-05-02) checks that every
// payload's explicit RenderedClaimUse.ClaimForm is supported by the
// underlying evidence's actual ClaimForm projection. If LLM emitted
// claim_form=call_edge but the cited evidence has AnchorKind=Definition
// (ClaimDefinitionFact), the annotation is internally inconsistent.
//
// Default classification: STRICT — explicit LLM self-contradiction
// the finalizer can fix without new evidence (FinalizerOnly).
//
// Empty ClaimUse / nil ClaimUse / empty ClaimForm → skipped (no
// annotation, nothing to validate). Phase 3's emit_answer_document
// validateClaimUse already rejects unknown enum values; this oracle
// validates SEMANTIC consistency between annotation and evidence.
// runStepIdentifierBackedByEvidenceOracle (Phase 4 extension,
// 2026-05-02) walks each AnswerStep's prose, extracts every
// backtick-quoted identifier, and verifies that each appears in
// the typed evidence pool's structural fields (Subject / Object /
// AnchorSymbol on EvidenceItem, plus AnswerSymbol.Name on the
// answer-symbol slate). Identifiers that don't appear in any of
// those typed fields are flagged as unverified — the explorer's
// emit_evidence never structurally captured them, so the LLM is
// either hallucinating the name or remembered a position
// (file:line) without the corresponding identifier.
//
// Pure new-world signal: the verified-identifier set is built
// from typed EvidenceItem fields ONLY. NO file-content reading,
// NO ±N line windows, NO token-overlap heuristic. Backtick
// extraction is a closed syntactic regex (single-backtick pairs
// containing identifier-shaped tokens). Identifiers that the
// explorer did emit as Subject/Object/AnchorSymbol pass through
// trivially; identifiers the LLM invented at finalize time
// (s1a-style "checkBudget" hallucination) fail because the typed
// pool never observed the name.
//
// Default classification: SOFT (defaultSoftKinds). Promotion via
// pipeline_contract_strict_kinds when operator's pipeline
// guarantees full-coverage emit_evidence. Fallback target:
// BackToExplore — re-run the explorer to capture the missing
// identifier in a typed evidence field.
//
// valueLiteralBackedByTypedEvidence reports whether `literal` is
// supported by the typed evidence pool. Two layers: full-string
// equality against EvidenceItem.Subject/Object/AnchorSymbol fields
// (preferred — exact match), or identifier-token membership in the
// pre-built verified set (fallback for multi-token literals).
//
// identifierRegex matches a single Go-style identifier (1+ chars,
// must start with letter or underscore). Compiled once at package
// level for the scanner.
var identifierRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// buildVerifiedIdentifierSet constructs the typed-evidence-pool
// identifier set used by runStepIdentifierBackedByEvidenceOracle.
// Sources (in order, all typed):
//
//   - mut.EmittedEvidence(): every EvidenceItem's Subject, Object,
//     AnchorSymbol fields when identifier-shaped.
//   - mut.EmittedAnswerSymbols(): every AnswerSymbol.Name.
//   - doc.Symbols: same as above (for the rendered slate path).
//   - rm.AnalyzerHints.MentionedEntities: identifier-shaped tokens
//     the analyzer LLM emitted (already RawRequest-validated).
//
// identifierTokenizer extracts identifier-shaped tokens from
// arbitrary text (used to handle multi-word evidence fields like
// Subject="foo calls bar" → adds {foo, calls, bar}). Min 2 chars
// to avoid populating verified set with noise tokens (`a`, `i`,
// `n` from prose). The 2-char floor matches common practice for
// distinguishing identifier prose from loop-counter prose; 1-char
// identifiers in step.description's backticks are admitted via
// the explicit identifier-check path (extractBacktickIdentifiers
// uses identifierRegex which accepts 1+).
var identifierTokenizer = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]+`)

// runValueSecondaryCitationFocusOracle (Phase 6 stage 7, 2026-05-03)
// is the new-world replacement for the retired emit-time
// validateValueCitationFocus token-overlap heuristic. The oracle
// fires on shape=value answers carrying 2+ citations: every
// non-primary citation MUST have a typed-pool match (Subject /
// AnchorSymbol / Object) that names the same scalar literal as
// doc.Value.Literal under the AnswerSubject.Kind lookup discipline.
//
// Off-focus citations turn the answer's "scalar" claim into a soft
// citation cluster — broader background masquerading as direct
// support. The fix is for the extractor to drop the citation (the
// background belongs in summary prose without an extra cite) or
// re-pick a citation that truly defines / references the literal.
//
// Old-world (emit-time validateValueCitationFocus) read citation
// File / Line + token-overlap fallbacks (citationLooksCommentOnly,
// quote-substring); a generic literal like "true" or a single-word
// type name would falsely satisfy the cite when ANY token shape
// matched. New-world reads ONLY the typed evidence pool — no
// raw-source-line tokens, no Quote scanning. Skipped silently when
// AnswerSubject.Kind is not in the citation-focus subject set
// (mirrors valueSubjectNeedsCitationFocus discipline) or pool is
// empty.
//
// matchCitationInEvidencePool finds the highest-scoring
// EvidenceItem whose [LineStart, LineEnd] window contains
// (cite.File, cite.Line). Returns the zero-value EvidenceItem when
// no overlap exists — caller checks Source=="" for that case.
//
// runAbsenceScopeBoundOracle (Phase 4, 2026-05-02) fires when the
// AnswerDocument claims a NEGATIVE finding (status=absent) but no
// citation in the pool carries a bounded negative scope to back the
// claim up. Operationally: an answer that says "X is absent from the
// codebase" must cite at least one Citation with Scope=Negative AND
// non-empty NegativePattern (the pattern that was searched for and
// found absent), otherwise the absence is unfounded.
//
// Default classification: STRICT — safety-critical for config-trace /
// absence questions where downstream consumers act on the negative
// finding (operator removes a config knob, etc.). Fallback target is
// BackToExtract: the next pass must either surface the bounded
// negative citation or re-frame the finding.
//
// inferClaimFormsFromCitations returns, per citation index, the set
// of ClaimForm values projected from the underlying evidence (when
// matchable). Used to compute "inferred coverage" for facets the
// LLM did not explicitly annotate via claim_use.
//
// claimFormForCitation finds the EvidenceItem in MutableState's
// emitted-evidence pool matching the citation by Source path +
// LineStart, then runs ClaimFormOf on it. Returns empty on no match.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// defaultSoftKinds is the set of ViolationKinds that, by default,
// do NOT hard-fail the contract gate (they are mirrored to closure
// for telemetry / future learning but don't trigger finalize retry).
// The pre-commit-53 violation kinds (ViolFamilyMismatch / ViolCitation / ...)
// are NOT in this set; their hard-gate behaviour is preserved
// byte-identically. Only the 3 commit-53 newcomers default to soft.
//
// Operators promote a kind to strict (or demote one to soft) via
// yaml pipeline_contract_strict_kinds + pipeline_contract_soft_kinds;
// see SetSoftViolationKinds below.
// shouldReviewConsistency gates whether the self-consistency
// reviewer dispatches for a given AnswerDocument. The reviewer
// only adds value when the answer has BOTH a summary section
// AND a non-trivial body — single-shape scalars (ShapeValue /
// ShapeBoolean / ShapeConfigValue) lack the dual-paragraph
// structure that intra-answer contradictions live in.
//
// renderConsistencyReviewBody assembles a markdown body string
// from doc.Steps + doc.Symbols so the reviewer LLM sees the
// finalizer's bullet structure verbatim. Citations are stripped
// to file:line form (no Quote text) since we don't want the
// reviewer hallucinating about repo content — its job is
// purely prose↔prose.
//
// runSelfConsistencyReview dispatches the reviewer LLM and
// converts its verdict into types.Violation entries. Method on
// Orchestrator so it has access to reviewer / yaml flags / emit
// surface for the REPL bottom-status line.
//
// defaultSoftKinds returns the canonical soft-by-default map. v3 B0
// (2026-05-04) derives this from each kind's ViolKindSpec.SoftByDefault
// in the registry; the legacy literal below is retained for
// migration-window byte-identical verification
// (TestRegistryDerivesAllLegacyTables). Unregistered kinds fall through
// to the legacy literal so back-compat is preserved.
func defaultSoftKinds() map[types.ViolationKind]bool {
	out := map[types.ViolationKind]bool{}
	for _, spec := range types.AllViolKindSpecs() {
		if spec.SoftByDefault {
			out[spec.Kind] = true
		}
	}
	for k, v := range legacyDefaultSoftKinds() {
		if _, ok := out[k]; !ok && v {
			out[k] = true
		}
	}
	return out
}

// legacyDefaultSoftKinds retains the pre-v3 hardcoded literal. Kept
// during the migration window only.
func legacyDefaultSoftKinds() map[types.ViolationKind]bool {
	return map[types.ViolationKind]bool{
		types.ViolViewIntentMismatch:           true,
		types.ViolSubTopicCountMismatch:        true,
		types.ViolDiagramIdentifier:            true,
		types.ViolDeclaredCountDrift:           true,
		types.ViolExternalArtifactUnderdecoded: true,
		// Block 1 (2026-05-02) reviewer-side kinds — telemetry only,
		// never block apply / answer ship.
		types.ViolPlanCritic:              true,
		types.ViolReflectorObservation:    true,
		types.ViolAnswerReviewerDistilled: true,
		// Block 2 (2026-05-02) Intent / Subject / PredicateAxis
		// oracle kinds — soft by default; promote individually via
		// pipeline_contract_strict_kinds when an operator wants
		// retry-loop enforcement.
		types.ViolIntentTraceShallow:     true,
		types.ViolIntentEnumerateNotList: true,
		types.ViolIntentRootCauseNoCause: true,
		types.ViolIntentConfigNoTrail:    true,
		types.ViolSubjectAnchorMissing:   true,
		types.ViolPredicateAxisMissing:   true,
		// Phase 4 (Semantic Surface Contract) — default classification:
		//   ViolFacetUncovered      → SOFT (covering a facet often
		//     requires fresh evidence; a missed HARD facet is a hint
		//     to re-explore, not a finalizer self-failure).
		//   ViolClaimFormUnsupported → STRICT (LLM emitted an annotation
		//     incompatible with the cited evidence — a finalizer rewrite
		//     fixes it, no new evidence needed).
		//   ViolAbsenceScopeExceeded → STRICT (safety-critical: the
		//     answer overstates the searched scope on a negative claim;
		//     downstream consumers act on the absence, must not be
		//     overstated).
		types.ViolFacetUncovered: true,
		// Phase 4 extension (2026-05-02) — step-identifier-backed-by-
		// evidence oracle. SOFT-by-default: the signal is precise but
		// the verified-identifier set may be incomplete on some
		// explorer paths; soft fail + retry hint is the safe default.
		// Operators promote via pipeline_contract_strict_kinds when
		// their pipeline guarantees full-coverage emit_evidence.
		types.ViolStepIdentifierUnverified: true,
		// Phase 5 — telemetry-only richness regression. SOFT and
		// explicitly NOT promotable; soft classification is permanent.
		types.ViolRichnessRegression: true,
		// Phase 6 stage 7 — value secondary citation focus. SOFT-by-
		// default: under default classification the answer ships
		// with the off-focus citations and the gap is recorded for
		// cross-Run learning. Operators promote to STRICT when their
		// extractor pipeline guarantees focused secondary citations.
		types.ViolValueSecondaryCitationOffFocus: true,
		// B6-F1 (post-shape consolidated audit, 2026-05-04) —
		// cross-citation single-locus oracle. SOFT-by-default: the
		// answer can ship with a "different lines for same symbol"
		// note (the Detail names both loci verbatim); operators who
		// need auto-rewrite-to-single-locus behaviour promote via
		// pipeline_contract_strict_kinds.
		types.ViolCrossCitationConflict: true,
		// G3 (post_v2_runtime_gap_remediation, 2026-05-04) —
		// diagram edge label inference vs typed RelationKind drift.
		// SOFT-by-default and DELIBERATELY not promotable to STRICT:
		// label inference is a noisy signal (R3 invariant — only
		// precise signals drive hard gates). Operators who set
		// pipeline_contract_strict_kinds for this kind get a no-op
		// promotion at the gate level; the validator emits SOFT
		// regardless.
		types.ViolDiagramEdgeLabelMismatch: true,
		// G5 (post_v2_runtime_gap_remediation, 2026-05-04) — semantic
		// quality reviewer signal. SOFT-by-default;
		// pipeline_contract_strict_kinds CAN promote (unlike
		// DiagramEdgeLabelMismatch — reviewer LLM is more reliable
		// than label substring inference) so operators who want a
		// stricter answer floor can lift it into the retry loop.
		types.ViolAnswerSemanticUnderfilled: true,
		// 修 B (post_v2_runtime_gap_remediation, 2026-05-04) —
		// enumeration evidence pool underspecification. SOFT-by-
		// default; pipeline_contract_strict_kinds CAN promote
		// once eval validates the false-positive rate is low.
		types.ViolEnumerationEvidenceUnderspecified: true,
		// B3 v3 (2026-05-04) — diagram relation typed-first label-only
		// advisory. SOFT-by-default; promotable when operator wants
		// typed declarations enforced.
		types.ViolDiagramRelationLabelOnly: true,
		// B2 v3 (2026-05-04) — three-layer quality contract violations
		// are NOT in the soft map; they trigger retry by default
		// (Severity=Medium → RetryEligible=true). Operators may demote
		// via pipeline_contract_soft_kinds when noise rate is too high.
		// Listed here as documentation (the comment, not the entry).

		// Multi-repo write fail-loud (design §4.5.5, 2026-05-08).
		// SOFT here means "no auto-retry pathway" — the actual
		// fail-loud comes from planPostHook returning an error which
		// terminates the Run with a user-facing "split into separate
		// runs" message. Operators cannot promote (Promotable=false).
		types.ViolWriteCrossSubRepoForbidden: true,

		// L3 negative-knowledge answer validator (TypedDenials Phase F,
		// 2026-05-08). SOFT-by-default — ledger-only signal, never
		// blocks shipping; operators may promote via
		// pipeline_contract_strict_kinds yaml once baseline eval
		// confirms false-positive rate is low for their workload.
		types.ViolDeniedTokenUndeclared: true,
	}
}

// softViolationKinds is the active set; mutated by
// SetSoftViolationKinds at startup.
//
// Commit 55 (Batch A.1): the map is now guarded by softKindsMu
// so parallel tests (and any future runtime-reconfig path) cannot
// observe a half-mutated state. Production cmd calls
// SetSoftViolationKinds once at startup before any Run, so the
// mutex contention is effectively zero at runtime; tests with
// t.Cleanup-restored overrides are race-free regardless of go
// test -parallel.
var (
	softKindsMu        sync.RWMutex
	softViolationKinds = defaultSoftKinds()
)

// SetSoftViolationKinds replaces the active soft-kind set. Empty
// args restore defaults. Called from cmd/root.go after reading
// runtime config. Order: start from defaults, add `extraSoft`,
// remove `extraStrict`.
func SetSoftViolationKinds(extraSoft []string, extraStrict []string) {
	out := defaultSoftKinds()
	for _, name := range extraSoft {
		k := strings.TrimSpace(name)
		if k != "" {
			out[types.ViolationKind(k)] = true
		}
	}
	for _, name := range extraStrict {
		delete(out, types.ViolationKind(strings.TrimSpace(name)))
	}
	softKindsMu.Lock()
	softViolationKinds = out
	softKindsMu.Unlock()
}

// isSoftViolationKind is the read-side predicate. Used by
// hasAnyStrictViolation + the soft-violation-only renderer.
//
// R15 (post_shape_residual_audit.md, 2026-05-04): refactored to
// read from types.ViolationProfileFor — the single source of truth
// shared with R14 Severity. Legacy softViolationKinds map is
// preserved for operator-yaml override fidelity (extraSoft +
// extraStrict), but the underlying classification reads R14
// Severity to ensure Critical/High/Medium kinds are never
// mis-classified as soft (which was the m1a r2 retry-skipped bug).
//
// Resolution rule (R15):
//   - operator-yaml extraStrict overrides win (kind absent from
//     softViolationKinds → strict)
//   - operator-yaml extraSoft overrides win (kind present in
//     softViolationKinds → soft, even if R14 says Critical)
//   - default (no operator override): consult ViolationProfile —
//     Severity == Soft → soft; otherwise strict
func isSoftViolationKind(k types.ViolationKind) bool {
	softKindsMu.RLock()
	mapVal, hasOverride := softViolationKinds[k]
	softKindsMu.RUnlock()
	if hasOverride {
		// Operator-yaml or initialised default explicitly named this
		// kind — honour the override exactly.
		return mapVal
	}
	// No explicit override → consult ViolationProfile. Pass
	// isStrict=false because we're in the "no operator strict
	// override" branch by construction.
	return !types.ViolationProfileFor(k, false).RetryEligible
}

// hasAnyStrictViolation reports whether the slice contains at least
// one violation whose Kind is NOT in softViolationKinds. The gate
// flips Passed=false only when this returns true.
func hasAnyStrictViolation(vs []types.Violation) bool {
	for _, v := range vs {
		if !isSoftViolationKind(v.Kind) {
			return true
		}
	}
	return false
}

// isJustifiedAbsenceAnswer reports whether the finalized document is
// an honest "zero" shape AND the investigation did enough work to
// trust that zero. Both halves matter:
//
//	shape check — 0 symbols with completeness=complete, a value
//	              literal that reads as zero/none, or boolean=false.
//	trust check — shape-tiered: shallow shapes ("how many X",
//	              "does X exist", "what is the value of K") are
//	              honestly answered with ONE list_files / grep /
//	              exec_command / repo_map — no file contents needed.
//	              Deep shapes ("list all handlers that do X",
//	              "explain the flow", "walk the call chain") demand
//	              at least one real content read (read_file, or a
//	              grep that returned line-bearing matches) so the
//	              LLM cannot claim "no handler does X" without
//	              opening any file.
//
// The shape-tiered gate replaces the earlier "read_file ≥ 3"
// threshold which wrongly rejected legitimate one-call answers on
// existence / count questions.
func isJustifiedAbsenceAnswer(mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	// B8-T4 (block_only_carrier.md §5.8): structural-path V1 reader
	// removed (doc.Shape / doc.Symbols / doc.Value / doc.Boolean).
	// The declarative path (StableAbsenceJustification) is V2-compatible
	// and now the sole fast-path; V2 carrier's runV2BlockOracles +
	// validateRequiredBlockCoverage cover the absence-shape coherence
	// (e.g. an empty bullet_list block under QFEnumeration is
	// structurally an "honest zero").
	if strings.TrimSpace(mut.StableAbsenceJustification()) != "" {
		return hasAnyInvestigationSuccess(mut)
	}
	return false
}

// hasAnyInvestigationSuccess reports whether Turn A succeeded in at
// least one investigation-class tool call. This is the audit floor
// for the declarative absence path — the LLM's "this is zero" claim
// is not credible when the investigation is entirely empty.
func hasAnyInvestigationSuccess(mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	ta := mut.TurnAArtifacts()
	if ta == nil {
		return false
	}
	investigation := map[string]bool{
		"grep":         true,
		"exec_command": true,
		"list_files":   true,
		"read_file":    true,
		"repo_map":     true,
	}
	for _, r := range ta.ToolResults {
		if !r.Success {
			continue
		}
		if investigation[r.ToolName] {
			return true
		}
	}
	return false
}

// isShallowShape reports whether a shape can be honestly answered
// without reading file contents. The distinction tracks what the
// question fundamentally asks:
//
//	shallow — the answer is a count, an existence decision, or a
//	          single config value. "How many .py files" is answered
//	          by a find / ls / list_files in one call; opening any
//	          file adds no information.
//	deep    — the answer enumerates functions that do X, walks a
//	          flow, or explains a mechanism. Claiming "no handler
//	          does X" demands inspecting the candidate handlers'
//	          code — listing file names is not sufficient because
//	          the question is about behaviour, not identity.
//
// hasInvestigationEvidence reports whether Turn A did enough work to
// back an absence claim in the given shape. Two-tier rule:
//
//	shallow shape — any single successful investigation-class tool
//	                call is sufficient. Rejects the "zero tools,
//	                pure guess" failure mode and nothing else.
//	deep shape    — at least one actual content read (read_file or
//	                line-bearing grep). Rejects "I listed the files
//	                and claim nothing inside does X" without looking.
func isDriftBoundedCitationAnswer(bus *types.BusContext, out *agent.StageOutput) bool {
	if bus == nil || bus.AnalysisIR == nil || bus.Mutable == nil {
		return false
	}
	// B8-T4 (block_only_carrier.md §5.8): the doc.Shape gate
	// (ShapeExplanation / ShapeStepList only) is removed — V2
	// carrier covers explanation/step semantics through QFGeneric +
	// QFRootCauseTrace block requirements; the surface-plan mode +
	// drift-bounded surface items are the load-bearing precondition.
	docV2 := bus.Mutable.AnswerDocumentV2()
	if docV2 == nil {
		return false
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(bus)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return false
	}
	if len(plan.LogSourceDriftAnchors) == 0 || len(plan.DriftBoundedSurfaceItems) == 0 {
		return false
	}
	return finalizerCitationPoolSize(bus.Mutable, out) >= 1
}

// citationRegex matches `path/to/file.ext:NNN` style references. The
// path must contain at least one `/` or end in a typical source
// extension; the line is a positive integer. Permissive on the path
// shape so it catches subdir hits like `internal/agent/explorer.go:42`
// without matching prose like "step 3:" or "10:30".
//
// The pattern is intentionally structural (path char class + dot +
// extension + colon + digits), not a curated list of file extensions
// known to this repo. A new repo with .lua / .php / .swift sources
// gets the same recall without code changes.
var citationRegex = regexp.MustCompile(
	`(?:^|[\s\(\[\<` + "`" + `])` + // word boundary leader
		`([A-Za-z0-9_\-./]*[A-Za-z0-9_\-]+\.[A-Za-z0-9]{1,8})` + // path with extension
		`:(\d{1,6})` + // : line
		`(?:-(\d{1,6}))?`, // optional -end for ranges
)

// extractCitationsFromAnswer pulls every file:line[-end] reference
// out of the answer body and returns them as a contract.Citation
// slice. Duplicates (same file, same line) are de-duplicated so a
// single reference repeated three times in the prose still counts
// as one citation (the contract checker measures distinct anchors).
func extractCitationsFromAnswer(text string) []contract.Citation {
	if text == "" {
		return nil
	}
	matches := citationRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]contract.Citation, 0, len(matches))
	for _, m := range matches {
		file := m[1]
		// Reject obvious prose hits that the regex's permissive
		// path-char class still lets through. The structural cues
		// are: must contain at least one `/` (otherwise it's a bare
		// "go.mod:5" or "README.md:1" — still a valid citation, so
		// we let those through) AND the matched line must parse as
		// a non-zero positive int (already enforced by \d+).
		if file == "" {
			continue
		}
		line, err := strconv.Atoi(m[2])
		if err != nil || line == 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", file, line)
		if seen[key] {
			continue
		}
		seen[key] = true
		c := contract.Citation{File: file, Line: line}
		if len(m) > 3 && m[3] != "" {
			if end, err := strconv.Atoi(m[3]); err == nil && end >= line {
				c.Lines = []int{line, end}
			}
		}
		out = append(out, c)
	}
	return out
}

// renderViolations turns a contract.Result into a single short
// diagnostic string suitable for injecting into the explorer's
// retry hint. One sentence per violation, separated by `; `, so
// the Retry Directive section stays compact.
//
// Session 11 F4 routes through hint.Composer.RenderCompact so the
// legacy one-line format is now generated by the structured-hint
// facility (in preparation for the eventual strict-mode switch
// that renders the 6-field block instead). Call sites / tests
// that match the legacy ";"-separated format continue to work
// byte-identically.
func renderViolations(res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	// W2.4 (2026-05-05): drop IsDerived=true entries before
	// rendering so the retry hint surfaces ONE root per
	// fingerprint, not N facets of the same problem. ChainsDemoted
	// / SymbolAnchorMismatch / EdgeUnsupported etc. that are
	// flagged as derivations of a higher-level root carry no
	// independent fix instruction; suppressing them stops the
	// "fix one, three new ones rotate in" loop seen pre-W2.
	roots := FilterDerivedViolations([]types.Violation(res.Violations))
	if len(roots) == 0 {
		return ""
	}
	h, _ := hintComposerSingleton.Compose(hint.Context{}, roots)
	return hintComposerSingleton.RenderCompact(h)
}

// hintComposerSingleton is the package-wide Composer used by every
// orchestrator-side hint producer. The zero config (non-strict,
// default caps) matches the pre-session-11 behaviour; switching to
// strict mode is a one-line change here once every producer
// populates the 6-field contract.
var hintComposerSingleton = hint.New(hint.DefaultConfig())

// finalizerCitationPoolSize returns the authoritative citation count
// to feed into criterion.Env.DraftCitations. The count is sourced in
// this priority order:
//
//  1. Mutable.AnswerDocumentV2().Citations — populated by V2
//     emit_answer_document.Execute after grounding + remap. This is
//     the exact pool the renderer consults.
//  2. extractCitationsFromAnswer(out.FinalAnswer) — legacy text-regex
//     fallback when AnswerDocumentV2 was never set (test harnesses
//     that route directly through StageOutput.FinalAnswer).
//
// The regex fallback under-counts on enumeration / step_list shapes
// because those shapes inline citations into per-row renders and do
// not emit the pool as a bulleted list. Using the pool count fixes
// the "4 citations but orchestrator says 1" class of bugs.
func finalizerCitationPoolSize(mut *types.MutableState, out *agent.StageOutput) int {
	if mut != nil {
		if doc := mut.AnswerDocumentV2(); doc != nil && len(doc.Citations) > 0 {
			return len(doc.Citations)
		}
	}
	if out != nil {
		return len(extractCitationsFromAnswer(out.FinalAnswer))
	}
	return 0
}

// formatViolationsForLogger renders the contract checker's
// violation digest as a single string, suitable for either a
// logger.Warning line OR an answer-prepended caveat (caller's
// choice). Pure formatting — no side effects on the answer.
//
// Returns "" when the result passed or carries no violations
// (caller treats empty string as "nothing to surface").
//
// Pre-B4-F3 the function was named appendViolationsToAnswer and
// ── R2.1 V2 self-consistency reviewer (post_shape_residual_audit.md
// 2026-05-04) ──────────────────────────────────────────────────────

// shouldReviewConsistencyV2 gates whether the self-consistency
// reviewer dispatches for a given V2 AnswerDocumentV2. The reviewer
// adds value only when the doc has BOTH a non-trivial summary AND
// enough body content for inter-paragraph contradictions to live in.
// V2 carrier mirrors the V1 floors (summary >= 100 chars + body
// items >= 3) but reads them off blocks[] instead of doc.Summary /
// doc.Steps / doc.Symbols.
//
// Family eligibility: only families that produce a summary + body
// structure benefit. Single-block scalar / boolean / config_value
// answers (BlockScalar / BlockDecision sole-content shapes) lack
// the dual-paragraph surface; reviewer is silently skipped.
func shouldReviewConsistencyV2(doc *types.AnswerDocumentV2) bool {
	if doc == nil || len(doc.Blocks) == 0 {
		return false
	}
	summaryRunes := 0
	bodyItems := 0
	hasSection := false
	for _, blk := range doc.Blocks {
		switch blk.Kind {
		case types.BlockSummary:
			summaryRunes += len([]rune(strings.TrimSpace(blk.Text)))
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
			bodyItems += len(blk.Items)
		case types.BlockSection:
			// Section text counts as body content; multi-paragraph
			// section often is where contradictions surface.
			if strings.TrimSpace(blk.Text) != "" {
				hasSection = true
				bodyItems++
			}
		}
	}
	if summaryRunes < 100 {
		return false
	}
	if bodyItems < 3 && !hasSection {
		return false
	}
	return true
}

// renderConsistencyReviewBodyV2 assembles a markdown body string
// from V2 blocks for the reviewer LLM. Walks blocks in declaration
// order so the reviewer sees the same surface the user sees.
//
// Only body-shaped blocks render here (BlockOrderedList /
// BlockBulletList / BlockSection / BlockTable / BlockCaveat /
// BlockScalar / BlockDecision); BlockSummary is rendered separately
// as the AnswerSummary input. BlockDiagram is skipped — reviewer's
// job is prose↔prose, not figure validation.
//
// Citations strip to file:line form via Quote=""; reviewer should
// not introspect repo content.
func renderConsistencyReviewBodyV2(doc *types.AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	for _, blk := range doc.Blocks {
		switch blk.Kind {
		case types.BlockSummary, types.BlockDiagram:
			continue
		case types.BlockSection:
			title := strings.TrimSpace(blk.Title)
			if title != "" {
				fmt.Fprintf(&b, "## %s\n", title)
			}
			text := strings.TrimSpace(blk.Text)
			if text != "" {
				fmt.Fprintf(&b, "%s\n\n", text)
			}
		case types.BlockOrderedList:
			for i, it := range blk.Items {
				desc := itemBodyText(it)
				fmt.Fprintf(&b, "%d. %s\n", i+1, desc)
			}
			b.WriteString("\n")
		case types.BlockBulletList, types.BlockTable:
			for _, it := range blk.Items {
				desc := itemBodyText(it)
				fmt.Fprintf(&b, "- %s\n", desc)
			}
			b.WriteString("\n")
		case types.BlockScalar:
			text := strings.TrimSpace(blk.Text)
			if text != "" {
				fmt.Fprintf(&b, "Scalar: %s\n\n", text)
			}
		case types.BlockDecision:
			text := strings.TrimSpace(blk.Text)
			if text != "" {
				fmt.Fprintf(&b, "Decision: %s\n\n", text)
			}
		case types.BlockCaveat:
			text := strings.TrimSpace(blk.Text)
			if text != "" {
				fmt.Fprintf(&b, "[caveat] %s\n\n", text)
			}
		}
	}
	return b.String()
}

// itemBodyText returns the most informative single-line rendering
// of an AnswerBlockItem for the reviewer's body view: prefer the
// Label when set, fall back to Text. Caller adds ordinal / bullet
// prefix.
func itemBodyText(it types.AnswerBlockItem) string {
	label := strings.TrimSpace(it.Label)
	text := strings.TrimSpace(it.Text)
	switch {
	case label != "" && text != "":
		return label + " — " + text
	case label != "":
		return label
	default:
		return text
	}
}

// runSelfConsistencyReviewV2 dispatches the reviewer LLM with V2
// carrier inputs and converts its verdict into types.Violation
// entries. Mirrors the V1 control flow — reviewer error / nil
// verdict / sub-floor confidence are non-fatal (answer ships,
// learning failure recorded, no violations returned).
func (o *Orchestrator) runSelfConsistencyReviewV2(doc *types.AnswerDocumentV2, mut *types.MutableState) []types.Violation {
	if o == nil || o.selfConsistencyReviewer == nil || doc == nil || mut == nil {
		return nil
	}
	floor := o.selfConsistencyMinConfidence
	if floor <= 0 {
		floor = 0.8
	}

	// Status line: never silent background work.
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeSelfConsistencyStart,
		Reasoning:  selfConsistencyReviewStartMessage(o.busCtx.Language),
	})

	// Pull summary + body off V2 blocks. Strip system-injected
	// hedge / Authority caveat tokens so the reviewer LLM doesn't
	// flag system annotations as factual contradictions.
	summaryText := ""
	for _, blk := range doc.Blocks {
		if blk.Kind == types.BlockSummary {
			summaryText += blk.Text
		}
	}
	in := SelfConsistencyInput{
		OriginalRequest:   mut.Objective(),
		AnswerSummary:     render.StripAuthorityArtifacts(summaryText),
		AnswerBody:        render.StripAuthorityArtifacts(renderConsistencyReviewBodyV2(doc)),
		EvidenceAnchorSet: BuildEvidenceAnchorSet(mut.EmittedEvidence()),
	}

	ctx := o.busCtx.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	verdict, err := o.selfConsistencyReviewer.Review(ctx, in)
	if err != nil {
		mut.AppendLearningFailure("self_consistency_reviewer", err.Error())
		logging.Warning("[self_consistency_reviewer] dispatch failed (non-fatal): %v", err)
		return nil
	}
	if verdict == nil || verdict.Consistent || verdict.Confidence < floor {
		if verdict != nil {
			logging.Info("[self_consistency_reviewer] verdict consistent=%v confidence=%.2f (floor=%.2f) reasoning=%q",
				verdict.Consistent, verdict.Confidence, floor, verdict.Reasoning)
		}
		return nil
	}

	// Surface contradiction count + rewrite-mode to the user. The
	// NoticeKind tracks the rewrite flag so the dock paints active
	// rewriting in yellow (system is doing more work) and the
	// advisory-only path in gray (already shipping; just FYI).
	contradictionKind := render.NoticeSelfConsistencyContradictionLogged
	if o.selfConsistencyRewriteOnContradiction {
		contradictionKind = render.NoticeSelfConsistencyContradictionRewriting
	}
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: contradictionKind,
		Reasoning:  selfConsistencyContradictionMessage(o.busCtx.Language, o.selfConsistencyRewriteOnContradiction, len(verdict.Contradictions)),
	})

	out := make([]types.Violation, 0, len(verdict.Contradictions))
	totalN := len(verdict.Contradictions)
	reasoning := clampReasoningForRepair(verdict.Reasoning)
	for i, c := range verdict.Contradictions {
		out = append(out, types.Violation{
			Kind: types.ViolSelfContradiction,
			Detail: fmt.Sprintf("self_contradiction[%s] — SUMMARY: %q ⇄ BODY: %q",
				c.Topic, c.SummaryClaim, c.BodyClaim),
			Repair:     buildSelfContradictionRepair(c, reasoning, i+1, totalN),
			ClusterKey: topicClusterKey(c.Topic, "answer_summary_body_consistency"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_summary_body_consistency",
				Reason:     "reviewer detected inter-paragraph contradiction (V2 carrier)",
				Confidence: verdict.Confidence,
			},
			Stage: string(types.StageFinalize),
		})
	}
	logging.Info("[self_consistency_reviewer] V2 emitted %d contradiction(s) at confidence=%.2f rewrite_on=%v reasoning=%q",
		len(verdict.Contradictions), verdict.Confidence, o.selfConsistencyRewriteOnContradiction, verdict.Reasoning)
	return out
}

// clampReasoningForRepair returns a single-line, ≤ 200-rune
// rendering of the reviewer's reasoning, suitable for embedding
// in a Violation.Repair string.
func clampReasoningForRepair(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
	s = strings.TrimSpace(s)
	const maxLen = 200
	r := []rune(s)
	if len(r) > maxLen {
		return string(r[:maxLen]) + "…"
	}
	return s
}

// buildSelfContradictionRepair generates the LLM-actionable repair
// prose for a single contradiction. Names both verbatim claims +
// reviewer reasoning + ordinal so the finalizer's retry sees
// concrete reconciliation targets, not abstract guidance.
func buildSelfContradictionRepair(c SelfConsistencyContradiction, reasoning string, idx, total int) string {
	var b strings.Builder
	if total > 1 {
		fmt.Fprintf(&b, "[%d/%d] ", idx, total)
	}
	b.WriteString("re-emit emit_answer_document so summary and body align on this fact: ")
	if c.Topic != "" {
		fmt.Fprintf(&b, "topic %q. ", c.Topic)
	}
	if c.SummaryClaim != "" {
		fmt.Fprintf(&b, "Summary asserts %q; ", c.SummaryClaim)
	}
	if c.BodyClaim != "" {
		fmt.Fprintf(&b, "body asserts %q. ", c.BodyClaim)
	}
	b.WriteString("Pick the claim supported by the cited evidence and rewrite the OTHER side to match (do NOT invent new evidence).")
	if reasoning != "" {
		fmt.Fprintf(&b, " Reviewer reasoning: %s", reasoning)
	}
	return b.String()
}

// always wrote the digest into the answer text. The B4
// (post_shape_retirement_consolidated_audit.md §8 Batch B4,
// 2026-05-04) refactor decoupled the formatter from the channel
// decision: callers now invoke this helper to GET the digest, and
// the channel decision (user panel vs operator log) lives in
// orchestrator.go::decideViolationOutputChannel reading
// settings.ViolationBudget.UserVisibleViolationCaveat.
func formatViolationsForLogger(res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("· answer-contract validation exhausted: ")
	b.WriteString(renderViolations(res))
	return b.String()
}

// validateEnumerationEvidenceCoverage (修 B,
// post_v2_runtime_gap_remediation, 2026-05-04) is the structural
// gate that fires when the user's question states an explicit
// enumeration count N AND the evidence pool has fewer than N
// distinct typed anchor_symbol values. It diagnoses the s1a-class
// failure where the explorer aggregates N parallel callsites into
// a single line_range item with anchor_symbol pointing at the
// container array — leaving the pool with too few typed names for
// the renderer to enumerate without placeholders.
//
// Trigger conditions (all must hold):
//
//  1. mut.RequestModel().EnumerationBoundary != nil AND DeclaredCount > 0
//  2. The resolved family is enumeration-shaped (QFEnumeration /
//     QFRootCauseTrace / QFCallChain) — these are the families
//     whose answers carry an ordered_list slate where each item
//     SHOULD render a verbatim identifier.
//  3. len(unique anchor_symbol in evidence pool) < DeclaredCount.
//
// The signal is typed (DeclaredCount integer + unique-anchor count
// integer) so SOFT classification is precise; hard-gate promotion
// is intentionally gated behind operator opt-in via
// pipeline_contract_strict_kinds because identifying which
// anchor_symbol values are "named callsite identifiers" vs
// "container-name aggregations" requires structural pattern match
// the explorer pipeline does not currently expose.
//
// Repair routes to FallbackBackToExplore — the only honest fix is
// gathering more evidence, not finalizer rewrite.
func validateEnumerationEvidenceCoverage(mut *types.MutableState, view *types.AnswerSemanticView) []types.Violation {
	if mut == nil || view == nil {
		return nil
	}
	rm := mut.RequestModel()
	if rm == nil || rm.EnumerationBoundary == nil {
		return nil
	}
	declared := rm.EnumerationBoundary.DeclaredCount
	if declared <= 0 {
		return nil
	}
	switch view.Family {
	case types.QFEnumeration, types.QFRootCauseTrace, types.QFCallChain:
		// fall through
	default:
		return nil
	}

	uniqueAnchors := make(map[string]struct{}, declared)
	for _, ev := range mut.EmittedEvidence() {
		s := strings.TrimSpace(ev.AnchorSymbol)
		if s == "" {
			continue
		}
		uniqueAnchors[s] = struct{}{}
	}
	if len(uniqueAnchors) >= declared {
		return nil
	}
	gap := declared - len(uniqueAnchors)
	return []types.Violation{{
		Kind: types.ViolEnumerationEvidenceUnderspecified,
		Detail: fmt.Sprintf(
			"the question states %d enumerated items but the evidence pool has only %d distinct named anchors (gap: %d)",
			declared, len(uniqueAnchors), gap),
		Repair: fmt.Sprintf(
			"re-investigate the source files that contain the parallel callsites and emit one evidence item PER named identifier (each with `scope=line` and a distinct `anchor_symbol`). The current pool aggregates the parallel set into too few typed names — the answer cannot enumerate %d items without %d distinct anchors. Read the file region carrying the parallel definitions / registrations / appends and call `emit_evidence` with one item per identifier.",
			declared, declared),
		ClusterKey: familyClusterKey(view.Family, "evidence_pool_named_anchors"),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "evidence_pool_named_anchors",
			Reason:     "enumeration evidence pool below declared count",
			Confidence: 0.7,
		},
		Stage: string(types.StageExtract),
	}}
}

// shouldReviewSemanticQuality gates the G5 reviewer dispatch
// (post_v2_runtime_gap_remediation, 2026-05-04; P4 prefilter
// 2026-05-10). Skips when:
//   - any HARD violation already present (we'd double-noise the
//     answer that's already failing other gates)
//   - the doc has no body to evaluate (single-block summary-only)
//   - the doc has emitted no facet declarations at all (no
//     surface for AnchoredCount/DeclaredCount to evaluate)
//
// Returns true to invoke the reviewer.
//
// P4 prefilter rationale: the reviewer's load-bearing axis is
// per-facet AnchoredCount vs DeclaredCount mismatch. A doc that
// declared zero facets cannot have an anchored-vs-declared gap by
// construction; running the reviewer on it just spends an LLM
// round. Forensic data (May-9 sweep): docs with declared count = 0
// account for ~7% of reviewer dispatches, ~all of which produced
// either nil verdicts or sub-floor confidence.
func shouldReviewSemanticQuality(doc *types.AnswerDocumentV2, existing []types.Violation) bool {
	if doc == nil || len(doc.Blocks) < 2 {
		// No body — reviewer has nothing to judge.
		return false
	}
	// Hard violations present — defer reviewer to a clean run.
	for _, v := range existing {
		switch v.Kind {
		case types.ViolBlockCoverageMissing,
			types.ViolPrincipalClaimUseMissing,
			types.ViolFacetUncovered,
			types.ViolDiagramEdgeUnsupported,
			types.ViolFamilyMismatch,
			types.ViolViewIntentMismatch,
			types.ViolViewSwap:
			return false
		}
	}
	// P4 prefilter: skip when no block declared any facet via
	// block.facet_ids[] OR claim_uses[].facet_id. Walking once is
	// cheap relative to a reviewer LLM round-trip.
	for _, b := range doc.Blocks {
		if len(b.FacetIDs) > 0 {
			return true
		}
		for _, cu := range b.ClaimUses {
			if strings.TrimSpace(cu.FacetID) != "" {
				return true
			}
		}
	}
	return false
}

// runSemanticQualityReview dispatches the G5 reviewer LLM and
// converts its verdict into types.Violation entries. Coverage and
// richness concerns use ViolAnswerSemanticUnderfilled (soft by
// default); topic_mismatch uses ViolAnswerTopicMismatch (high severity
// by default).
//
// Mirrors runSelfConsistencyReviewV2 control flow — reviewer error /
// nil verdict / sub-floor confidence are non-fatal (answer ships,
// learning failure recorded, no violations returned).
func (o *Orchestrator) runSemanticQualityReview(doc *types.AnswerDocumentV2, mut *types.MutableState) []types.Violation {
	if o == nil || o.semanticQualityReviewer == nil || doc == nil || mut == nil {
		return nil
	}
	// P7 (2026-05-10) — surface the reviewer dispatch to the dock
	// so the user knows the second reviewer is running. Parallels
	// selfConsistencyReviewStartMessage at the top of
	// runSelfConsistencyReviewV2.
	if o.busCtx != nil {
		o.emit(render.Event{
			Kind:       render.EventOrchestratorNotice,
			Timestamp:  time.Now(),
			Agent:      "orchestrator",
			NoticeKind: render.NoticeSemanticQualityReviewStart,
			Reasoning:  semanticQualityReviewStartMessage(o.busCtx.Language),
		})
	}
	// P4 (2026-05-10): respect operator-tunable floor when set
	// (yaml `pipeline_semantic_quality_min_confidence`); fall back
	// to SemanticQualityMinConfidenceDefault (0.92).
	floor := SemanticQualityMinConfidenceDefault
	if o.semanticQualityMinConfidence > 0 && o.semanticQualityMinConfidence <= 1 {
		floor = o.semanticQualityMinConfidence
	}

	// Project the typed orchestrator state into the reviewer input.
	view := types.BuildAnswerSemanticViewForBusContext(o.busCtx)
	summaryText := ""
	for _, blk := range doc.Blocks {
		if blk.Kind == types.BlockSummary {
			summaryText += blk.Text
		}
	}
	in := BuildSemanticQualityInput(
		mut.Objective(),
		render.StripAuthorityArtifacts(summaryText),
		render.StripAuthorityArtifacts(renderConsistencyReviewBodyV2(doc)),
		doc, view,
		BuildEvidenceAnchorSet(mut.EmittedEvidence()),
	)

	ctx := o.busCtx.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	verdict, err := o.semanticQualityReviewer.Review(ctx, in)
	if err != nil {
		mut.AppendLearningFailure("semantic_quality_reviewer", err.Error())
		logging.Warning("[semantic_quality_reviewer] dispatch failed (non-fatal): %v", err)
		return nil
	}
	effectiveFloor := semanticQualityEffectiveConfidenceFloor(floor, verdict)
	if verdict == nil || verdict.Sufficient || verdict.Confidence < effectiveFloor {
		if verdict != nil {
			logging.Info("[semantic_quality_reviewer] verdict sufficient=%v confidence=%.2f (floor=%.2f) reasoning=%q",
				verdict.Sufficient, verdict.Confidence, effectiveFloor, verdict.Reasoning)
		}
		return nil
	}
	concerns := verdict.Concerns
	if effectiveFloor < floor && verdict.Confidence < floor {
		concerns = filterSemanticQualityConcernsByKind(concerns, semanticConcernTopicMismatch)
		if len(concerns) == 0 {
			return nil
		}
	}

	out := make([]types.Violation, 0, len(concerns))
	reasoning := clampReasoningForRepair(verdict.Reasoning)
	for i, c := range concerns {
		kind := semanticQualityViolationKind(c)
		normalizedKind := normaliseSemanticQualityConcernKind(c.Kind)
		detailPrefix := "answer_underfilled"
		rootField := "answer_semantic_quality"
		repair := fmt.Sprintf("[%d/%d] expand the answer to address %q. %s",
			i+1, len(concerns), c.Topic, c.Suggestion)
		if kind == types.ViolAnswerTopicMismatch {
			detailPrefix = "answer_topic_mismatch"
			rootField = "answer_semantic_topic"
			repair = fmt.Sprintf("[%d/%d] rewrite the answer around the requested topic %q. %s",
				i+1, len(concerns), c.Topic, c.Suggestion)
		}
		if reasoning != "" {
			repair += fmt.Sprintf(" Reviewer rationale: %s", reasoning)
		}
		detail := fmt.Sprintf("%s[%s:%s] — observation: %s",
			detailPrefix, normalizedKind, c.Topic, c.Observation)
		clusterKey := topicClusterKey(c.Topic, "answer_semantic_quality")
		root := types.SuspectedRoot{
			IRField:    rootField,
			Reason:     "reviewer detected " + normalizedKind,
			Confidence: verdict.Confidence,
		}
		if kind == types.ViolAnswerTopicMismatch {
			out = append(out, types.Violation{
				Kind:          types.ViolAnswerTopicMismatch,
				Detail:        detail,
				Repair:        repair,
				ClusterKey:    clusterKey,
				SuspectedRoot: root,
				Stage:         string(types.StageFinalize),
			})
			continue
		}
		out = append(out, types.Violation{
			Kind:          types.ViolAnswerSemanticUnderfilled,
			Detail:        detail,
			Repair:        repair,
			ClusterKey:    clusterKey,
			SuspectedRoot: root,
			Stage:         string(types.StageFinalize),
		})
	}
	logging.Info("[semantic_quality_reviewer] emitted %d concern(s) at confidence=%.2f reasoning=%q",
		len(verdict.Concerns), verdict.Confidence, verdict.Reasoning)
	return out
}
