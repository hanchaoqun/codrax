package orchestrator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

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
		if doc := mut.AnswerDocument(); doc != nil {
			// B8-T1 (block_only_carrier.md §5.8) — V1 ShapeText
			// removed; the contract checker now reads carrier-
			// neutral fields (Text, Citations, IsAbsence). The
			// V2 carrier path produces the same draft fields via
			// the V2 renderer's StageOutput.FinalAnswer.
			if len(doc.Citations) > 0 {
				draft.Citations = make([]contract.Citation, 0, len(doc.Citations))
				for _, c := range doc.Citations {
					draft.Citations = append(draft.Citations, contract.Citation{
						File: c.File,
						Line: c.Line,
					})
				}
			}
		}
	}
	// Commit 55 Batch C — wire SymbolOracle through to the contract
	// checker so must_include / must_exclude / acceptance(contains_symbol)
	// substring matches get supplementary oracle validation. nil
	// oracle (no graph) preserves pre-commit-55 behaviour.
	var oracle types.SymbolOracle
	if mut != nil {
		if g, ok := mut.SearchGraph().(*repotypes.Graph); ok && g != nil {
			oracle = repomap.NewSymbolOracle(g)
		}
	}
	result := contract.CheckWithOracle(draft, c, oracle)

	// Commit 53 P2 — Answer Shape Oracle. After the contract.Check
	// suite, run additional read-mode-only coherence checks that need
	// the full IR (not just the contract). These produce new
	// ViolationKind values (shape_intent_mismatch / sub_topic_count_
	// mismatch) that downstream sees as ordinary violations. Soft by
	// default at the gate-layer (see softViolationKinds below); the
	// Append-to-closure path is unchanged.
	// B8-T4 (block_only_carrier.md §5.8, 2026-05-03): V1 oracle
	// dispatch block deleted. emit_answer_document V1 schema is
	// rejected at runtime since B8-T3, so mut.AnswerDocument()
	// returns nil on every read-mode finalize. The V2 dispatch
	// block below is the sole oracle driver. The V1-only oracles
	// (Intent / Subject / PredicateAxis / FacetCoverage / ClaimForm
	// / AbsenceScope / Richness / StepIdentifier /
	// ValueSecondaryCitation / SymbolAnchorTrack /
	// StructuralEnumerationDivergence) are deleted with this block;
	// V2 siblings (runV2BlockOracles +
	// runStructuralEnumerationDivergenceOracleV2 +
	// runSymbolAnchorTrackOracleV2) cover the equivalent typed
	// axes via BlockRequirement.FacetIDs +
	// BlockRequirement.AcceptableClaimForms +
	// BlockRequirement.SurfaceRoleHint.
	if mut != nil {

		// B4 V2 block-only carrier validators (block_only_carrier.md
		// §5.4). When the LLM emitted document_model="v2",
		// AnswerDocumentV2 is non-nil on Mutable; we run the 4 V2
		// validators against it. SOFT-by-default during B4-B5
		// (telemetry-only). B6 will promote them to STRICT.
		//
		// Sibling block to the V1 path — V2 docs are a separate
		// carrier; the two oracles never run on the same document.
		// Per the carrier-mutual-exclusion invariant set up in B3,
		// AnswerDocument() and AnswerDocumentV2() are never both
		// non-nil on the same Mutable.
		if docV2 := mut.AnswerDocumentV2(); docV2 != nil && o != nil && o.busCtx != nil {
			// B7-T2 V1/V2 dispatch telemetry — one debug line per
			// contract check so operators can confirm the V2 path
			// fired (and which family) when grepping
			// [trace/v1v2_diff]. Live as long as both carriers
			// coexist; B8-T7 removes when V1 disappears.
			view := types.BuildAnswerSemanticViewForBusContext(o.busCtx)
			family := types.QuestionFamily("")
			requiredCount := 0
			if view != nil {
				family = view.Family
				requiredCount = len(view.RequiredBlocks)
			}
			logging.Debug("[trace/v1v2_diff] carrier=v2 family=%s blocks=%d required_block_kinds=%d v2_default=%v v1_strict=%v",
				family, len(docV2.Blocks), requiredCount, EmitV2Default(), V1OracleStrictMode())
			if view != nil {
				result.Violations = append(result.Violations,
					runV2BlockOracles(docV2, view)...)
			}
			// B5-T4: V2 adaptation of the structural-enumeration
			// divergence oracle. Same precise typed-graph signal as
			// the V1 path; consumes V2 blocks instead of doc.Symbols.
			if rmFull := mut.RequestModel(); rmFull != nil {
				result.Violations = append(result.Violations,
					runStructuralEnumerationDivergenceOracleV2(docV2, rmFull, mut)...)
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

	// B8-T4 (block_only_carrier.md §5.8, 2026-05-03): Commit 62
	// self-consistency reviewer dispatch deleted. The reviewer
	// consumed V1 doc.Summary + doc.Steps; V1 emit is rejected
	// at runtime since B8-T3, so the reviewer would never fire.
	// V2 carrier carries an equivalent structure (block-kind=summary
	// + body blocks); a V2 reviewer can be re-added later as a
	// separate session. Reviewer field remains on Orchestrator
	// (used only by tests today); a future session prunes it.

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
		Kind:   types.ViolExternalArtifactUnderdecoded,
		Detail: res.Detail,
		Repair: res.Detail,
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "external_artifact_decoded",
			Reason:     "answer underdecodes triaged log / perf trace",
			Confidence: 0.7,
		},
		Stage: string(types.StageFinalize),
	}}
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
	doc := mut.AnswerDocument()
	if doc == nil || len(doc.Citations) == 0 {
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
	for _, cit := range doc.Citations {
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

	// Doc-level caveat present (system tag inside) → renderer ran
	// successfully → the user sees the hedging signal. Pass. We grep
	// for the system-private tag rather than the user-visible prefix
	// "Authority: " so an LLM-written caveat that happens to start
	// with that prefix isn't mistaken for the system's signal.
	if strings.Contains(draftText, render.AuthorityCaveatTag()) {
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
		Repair: "Re-emit emit_answer_document with all required fields populated for the target shape (see the skill's Required-field dispatch table). The system will re-attach the drift disclosure automatically once the structured emit succeeds — do not write the disclosure prose yourself.",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_authority",
			Reason:     "drift disclosure missing on rendered answer (likely structured-emit failure)",
			Confidence: 0.6,
		},
		Stage: string(types.StageFinalize),
	}}
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
// runSymbolAnchorTrackOracleV2 is the V2 carrier adaptation of
// runSymbolAnchorTrackOracle (B5-T5 落地). Same precise rejection-
// counter signal as the V1 path; reads V2 blocks instead of
// doc.Symbols / doc.Steps to compute the principal-anchor count.
//
// Mapping V2 → V1-equivalent counts:
//   - "Got symbols" = sum of items[] across BlockOrderedList,
//     BlockBulletList, BlockTable (each item is one principal
//     anchor in V2's vocabulary).
//   - "Got steps" = items[] count of BlockOrderedList specifically
//     (V1's StepList shape becomes a BlockOrderedList in V2).
//
// "Required shape" is derived from view.Family + the family's
// primary required block kind (mirrors the V1 oracle's
// requiredShape parameter).
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
			gotSteps += len(blk.Items)
			gotSymbols += len(blk.Items)
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
		Repair: "the next investigation pass should re-explore around the principal entities the rejected items pointed to, with read_file slices that cover each entity's definition line.",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "explorer_def_region_coverage_v2",
			Reason:     "repeated emit_answer_symbol rejections at line-anchor verifier (V2 carrier)",
			Confidence: 0.7,
		},
		Stage: string(types.StageExtract),
	}}
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
// any block.Title strings (used by the V1 oracle's
// strings.Contains check unchanged — still verbatim substring
// match per R2).
func runStructuralEnumerationDivergenceOracleV2(docV2 *types.AnswerDocumentV2, rm *types.RequestModel, mut *types.MutableState) []types.Violation {
	if docV2 == nil || rm == nil || mut == nil {
		return nil
	}
	if !rm.Predicates.IsCategoryEnumeration {
		return nil
	}
	graph, ok := mut.SearchGraph().(*repotypes.Graph)
	if !ok || graph == nil {
		return nil
	}
	candidateEntities := append([]string(nil), rm.AnalyzerHints.PrimaryEntities...)
	candidateEntities = append(candidateEntities, rm.AnalyzerHints.Entities...)
	var ifaceName string
	var typedImpl []*repotypes.Symbol
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
// haystack — matching the V1 oracle's strings.Contains semantics.
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

// EmitV2Default / SetEmitV2Default / V1OracleStrictMode /
// SetV1OracleStrictMode (B6, 2026-05-03) live in internal/types so
// agent / render / orchestrator can all read them without import
// cycles. The orchestrator package re-exports them as thin wrappers
// for callers that already have orchestrator imported.

// EmitV2Default returns types.EmitV2Default() — V2 carrier default
// gate. See internal/types/answer_document_v2.go for semantics.
func EmitV2Default() bool { return types.EmitV2Default() }

// SetEmitV2Default flips the gate. cmd/root.go calls this at startup.
func SetEmitV2Default(on bool) { types.SetEmitV2Default(on) }

// V1OracleStrictMode returns types.V1OracleStrictMode() — rollback
// rope to restore V1 oracle strict semantics during V2 default.
func V1OracleStrictMode() bool { return types.V1OracleStrictMode() }

// SetV1OracleStrictMode flips the rope. cmd/root.go startup.
func SetV1OracleStrictMode(on bool) { types.SetV1OracleStrictMode(on) }

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
//
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// defaultSoftKinds is the set of ViolationKinds that, by default,
// do NOT hard-fail the contract gate (they are mirrored to closure
// for telemetry / future learning but don't trigger finalize retry).
// The pre-commit-53 violation kinds (ViolShape / ViolCitation / ...)
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
func defaultSoftKinds() map[types.ViolationKind]bool {
	return map[types.ViolationKind]bool{
		types.ViolShapeIntentMismatch:          true,
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
func isSoftViolationKind(k types.ViolationKind) bool {
	softKindsMu.RLock()
	defer softKindsMu.RUnlock()
	return softViolationKinds[k]
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
	doc := mut.AnswerDocument()
	// Declarative path — the LLM called emit_investigation_complete
	// with an absence_justification saying "this is an honest zero
	// with nothing to cite." We trust the claim but still audit that
	// the explorer ran at least one investigation-class tool; a zero-
	// tool "I didn't look and declared absence" run is rejected.
	// This rescues explanation-shape absence answers (e.g. a prose
	// sentence "There are no Python files in this repo") whose
	// structural shape would otherwise fail isAbsenceShape and make
	// the citation-floor gate fire with no possible repair.
	if strings.TrimSpace(mut.StableAbsenceJustification()) != "" {
		return hasAnyInvestigationSuccess(mut)
	}
	// Structural path — the finalized document's shape itself reads
	// as zero (empty symbols + complete, literal "0"/"none"/"zero",
	// boolean=false). Audit depth is shape-tiered: shallow shapes
	// accept any one investigation tool; deep shapes require a real
	// content read.
	if !isAbsenceShape(doc) {
		return false
	}
	return hasInvestigationEvidence(mut, doc)
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
	for _, r := range ta.ToolResults {
		if !r.Success {
			continue
		}
		if investigationToolKinds[r.ToolName] {
			return true
		}
	}
	return false
}

func isAbsenceShape(doc *types.AnswerDocument) bool {
	if doc == nil {
		return false
	}
	switch doc.Shape {
	case types.ShapeListOfSymbols:
		// Empty symbols slate is an honest "zero" signal as long as
		// the completeness is NOT lower_bound (lower_bound on [] is
		// self-contradictory and is rejected upstream in
		// emit_answer_symbol anyway). Both CompletenessComplete ("I
		// enumerated every match and there are none") and
		// CompletenessUnknown ("extractor skipped emit_answer_symbol
		// entirely because the LLM found nothing") are valid ways a
		// real LLM expresses zero on a list_of_symbols question; the
		// earlier "must be Complete" rule rejected the common
		// "nothing matched, no emit" path and made count/existence
		// questions mis-shaped by the analyzer unrecoverable.
		if len(doc.Symbols) != 0 {
			return false
		}
		return doc.SymbolsCompleteness != types.CompletenessLowerBound
	case types.ShapeValue, types.ShapeConfigValue:
		if doc.Value == nil {
			return false
		}
		lit := strings.ToLower(strings.TrimSpace(doc.Value.Literal))
		return isZeroLiteral(lit)
	case types.ShapeBoolean:
		return doc.Boolean != nil && !doc.Boolean.Decision
	}
	return false
}

// isZeroLiteral recognises the common cross-language ways a finalizer
// expresses "no such value": empty string, "0", "none", "null", "nil",
// "无" and "没有" (Chinese), "zero". Kept deliberately short so every
// entry is one a real LLM has been seen to produce.
func isZeroLiteral(lit string) bool {
	switch lit {
	case "", "0", "none", "null", "nil", "无", "没有", "zero":
		return true
	}
	return false
}

// investigationToolKinds is the set of tools whose successful
// invocation counts as "the explorer did real work". Excludes
// orchestration / transport tools (propose_sub_agents, emit_* family).
var investigationToolKinds = map[string]bool{
	"grep":         true,
	"exec_command": true,
	"list_files":   true,
	"read_file":    true,
	"repo_map":     true,
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
func isShallowShape(s types.AnswerShape) bool {
	switch s {
	case types.ShapeValue, types.ShapeBoolean, types.ShapeConfigValue:
		return true
	}
	return false
}

// isContentRead reports whether a tool result represents the LLM
// actually reading file content (as opposed to enumerating names).
// read_file always qualifies; grep only when its summary carries
// line-bearing matches (not the "[grep: N matching files]"
// files_only shape).
func isContentRead(r types.ToolResult) bool {
	switch r.ToolName {
	case "read_file":
		return true
	case "grep":
		// files_only / no-match summaries advertise themselves with a
		// bracketed prefix or the literal "no matches" phrase. Both
		// are name-only signals and do not prove the LLM read code.
		if strings.HasPrefix(r.Summary, "[grep:") && strings.Contains(r.Summary, "matching files]") {
			return false
		}
		if strings.Contains(r.Summary, "no matches") {
			return false
		}
		return true
	}
	return false
}

// hasInvestigationEvidence reports whether Turn A did enough work to
// back an absence claim in the given shape. Two-tier rule:
//
//	shallow shape — any single successful investigation-class tool
//	                call is sufficient. Rejects the "zero tools,
//	                pure guess" failure mode and nothing else.
//	deep shape    — at least one actual content read (read_file or
//	                line-bearing grep). Rejects "I listed the files
//	                and claim nothing inside does X" without looking.
func hasInvestigationEvidence(mut *types.MutableState, doc *types.AnswerDocument) bool {
	if mut == nil {
		return false
	}
	ta := mut.TurnAArtifacts()
	if ta == nil {
		return false
	}
	// Empty list_of_symbols audits as shallow: the claim IS the
	// emptiness ("zero matches"), not a behaviour assertion over
	// candidate handlers. A `find` / `grep files_only` / `list_files`
	// one-shot proves the count directly; opening any file adds no
	// information. Only non-empty list_of_symbols (enumerations +
	// behaviour claims) needs content-read audit.
	emptyListAbsence := doc != nil && doc.Shape == types.ShapeListOfSymbols && len(doc.Symbols) == 0
	shallow := emptyListAbsence || (doc != nil && isShallowShape(doc.Shape))
	for _, r := range ta.ToolResults {
		if !r.Success {
			continue
		}
		if !investigationToolKinds[r.ToolName] {
			continue
		}
		if shallow {
			return true
		}
		if isContentRead(r) {
			return true
		}
	}
	return false
}

func isDriftBoundedCitationAnswer(bus *types.BusContext, out *agent.StageOutput) bool {
	if bus == nil || bus.AnalysisIR == nil || bus.Mutable == nil {
		return false
	}
	doc := bus.Mutable.AnswerDocument()
	if doc == nil {
		return false
	}
	if doc.Shape != types.ShapeExplanation && doc.Shape != types.ShapeStepList {
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
	// The Composer accepts []types.Violation and contract.Violation
	// is a type alias, so the slice passes through without copying.
	h, _ := hintComposerSingleton.Compose(hint.Context{}, []types.Violation(res.Violations))
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
//  1. Mutable.AnswerDocument().Citations — populated by
//     emit_answer_document.Execute after grounding + remap. This is
//     the exact pool the renderer consults.
//  2. extractCitationsFromAnswer(out.FinalAnswer) — legacy text-regex
//     fallback when the AnswerDocument was never set (test harnesses
//     that route directly through StageOutput.FinalAnswer).
//
// The regex fallback under-counts on list_of_symbols / step_list
// because those shapes inline citations into per-row renders and do
// not emit the pool as a bulleted list. Using the pool count fixes
// the "4 citations but orchestrator says 1" class of bugs.
func finalizerCitationPoolSize(mut *types.MutableState, out *agent.StageOutput) int {
	if mut != nil {
		if doc := mut.AnswerDocument(); doc != nil && len(doc.Citations) > 0 {
			return len(doc.Citations)
		}
	}
	if out != nil {
		return len(extractCitationsFromAnswer(out.FinalAnswer))
	}
	return 0
}

// appendViolationsToAnswer prepends a single visible warning line to
// the final answer text when the contract checker has exhausted its
// retry budget. The original answer is preserved beneath the warning
// so no information is lost — same fail-loud pattern P0.2 uses for
// shape-validator exhaustion. See feedback_honesty_over_cleverness.md.
func appendViolationsToAnswer(originalAnswer string, res contract.Result) string {
	if res.Passed || len(res.Violations) == 0 {
		return originalAnswer
	}
	var b strings.Builder
	b.WriteString("· answer-contract validation exhausted: ")
	b.WriteString(renderViolations(res))
	b.WriteString("\n\n")
	b.WriteString(originalAnswer)
	return b.String()
}
