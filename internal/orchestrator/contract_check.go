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
		if doc := mut.AnswerDocument(); doc != nil {
			draft.ShapeText = shapeTextForContractCheck(doc)
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
	if mut != nil {
		ir := mut.RequestModel()
		if doc := mut.AnswerDocument(); doc != nil && ir != nil {
			result.Violations = append(result.Violations,
				runAnswerShapeOracle(doc, ir, mut)...)
			// Block 2 (architecture overhaul 2026-05-02) — Intent /
			// Subject / PredicateAxis oracles. Each runs after the
			// existing Shape oracle so the violation set carries
			// breadth ("shape" + "intent depth" + "subject anchor"
			// + "axis evidence"). All SOFT-by-default; promotion
			// via pipeline_contract_strict_kinds.
			result.Violations = append(result.Violations,
				runIntentCoverageOracle(doc, ir)...)
			result.Violations = append(result.Violations,
				runSubjectAnchorOracle(doc, ir)...)
			result.Violations = append(result.Violations,
				runPredicateAxisOracle(doc, ir, mut)...)
			// Phase 4 (Semantic Surface Contract, 2026-05-02) —
			// FacetCoverage / ClaimForm / Absence-Scope oracles.
			// All three consume Phase-1 FacetCoverageContract +
			// Phase-3 RenderedClaimUse annotations. SOFT/STRICT
			// classification per defaultSoftKinds(); promotion via
			// pipeline_contract_strict_kinds. Master-switch:
			// pipeline_facet_validators_enabled (default true).
			if FacetValidatorsEnabled() {
				result.Violations = append(result.Violations,
					runFacetCoverageOracle(doc, ir, mut)...)
				result.Violations = append(result.Violations,
					runClaimFormSupportOracle(doc, mut)...)
				result.Violations = append(result.Violations,
					runAbsenceScopeBoundOracle(doc, ir)...)
				// Phase 5 (2026-05-02) — telemetry-only richness
				// regression oracle. SOFT-only ViolRichnessRegression
				// entries flow into the closure ledger and surface
				// in the end-of-Run [CGEC] summary's
				// optional_facets_covered counter. No retry, no fail.
				result.Violations = append(result.Violations,
					runRichnessTelemetryOracle(doc, ir, mut)...)
				// Phase 4 extension (2026-05-02) — step-identifier
				// backed-by-evidence oracle. Fires when AnswerStep
				// inline-code identifiers don't appear in the typed
				// evidence pool (Subject / Object / AnchorSymbol /
				// AnswerSymbol.Name). Catches s1a-class hallucination
				// where the LLM remembers a position but invents
				// names. SOFT-by-default; BackToExplore on retry.
				result.Violations = append(result.Violations,
					runStepIdentifierBackedByEvidenceOracle(doc, mut)...)
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

	// Commit 62 — self-consistency reviewer (read-mode mirror of
	// reflector model-vs-model review pattern). Independent LLM
	// reads doc.Summary + body bullets and detects FACTUAL
	// contradictions between them. Default this-phase: enabled +
	// rewrite_on_contradiction=true → contradictions land as
	// strict violations triggering finalizer retry. Soft mode
	// preserves the answer for the user with telemetry-only
	// recording. Skipped silently when reviewer not wired.
	if o != nil && o.selfConsistencyReviewer != nil && mut != nil {
		if doc := mut.AnswerDocument(); doc != nil && shouldReviewConsistency(doc) {
			result.Violations = append(result.Violations,
				o.runSelfConsistencyReview(doc, mut)...)
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

// runAnswerShapeOracle applies the read-mode Answer Shape Oracle
// (commit 53 P2): structural cross-checks between the finalized
// AnswerDocument and the analyzer's RequestModel that the contract
// schema cannot express. Returns a (possibly empty) violations slice.
//
// Two checks fire:
//
//   - Shape ↔ Intent coherence: a ShapeValue answer for an
//     IntentExplain / IntentRootCause request is suspicious — the
//     analyzer asked for prose, the finalizer produced a scalar.
//     Vice versa: ShapeExplanation for IntentCount is suspicious.
//
//   - SubTopic count mismatch: when the analyzer declared N
//     sub-topics, the doc.AnswerSymbols (when present) should cover
//     close to N distinct buckets. A 3-sub-topic request answered
//     with 1 symbol or 10 symbols is a coverage mismatch the
//     analyzer should re-examine.
//
// Both checks are SOFT by default (added to ViolDiagramIdentifier-
// /ViolSubTopicCountMismatch families that gate-soft-list excludes
// from Passed=false hard-fail). Operators promote to strict via
// gate_contract_strict_kinds yaml.
// runExternalArtifactDecodedCheck is the orchestrator-side oracle
// for CritExternalArtifactDecoded. Triggers structurally on
// (mut.LogTriage() != nil OR mut.PerfTrace() != nil); fails when
// draft text references fewer than the configured floor of
// bundle-extracted non-path tokens.
//
// Returns at most one Violation per call. Vacuously satisfied when
// no bundle is attached or both bundles produced zero decodable
// tokens — read-mode runs without --log / --htrace pay no cost.
//
// Wraps the criterion-package evaluator (which holds the canonical
// token-collection + threshold logic) so this oracle and the
// scheduler's other criterion.Eval call sites cannot drift on the
// definition of "decoded".
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

func runAnswerShapeOracle(doc *types.AnswerDocument, rm *types.RequestModel, mut *types.MutableState) []types.Violation {
	if doc == nil || rm == nil {
		return nil
	}
	var out []types.Violation

	// Commit 55 Batch A.3 — declared-count drift. The extractor's
	// emit_answer_symbol stamps the LLM's self-declared count on
	// Mutable; if the finalizer's rendered doc.Symbols length
	// diverges (mid-loop grounding stripped some, the finalizer
	// re-derived a different shape, etc.), the count claim was
	// silently invalidated. Soft-by-default — we don't want a
	// post-emit grounder strip to hard-fail an otherwise good
	// answer, but we DO want the reviewer to learn about the drift.
	if mut != nil {
		declared := mut.EmittedAnswerSymbolDeclaredCount()
		if declared > 0 && len(doc.Symbols) != declared {
			out = append(out, types.Violation{
				Kind: types.ViolDeclaredCountDrift,
				Detail: fmt.Sprintf("emit_answer_symbol declared count=%d but finalizer rendered %d doc.Symbols (post-emit drift)",
					declared, len(doc.Symbols)),
				Repair: fmt.Sprintf("re-emit emit_answer_symbol with count=%d to match the surviving slate after grounder strip, OR re-investigate to recover the dropped %d items if they are load-bearing",
					len(doc.Symbols), declared-len(doc.Symbols)),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_symbol",
					Reason:     "declared item count diverges from rendered slate",
					Confidence: 0.7,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}

	// Plan D rollout 2026-05-02 — step_list lower-bound for
	// RequestedEnumerationBoundary. The validator at emit time
	// rejects upper-bound overflow (principal_count > DeclaredCount).
	// The lower bound (principal_count < DeclaredCount) is SOFT here:
	// we don't hard-reject, because the LLM may have honestly only
	// found N-1 principal items even though the question quoted N
	// (e.g. user said "the 9 X's" but only 8 actually exist). The
	// reviewer learns the gap via this Violation and the final answer
	// ships with caveat header. Stage=finalize.
	//
	// Skips when EnumerationBoundary is absent OR DeclaredCount<=0
	// OR shape is anything but step_list.
	if rm.EnumerationBoundary != nil &&
		rm.EnumerationBoundary.DeclaredCount > 0 &&
		doc.Shape == types.ShapeStepList {
		boundary := rm.EnumerationBoundary
		principal := types.PrincipalStepCount(doc.Steps)
		if principal < boundary.DeclaredCount {
			out = append(out, types.Violation{
				Kind: types.ViolDeclaredCountDrift,
				Detail: fmt.Sprintf("user asked for %d principal item(s) (`%s`) but your steps[] has %d kind=principal step(s) (total %d steps; flow=%d, caveat=%d)",
					boundary.DeclaredCount, boundary.SourceQuote, principal, len(doc.Steps),
					types.CountStepsOfKind(doc.Steps, types.AnswerStepKindFlow),
					types.CountStepsOfKind(doc.Steps, types.AnswerStepKindCaveat)),
				Repair: fmt.Sprintf("either (a) re-emit emit_answer_document with %d kind=principal step(s) to match the question's bound, OR (b) when only %d principal items genuinely exist, leave Steps as-is and adjust the summary prose to acknowledge the mismatch (\"the question quoted N but only K independent items exist\")",
					boundary.DeclaredCount, principal),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_steps",
					Reason:     "principal step count below declared question boundary",
					Confidence: 0.7,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}

	// Shape ↔ Intent coherence.
	switch rm.Intent {
	case types.IntentExplain, types.IntentRootCause:
		// Explanation requests should produce explanation-class
		// shapes (Explanation / List). A pure value/boolean
		// answer is structurally inconsistent.
		switch doc.Shape {
		case types.ShapeValue, types.ShapeBoolean, types.ShapeConfigValue:
			out = append(out, types.Violation{
				Kind: types.ViolShapeIntentMismatch,
				Detail: fmt.Sprintf("intent=%s expects explanation-class shape; finalizer emitted shape=%s",
					rm.Intent, doc.Shape),
				Repair: fmt.Sprintf("re-emit emit_answer_document with shape=explanation (or list_of_symbols when the answer enumerates) — the user asked %q which expects prose, not a scalar",
					rm.Intent),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_shape",
					Reason:     "intent declares explanation; shape declares scalar",
					Confidence: 0.6,
				},
				Stage: string(types.StageFinalize),
			})
		}
	case types.IntentReturnValue, types.IntentConfigQuery:
		// Value-return / config-query requests want a scalar
		// answer. An explanation shape is structurally
		// inconsistent — the user asked "what's the value" not
		// "explain why".
		if doc.Shape == types.ShapeExplanation {
			out = append(out, types.Violation{
				Kind: types.ViolShapeIntentMismatch,
				Detail: fmt.Sprintf("intent=%s expects value-class shape; finalizer emitted shape=explanation",
					rm.Intent),
				Repair: fmt.Sprintf("re-emit emit_answer_document with shape=value or shape=config_value (whichever matches the question's literal type) — the user asked %q which expects a scalar/key-value, not prose",
					rm.Intent),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_shape",
					Reason:     "intent declares scalar; shape declares explanation",
					Confidence: 0.6,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}

	// SubTopic count check. Only meaningful when the analyzer
	// emitted >= 2 sub-topics AND the doc has emit_answer_symbol
	// rows; otherwise the check is moot.
	if len(rm.SubTopics) >= 2 && len(doc.Symbols) > 0 {
		distinctBuckets := countDistinctAnswerSymbolBuckets(doc.Symbols)
		expected := len(rm.SubTopics)
		// Tolerance: ±1 sub-topic absorbed (analyzer over/under
		// segmentation is common). A 3-topic request with 2 or 4
		// distinct buckets is fine; with 1 or 5+ it's a mismatch.
		if abs(distinctBuckets-expected) > 1 {
			repair := fmt.Sprintf("widen the answer to cover all %d sub-topics: emit one or more emit_answer_symbol items per sub-topic so doc.Symbols spans %d distinct files/symbols", expected, expected)
			if distinctBuckets > expected+1 {
				repair = fmt.Sprintf("the answer over-decomposed (%d buckets vs %d declared sub-topics) — collapse near-duplicate items into one bucket, or re-classify in the analyzer to recognise the additional sub-topics", distinctBuckets, expected)
			}
			out = append(out, types.Violation{
				Kind: types.ViolSubTopicCountMismatch,
				Detail: fmt.Sprintf("analyzer declared %d sub-topics; answer covers %d distinct buckets",
					expected, distinctBuckets),
				Repair: repair,
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "sub_topics",
					Reason:     "answer-symbol bucket count diverges from analyzer's sub-topic count",
					Confidence: 0.5,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}

	return out
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
// All produced violations are SOFT-by-default (cmd/root.go default
// strict-kinds excludes them). Block 3 routes them to selective
// fallback targets — IntentTrace/IntentRootCause/IntentConfig miss →
// BackToExplore (need more evidence); IntentEnumerate miss →
// FinalizerOnly (re-emit with the right shape).
func runIntentCoverageOracle(doc *types.AnswerDocument, rm *types.RequestModel) []types.Violation {
	if doc == nil || rm == nil {
		return nil
	}
	var out []types.Violation
	switch rm.Intent {
	case types.IntentTrace:
		if !traceAnswerHasDepth(doc) {
			out = append(out, types.Violation{
				Kind: types.ViolIntentTraceShallow,
				Detail: fmt.Sprintf(
					"intent=trace expects a multi-hop trace surface; finalizer emitted shape=%s with %d step(s) and %s sequenceDiagram block",
					doc.Shape, len(doc.Steps), boolToHasOrNot(hasSequenceDiagram(doc.Summary), "a", "no")),
				Repair: "re-emit emit_answer_document for this trace question with EITHER (a) steps[] containing >=2 hops, each step naming the file:line where the chain advances, OR (b) summary embedding a fenced sequenceDiagram block tracing the chain (`Actor->>Other: call`).",
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_shape",
					Reason:     "intent declares trace; answer surface lacks multi-hop depth",
					Confidence: 0.55,
				},
				Stage: string(types.StageFinalize),
			})
		}
	case types.IntentEnumerate:
		if !enumerateAnswerHasList(doc) {
			out = append(out, types.Violation{
				Kind: types.ViolIntentEnumerateNotList,
				Detail: fmt.Sprintf(
					"intent=enumerate expects a list surface; finalizer emitted shape=%s with %d step(s) and %d symbol(s)",
					doc.Shape, len(doc.Steps), len(doc.Symbols)),
				Repair: "re-emit emit_answer_document with shape=list_of_symbols and >=2 items in symbols[] (one per enumerated value), OR shape=step_list with >=2 entries each naming one value.",
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_shape",
					Reason:     "intent declares enumerate; answer surface is not a list",
					Confidence: 0.6,
				},
				Stage: string(types.StageFinalize),
			})
		}
	case types.IntentRootCause:
		if len(doc.Citations) == 0 {
			out = append(out, types.Violation{
				Kind:   types.ViolIntentRootCauseNoCause,
				Detail: "intent=root_cause expects at least one citation naming the causal anchor; finalizer emitted zero citations.",
				Repair: "re-emit emit_answer_document with at least one citation in citations[] pointing at the file:line where the cause is implemented (the failing branch, the buggy line, the missing guard).",
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_root_cause_anchor",
					Reason:     "intent declares root_cause; answer carries no grounded cause site",
					Confidence: 0.7,
				},
				Stage: string(types.StageFinalize),
			})
		}
	case types.IntentConfigQuery:
		if !configQueryAnswerHasTrail(doc) {
			out = append(out, types.Violation{
				Kind: types.ViolIntentConfigNoTrail,
				Detail: fmt.Sprintf(
					"intent=config_query expects shape=value/config_value OR at least one citation in a config-shaped path (yaml/json/toml/ini/.env/shell rc); finalizer emitted shape=%s with %d citation(s)",
					doc.Shape, len(doc.Citations)),
				Repair: "re-emit emit_answer_document with EITHER (a) shape=value or shape=config_value with the literal in value{} and citation_ref pointing at the defining line, OR (b) keep the current shape but add at least one citation pointing at a config file (yaml/json/toml/ini/.env/shell rc).",
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_config_trail",
					Reason:     "intent declares config_query; answer surface lacks the canonical literal-or-config-citation pair",
					Confidence: 0.55,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}
	return out
}

// traceAnswerHasDepth reports whether a trace-class answer surface
// has structural depth — either >=2 PRINCIPAL step hops or a
// sequenceDiagram fenced block in summary.
//
// Plan D rollout (2026-05-02): counts only kind=principal steps, so
// a 1-principal + 1-flow answer no longer falsely satisfies "trace
// has depth". Flow narration adds reader continuity but is not
// itself a hop in the trace.
func traceAnswerHasDepth(doc *types.AnswerDocument) bool {
	if doc == nil {
		return false
	}
	if types.PrincipalStepCount(doc.Steps) >= 2 {
		return true
	}
	return hasSequenceDiagram(doc.Summary)
}

// enumerateAnswerHasList reports whether an enumeration answer
// renders as a list surface.
//
// Plan D rollout: principal-only count, mirroring traceAnswerHasDepth.
func enumerateAnswerHasList(doc *types.AnswerDocument) bool {
	if doc == nil {
		return false
	}
	if doc.Shape == types.ShapeListOfSymbols {
		return true
	}
	if doc.Shape == types.ShapeStepList && types.PrincipalStepCount(doc.Steps) >= 2 {
		return true
	}
	return false
}

// configQueryAnswerHasTrail reports whether a config-query answer
// has either a value/config_value shape OR at least one citation
// pointing at a config-shaped file path.
func configQueryAnswerHasTrail(doc *types.AnswerDocument) bool {
	if doc == nil {
		return false
	}
	if doc.Shape == types.ShapeValue || doc.Shape == types.ShapeConfigValue {
		return true
	}
	for _, c := range doc.Citations {
		if isConfigShapedPath(c.File) {
			return true
		}
	}
	return false
}

// hasSequenceDiagram reports whether the summary contains a
// sequenceDiagram fenced block. Lookup is purely structural —
// matches "```mermaid" followed by "sequenceDiagram" within the
// same fence body. No keyword matching on prose text.
func hasSequenceDiagram(summary string) bool {
	if summary == "" {
		return false
	}
	// Find `+"```"+`mermaid…sequenceDiagram… within the same fenced block.
	fenceIdx := strings.Index(summary, "```mermaid")
	for fenceIdx >= 0 {
		body := summary[fenceIdx+len("```mermaid"):]
		closeIdx := strings.Index(body, "```")
		if closeIdx < 0 {
			break
		}
		if strings.Contains(body[:closeIdx], "sequenceDiagram") {
			return true
		}
		next := strings.Index(summary[fenceIdx+1:], "```mermaid")
		if next < 0 {
			break
		}
		fenceIdx = fenceIdx + 1 + next
	}
	return false
}

// isConfigShapedPath reports whether the citation file path looks
// like a runtime config artifact (yaml/json/toml/ini/.env/shell rc).
// Pure extension/basename check — no path heuristics, no token
// matching. Used by the IntentConfigQuery oracle's secondary
// signal.
func isConfigShapedPath(file string) bool {
	if file == "" {
		return false
	}
	lower := strings.ToLower(file)
	switch {
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return true
	case strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".json5"):
		return true
	case strings.HasSuffix(lower, ".toml"), strings.HasSuffix(lower, ".ini"):
		return true
	case strings.HasSuffix(lower, ".env"):
		return true
	case strings.HasSuffix(lower, ".conf"), strings.HasSuffix(lower, ".cfg"):
		return true
	}
	// Common shell rc and dotfile basenames.
	base := lower
	if i := strings.LastIndexByte(lower, '/'); i >= 0 {
		base = lower[i+1:]
	}
	switch base {
	case ".env", ".envrc", ".bashrc", ".zshrc", ".profile", ".bash_profile":
		return true
	}
	return false
}

// boolToHasOrNot is a tiny presentation helper for the
// IntentTraceShallow Detail string. Renders:
//
//	hasSequenceDiagram(summary) → "a sequenceDiagram block"
//	!hasSequenceDiagram(summary) → "no sequenceDiagram block"
func boolToHasOrNot(b bool, hasArticle, missingWord string) string {
	if b {
		return hasArticle
	}
	return missingWord
}

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
// Pure structural — checks IR-emitted Subject text against
// AnswerDocument structure. No question-text matching.
func runSubjectAnchorOracle(doc *types.AnswerDocument, rm *types.RequestModel) []types.Violation {
	if doc == nil || rm == nil {
		return nil
	}
	subj := rm.AnswerSubject
	if !concreteSubjectKind(subj.Kind) || subj.Confidence < subjectAnchorOracleFloor {
		return nil
	}
	// EntityAxes carries the LLM-emitted subject identifiers; they
	// may be empty when the LLM declared a Kind without naming
	// concrete tokens. Without identifiers, we cannot check —
	// abstain.
	if len(subj.EntityAxes) == 0 {
		return nil
	}
	if anyEntityAxisOnAnswerSurface(doc, subj.EntityAxes) {
		return nil
	}
	return []types.Violation{{
		Kind: types.ViolSubjectAnchorMissing,
		Detail: fmt.Sprintf(
			"answer_subject.kind=%s confidence=%.2f names %v but the rendered answer surfaces none of those identifiers in symbols[].name, steps[].anchor, or inline `code` in summary/steps",
			subj.Kind, subj.Confidence, subj.EntityAxes),
		Repair: fmt.Sprintf(
			"re-emit emit_answer_document so the rendered answer surfaces at least one of %v — either as a symbols[] entry, as the .anchor on at least one step, or as an inline backtick `code` mention in summary or step descriptions.",
			subj.EntityAxes),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_subject_anchor",
			Reason:     "subject named in IR is missing from the answer surface",
			Confidence: float64(subj.Confidence),
		},
		Stage: string(types.StageFinalize),
	}}
}

// concreteSubjectKind reports whether the AnswerSubjectKind names
// a structural identifier that the answer can be expected to
// expose verbatim. Generic / unknown subjects can't be checked.
func concreteSubjectKind(k types.AnswerSubjectKind) bool {
	switch k {
	case types.SubjectFunctionName,
		types.SubjectTypeName,
		types.SubjectHandlerRoute,
		types.SubjectConfigKey,
		types.SubjectStructField,
		types.SubjectInterface:
		return true
	}
	return false
}

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
// Pure structural extraction — the inline-code parser walks
// backtick pairs, no regex on prose words.
func anyEntityAxisOnAnswerSurface(doc *types.AnswerDocument, axes []string) bool {
	for _, sym := range doc.Symbols {
		for _, a := range axes {
			if sym.Name == a {
				return true
			}
		}
	}
	for _, step := range doc.Steps {
		for _, tok := range extractInlineCodeTokens(step.Description) {
			for _, a := range axes {
				if tok == a {
					return true
				}
			}
		}
	}
	for _, tok := range extractInlineCodeTokens(doc.Summary) {
		for _, a := range axes {
			if tok == a {
				return true
			}
		}
	}
	return false
}

// extractInlineCodeTokens pulls every backtick-delimited token out
// of prose. Walks the bytes, opens on a single backtick, closes on
// the next single backtick. Handles paired ``code`` (rare) by
// treating the inner content as one token. Skips fenced code blocks
// (triple-backtick) — those do not contribute "the LLM mentioned X
// inline" signal.
func extractInlineCodeTokens(prose string) []string {
	if prose == "" {
		return nil
	}
	var out []string
	i := 0
	for i < len(prose) {
		// Skip fenced blocks.
		if i+2 < len(prose) && prose[i] == '`' && prose[i+1] == '`' && prose[i+2] == '`' {
			end := strings.Index(prose[i+3:], "```")
			if end < 0 {
				return out
			}
			i += 3 + end + 3
			continue
		}
		if prose[i] != '`' {
			i++
			continue
		}
		// Single-backtick run.
		end := strings.IndexByte(prose[i+1:], '`')
		if end < 0 {
			return out
		}
		tok := strings.TrimSpace(prose[i+1 : i+1+end])
		if tok != "" {
			out = append(out, tok)
		}
		i += 1 + end + 1
	}
	return out
}

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
// SOFT-by-default. Block 3 routes to BackToExplore (more evidence
// of the right shape).
func runPredicateAxisOracle(doc *types.AnswerDocument, rm *types.RequestModel, mut *types.MutableState) []types.Violation {
	if rm == nil || mut == nil {
		return nil
	}
	axis := rm.PredicateAxis
	if axis == types.AxisUnknown {
		return nil
	}
	allowed := types.PredicateAxisToAnchorKinds(axis)
	if len(allowed) == 0 {
		return nil
	}
	closure := mut.EvidenceClosure()
	if closure == nil {
		return nil
	}
	evidence := closure.Stats() // touch to ensure init; real evidence lives elsewhere
	_ = evidence
	// Walk the evidence buffer through the answer document's
	// citations as an indirection — every citation surfaces an
	// evidence anchor, and the citation-pool already filtered to
	// the items the answer actually rests on. We check the
	// EmittedAnswerSymbol slate as well so step_list / list_of_
	// symbols answers (whose citations are folded into per-row
	// references) still get a fair check.
	hasMatch := false
	walkAnchorKinds(mut, func(k types.AnchorKind) bool {
		for _, want := range allowed {
			if k == want {
				hasMatch = true
				return true // stop early
			}
		}
		return false
	})
	if hasMatch {
		return nil
	}
	allowedNames := make([]string, 0, len(allowed))
	for _, k := range allowed {
		allowedNames = append(allowedNames, string(k))
	}
	return []types.Violation{{
		Kind: types.ViolPredicateAxisMissing,
		Detail: fmt.Sprintf(
			"predicate_axis=%s expects at least one evidence anchor of kind in %v; the answer's grounded evidence carries no matching anchor_kind",
			axis, allowedNames),
		Repair: fmt.Sprintf(
			"the next investigation pass should produce at least one evidence item with anchor_kind in %v (the action shape the question's verb implies). Current evidence is grounded but anchor-shape mismatched — definitions / assignments / etc are present, the asked-for kind is missing. If a finalizer dispatch sees this hint, re-emit emit_answer_document with the existing evidence verbatim — the system will route the next attempt to investigate further automatically.",
			allowedNames),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "predicate_axis_anchor",
			Reason:     "axis declared but no evidence with matching AnchorKind",
			Confidence: 0.55,
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
// Default classification: SOFT (missed-facet recovery typically needs
// new evidence collection rather than a pure finalizer rewrite).
// Promotion via pipeline_contract_strict_kinds.
func runFacetCoverageOracle(doc *types.AnswerDocument, rm *types.RequestModel, mut *types.MutableState) []types.Violation {
	if doc == nil || rm == nil {
		return nil
	}
	// Build the FacetCoverageContract directly from RequestModel +
	// the live evidence pool. Mirrors BuildAnswerSurfacePlan's call
	// site at types/answer_surface_plan.go:1207 but without the
	// surrounding shape-plan overhead — we only need the contract.
	var surfaceEvidence []types.EvidenceItem
	if mut != nil {
		surfaceEvidence = mut.EmittedEvidence()
	}
	contract := types.CompileFacetCoverage(*rm, surfaceEvidence)
	if contract == nil {
		return nil
	}
	hardRequired := []types.FacetRequirement(nil)
	for _, req := range contract.Required {
		if req.Required == types.FacetHardRequired {
			hardRequired = append(hardRequired, req)
		}
	}
	if len(hardRequired) == 0 {
		return nil
	}

	// Collect explicit claim_use facet IDs across all payload kinds.
	explicitFacets := map[string]bool{}
	collectClaimUseFacets(doc, func(c *types.RenderedClaimUse) {
		if c != nil && c.FacetID != "" {
			explicitFacets[c.FacetID] = true
		}
	})

	// Collect inferred coverage from citations whose underlying
	// evidence projects into one of each facet's AcceptableForms.
	inferredFormsPerCitation := inferClaimFormsFromCitations(doc, mut)

	var out []types.Violation
	for _, req := range hardRequired {
		if explicitFacets[string(req.Kind)] {
			continue
		}
		if facetCoveredByInferredForms(req, inferredFormsPerCitation) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolFacetUncovered,
			Detail: fmt.Sprintf(
				"facet=%s (HARD-required for %s) has no payload coverage — neither an explicit claim_use.facet_id reference nor an inferred match against acceptable_forms=%v",
				req.Kind, contract.Family, claimFormsAsStrings(req.AcceptableForms)),
			Repair: fmt.Sprintf(
				"the next investigation pass should surface evidence for facet %q so the answer can cite it. If a finalizer dispatch sees this hint and the evidence is already in the pool, attach a claim_use{facet_id=%q,evidence_id=...} to the relevant payload row.",
				req.Kind, req.Kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "facet_coverage",
				Reason:     fmt.Sprintf("facet %s declared HARD but uncovered", req.Kind),
				Confidence: 0.6,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

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
// No Run-time side effect. The closure ledger is the only writer;
// no answer prose is altered, no retry triggered, no hard fail.
func runRichnessTelemetryOracle(doc *types.AnswerDocument, rm *types.RequestModel, mut *types.MutableState) []types.Violation {
	if doc == nil || rm == nil {
		return nil
	}
	var surfaceEvidence []types.EvidenceItem
	if mut != nil {
		surfaceEvidence = mut.EmittedEvidence()
	}
	contract := types.CompileFacetCoverage(*rm, surfaceEvidence)
	if contract == nil || len(contract.Optional) == 0 {
		return nil
	}

	explicitFacets := map[string]bool{}
	collectClaimUseFacets(doc, func(c *types.RenderedClaimUse) {
		if c != nil && c.FacetID != "" {
			explicitFacets[c.FacetID] = true
		}
	})
	inferredFormsPerCitation := inferClaimFormsFromCitations(doc, mut)

	var out []types.Violation
	for _, req := range contract.Optional {
		if req.EffectiveTier() != types.TierEnrichment {
			continue
		}
		if explicitFacets[string(req.Kind)] {
			continue
		}
		if facetCoveredByInferredForms(req, inferredFormsPerCitation) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolRichnessRegression,
			Detail: fmt.Sprintf(
				"optional facet=%s (tier=enrichment for %s) not covered — the answer is correct but did not surface a richness facet the evidence pool could have supported",
				req.Kind, contract.Family),
			Repair: "Telemetry-only signal — no retry triggered, no action required. To improve answer richness on subsequent dispatches, add a step / symbol / sentence that surfaces the optional facet's content using an existing citation.",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "richness_tier",
				Reason:     fmt.Sprintf("optional facet %s uncovered", req.Kind),
				Confidence: 0.4,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

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
// Empty AnswerSymbol slate + empty EmittedEvidence + empty
// AnalyzerHints all skip silently; the oracle only fires when
// the typed pool has SOMETHING to compare against, otherwise it
// would penalise legitimate first-emit cases where the tools
// haven't run yet.
func runStepIdentifierBackedByEvidenceOracle(doc *types.AnswerDocument, mut *types.MutableState) []types.Violation {
	if doc == nil || len(doc.Steps) == 0 {
		return nil
	}
	verified := buildVerifiedIdentifierSet(doc, mut)
	if len(verified) == 0 {
		// No typed evidence to compare against — skip rather than
		// flag every backtick. Test-mode / no-tool-run paths land
		// here and stay quiet.
		return nil
	}
	var out []types.Violation
	// Walk every prose surface that may carry backtick-quoted
	// identifiers: AnswerStep.Description, AnswerSymbol.Rationale,
	// AnswerBoolean.Rationale, and AnswerDocument.Summary. Each
	// surface produces one Violation entry naming the carrier so
	// the operator can find the offending text quickly.
	check := func(label, prose string) {
		ids := extractBacktickIdentifiers(prose)
		if len(ids) == 0 {
			return
		}
		var missing []string
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			if verified[id] {
				continue
			}
			missing = append(missing, id)
		}
		if len(missing) == 0 {
			return
		}
		out = append(out, types.Violation{
			Kind: types.ViolStepIdentifierUnverified,
			Detail: fmt.Sprintf(
				"%s inline-code identifiers not present in typed evidence pool: %v. The verified-identifier set is built from EvidenceItem.Subject / Object / AnchorSymbol and AnswerSymbol.Name fields the explorer / extractor structurally emitted. Identifiers in the prose that have no typed-pool support are typically hallucinations — the LLM remembered a file:line position but invented the name at that position.",
				label, missing),
			Repair: fmt.Sprintf(
				"the next investigation pass should call emit_evidence with at least one of %v as Subject / Object / AnchorSymbol, OR the answer-rendering pass should rephrase %s to use only identifiers the typed evidence pool already observed (cross-check by reading the prior stage's structured evidence section before re-emitting).",
				missing, label),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "step_identifier_provenance",
				Reason:     fmt.Sprintf("%s uses %d unverified identifier(s)", label, len(missing)),
				Confidence: 0.65,
			},
			Stage: string(types.StageFinalize),
		})
	}
	for i := range doc.Steps {
		check(fmt.Sprintf("steps[%d].description", i), doc.Steps[i].Description)
	}
	for i := range doc.Symbols {
		check(fmt.Sprintf("symbols[%d].rationale", i), doc.Symbols[i].Rationale)
	}
	if doc.Boolean != nil {
		check("boolean.rationale", doc.Boolean.Rationale)
	}
	if doc.Summary != "" {
		check("summary", doc.Summary)
	}
	return out
}

// extractBacktickIdentifiers walks s and returns every
// identifier-shaped token enclosed in single backticks. A
// backtick pair contains an identifier when the entire content
// matches the closed identifier regex (Go-style:
// [A-Za-z_][A-Za-z0-9_]+, min 2 chars). Pairs containing
// non-identifier text (Chinese, punctuation, multi-token prose,
// etc.) are silently skipped — they're prose / decoration, not
// load-bearing identifiers.
func extractBacktickIdentifiers(s string) []string {
	var out []string
	const idRe = `[A-Za-z_][A-Za-z0-9_]+`
	idCheck := identifierRegex
	for {
		i := indexBacktick(s)
		if i < 0 {
			return out
		}
		j := indexBacktick(s[i+1:])
		if j < 0 {
			return out
		}
		j += i + 1
		token := s[i+1 : j]
		s = s[j+1:]
		_ = idRe
		// Single-token check: full content must match the
		// identifier pattern exactly. Multi-word backticks (e.g.
		// `` `var checks` ``) are NOT identifiers; skip.
		if idCheck.MatchString(token) && idCheck.FindString(token) == token {
			out = append(out, token)
		}
	}
}

// indexBacktick returns the index of the first single-backtick
// character in s, or -1 if none. Triple-backtick fences and
// double-backticks (used for backtick-containing literals) are
// treated as their own backticks for purposes of this scanner —
// stricter handling would require parsing markdown which is out
// of scope; the false-positive risk is low because triple-fence
// content is typically multi-line code blocks not inline IDs.
func indexBacktick(s string) int {
	return strings.IndexByte(s, '`')
}

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
// Why these four: each is a typed structural slot the
// explorer / extractor / analyzer populates with identifiers
// they observed. Including them gives the LLM a wide enough net
// that legitimate identifiers always pass; excluding raw file
// content keeps the oracle precise (no token-overlap noise).
func buildVerifiedIdentifierSet(doc *types.AnswerDocument, mut *types.MutableState) map[string]bool {
	out := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		// For multi-word values (Subject="foo bar baz"), tokenize
		// and add identifier-shaped sub-tokens too. Common pattern
		// in evidence summaries.
		for _, tok := range identifierTokenizer.FindAllString(s, -1) {
			if identifierRegex.MatchString(tok) {
				out[tok] = true
			}
		}
	}
	if mut != nil {
		for _, ev := range mut.EmittedEvidence() {
			add(ev.Subject)
			add(ev.Object)
			add(ev.AnchorSymbol)
		}
		if syms, _ := mut.EmittedAnswerSymbols(); syms != nil {
			for _, s := range syms {
				add(s.Name)
			}
		}
		if rm := mut.RequestModel(); rm != nil {
			for _, e := range rm.AnalyzerHints.MentionedEntities {
				add(e)
			}
		}
	}
	if doc != nil {
		for _, s := range doc.Symbols {
			add(s.Name)
		}
	}
	return out
}

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

func runClaimFormSupportOracle(doc *types.AnswerDocument, mut *types.MutableState) []types.Violation {
	if doc == nil {
		return nil
	}
	var out []types.Violation
	visit := func(label string, ref int, claim *types.RenderedClaimUse) {
		if claim == nil || claim.ClaimForm == "" {
			return
		}
		evidenceForm := claimFormForCitationRef(doc, mut, ref)
		if evidenceForm == "" || evidenceForm == types.ClaimUnknown {
			// No evidence projection available — cannot validate.
			// Phase 0 trace data showed 34% of evidence projects to
			// ClaimUnknown; treating that as "supported" is the
			// back-compat-safe reading per the design doc §0.4.
			return
		}
		if claim.ClaimForm != evidenceForm {
			out = append(out, types.Violation{
				Kind: types.ViolClaimFormUnsupported,
				Detail: fmt.Sprintf(
					"%s declares claim_form=%s but the cited evidence projects to claim_form=%s",
					label, claim.ClaimForm, evidenceForm),
				Repair: fmt.Sprintf(
					"either drop %s.claim_use.claim_form (the LLM was wrong about the evidence shape), or change the citation to one whose anchor kind matches %s. The system will not auto-correct because this is a self-contradiction the LLM emitted; finalizer rewrite required.",
					label, claim.ClaimForm),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "claim_use.claim_form",
					Reason:     fmt.Sprintf("claim_form mismatch on %s", label),
					Confidence: 0.7,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}
	for i := range doc.Steps {
		visit(fmt.Sprintf("steps[%d]", i), doc.Steps[i].CitationRef, doc.Steps[i].ClaimUse)
	}
	for i := range doc.Symbols {
		// AnswerSymbol does not carry a CitationRef; the symbol IS
		// the citation (File + Line are direct anchors). Use ref=-1
		// so claimFormForCitationRef falls through to the symbol-
		// based projection branch.
		visit(fmt.Sprintf("symbols[%d]", i), -1, doc.Symbols[i].ClaimUse)
	}
	if doc.Value != nil {
		visit("value", doc.Value.CitationRef, doc.Value.ClaimUse)
	}
	if doc.Boolean != nil {
		visit("boolean", doc.Boolean.CitationRef, doc.Boolean.ClaimUse)
	}
	return out
}

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
// Trigger uses ONLY typed enums (AnswerExactResolutionAbsent +
// EvidenceScope=ScopeNegative); NO prose keyword matching. Following
// the "precise signals for hard gates" red line.
func runAbsenceScopeBoundOracle(doc *types.AnswerDocument, rm *types.RequestModel) []types.Violation {
	if doc == nil || doc.ExactResolution == nil {
		return nil
	}
	if doc.ExactResolution.Status != types.AnswerExactResolutionAbsent {
		return nil
	}
	// Walk citation pool; an absent claim is bounded when any citation
	// carries Scope=Negative + non-empty NegativePattern. The pattern
	// itself is the system's record of "what we searched for and
	// confirmed absent" — operators can audit the scope.
	bounded := false
	for _, c := range doc.Citations {
		if c.Scope == types.ScopeNegative && strings.TrimSpace(c.NegativePattern) != "" {
			bounded = true
			break
		}
	}
	if bounded {
		return nil
	}
	return []types.Violation{{
		Kind: types.ViolAbsenceScopeExceeded,
		Detail: "exact_resolution.status=absent declared but no citation carries scope=negative + a non-empty negative_pattern; the absence claim is unbounded",
		Repair: "Re-emit emit_answer_document with citations[] including at least one entry with scope='negative' AND a non-empty negative_pattern naming the exact search query (grep / repomap / file glob) that confirmed the absence. If the bounded evidence is already in the pool, attach it directly as a negative-scope citation; otherwise the next investigation pass must run the bounded search and surface its query.",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "exact_resolution.absence_scope",
			Reason:     "absence claim without bounded negative-scope citation",
			Confidence: 0.65,
		},
		Stage: string(types.StageFinalize),
	}}
}

// collectClaimUseFacets walks every payload position that may carry
// a RenderedClaimUse pointer and invokes fn for each non-nil one.
// Used by runFacetCoverageOracle to harvest explicit facet
// annotations across the document tree.
func collectClaimUseFacets(doc *types.AnswerDocument, fn func(*types.RenderedClaimUse)) {
	if doc == nil {
		return
	}
	for i := range doc.Steps {
		fn(doc.Steps[i].ClaimUse)
	}
	for i := range doc.Symbols {
		fn(doc.Symbols[i].ClaimUse)
	}
	if doc.Value != nil {
		fn(doc.Value.ClaimUse)
	}
	if doc.Boolean != nil {
		fn(doc.Boolean.ClaimUse)
	}
}

// inferClaimFormsFromCitations returns, per citation index, the set
// of ClaimForm values projected from the underlying evidence (when
// matchable). Used to compute "inferred coverage" for facets the
// LLM did not explicitly annotate via claim_use.
//
// The returned map is keyed by citation_ref (zero-based index into
// doc.Citations). A citation with no matching evidence in the
// MutableState's TurnA artefacts contributes ClaimUnknown; the
// caller must treat ClaimUnknown as non-coverage.
func inferClaimFormsFromCitations(doc *types.AnswerDocument, mut *types.MutableState) map[int]types.ClaimForm {
	if doc == nil || len(doc.Citations) == 0 || mut == nil {
		return nil
	}
	out := make(map[int]types.ClaimForm, len(doc.Citations))
	for i := range doc.Citations {
		out[i] = claimFormForCitation(doc.Citations[i], mut)
	}
	return out
}

// facetCoveredByInferredForms reports whether any citation's inferred
// ClaimForm is in the facet's AcceptableForms. AcceptableForms=nil
// means any non-Unknown form satisfies; matches Phase 1's
// claimFormMatches semantic exactly.
func facetCoveredByInferredForms(req types.FacetRequirement, inferred map[int]types.ClaimForm) bool {
	for _, form := range inferred {
		if form == "" || form == types.ClaimUnknown {
			continue
		}
		if len(req.AcceptableForms) == 0 {
			return true
		}
		for _, allow := range req.AcceptableForms {
			if form == allow {
				return true
			}
		}
	}
	return false
}

// claimFormForCitationRef returns the ClaimForm projection for the
// citation at index ref (-1 means no citation, returns empty). Used
// by runClaimFormSupportOracle to look up the actual evidence shape
// behind a payload's claim_use.
func claimFormForCitationRef(doc *types.AnswerDocument, mut *types.MutableState, ref int) types.ClaimForm {
	if doc == nil || ref < 0 || ref >= len(doc.Citations) {
		return ""
	}
	return claimFormForCitation(doc.Citations[ref], mut)
}

// claimFormForCitation finds the EvidenceItem in MutableState's
// emitted-evidence pool matching the citation by Source path +
// LineStart, then runs ClaimFormOf on it. Returns empty on no match.
//
// We read EmittedEvidence() rather than TurnAArtifacts.EvidenceItems
// because the latter is a compile-time snapshot taken at end-of-Turn-A;
// post-finalize oracles run when the live working pool already
// contains every evidence item (whether or not the snapshot was
// captured). EmittedEvidence is the union of all explorer + extractor
// emit_evidence calls in the Run.
func claimFormForCitation(citation types.Citation, mut *types.MutableState) types.ClaimForm {
	if mut == nil {
		return ""
	}
	for _, ev := range mut.EmittedEvidence() {
		if ev.Source != citation.File {
			continue
		}
		if ev.LineStart == 0 || citation.Line == 0 {
			continue
		}
		if citation.Line == ev.LineStart {
			return types.ClaimFormOf(ev)
		}
		if ev.LineEnd > 0 && citation.Line >= ev.LineStart && citation.Line <= ev.LineEnd {
			return types.ClaimFormOf(ev)
		}
	}
	return ""
}

// claimFormsAsStrings is a tiny renderer for violation Detail text.
func claimFormsAsStrings(forms []types.ClaimForm) []string {
	out := make([]string, 0, len(forms))
	for _, f := range forms {
		out = append(out, string(f))
	}
	return out
}

// walkAnchorKinds invokes fn for every AnchorKind observed across
// the Run's evidence surfaces (TurnA artefacts + emitted answer
// symbols). Stops early when fn returns true. Safe on nil mutable.
func walkAnchorKinds(mut *types.MutableState, fn func(types.AnchorKind) bool) {
	if mut == nil {
		return
	}
	if turnA := mut.TurnAArtifacts(); turnA != nil {
		for _, ev := range turnA.EvidenceItems {
			if ev.AnchorKind == "" {
				continue
			}
			if fn(ev.AnchorKind) {
				return
			}
		}
	}
}

// countDistinctAnswerSymbolBuckets reports how many distinct
// "buckets" (sub-topic-like grouping) the answer-symbols span.
// Bucket key is `File` when present (different files = different
// sub-topics in a multi-topic answer), else `Name` (different
// symbols = different topics within one file). Conservative: when
// both are blank (anomalous), each row counts as its own bucket
// so the divergence check leans toward over-counting.
func countDistinctAnswerSymbolBuckets(symbols []types.AnswerSymbol) int {
	seen := map[string]struct{}{}
	for i, s := range symbols {
		key := strings.TrimSpace(s.File)
		if key == "" {
			key = strings.TrimSpace(s.Name)
		}
		if key == "" {
			key = fmt.Sprintf("__row_%d", i)
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

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
// Floors: summary >= 100 chars, total body bullets >= 3, at
// least one body section non-empty. Tunable later via yaml when
// real eval shows the floor needs adjustment.
func shouldReviewConsistency(doc *types.AnswerDocument) bool {
	if doc == nil {
		return false
	}
	if len(strings.TrimSpace(doc.Summary)) < 100 {
		return false
	}
	switch doc.Shape {
	case types.ShapeStepList, types.ShapeListOfSymbols, types.ShapeExplanation:
		// continue
	default:
		return false
	}
	bodyItems := len(doc.Steps) + len(doc.Symbols)
	return bodyItems >= 3
}

// renderConsistencyReviewBody assembles a markdown body string
// from doc.Steps + doc.Symbols so the reviewer LLM sees the
// finalizer's bullet structure verbatim. Citations are stripped
// to file:line form (no Quote text) since we don't want the
// reviewer hallucinating about repo content — its job is
// purely prose↔prose.
//
// Plan D rollout (2026-05-02): kind=flow / kind=caveat steps render
// with an inline marker so the reviewer doesn't false-flag a count
// mismatch when the summary references a principal-count (e.g.
// summary says "9 X" and body has 10 numbered lines = 9 principal +
// 1 flow narration). Principal steps render plain (no marker) so
// pre-Kind reviewers stay byte-identical.
func renderConsistencyReviewBody(doc *types.AnswerDocument) string {
	var b strings.Builder
	for i, step := range doc.Steps {
		desc := strings.TrimSpace(step.Description)
		switch step.Kind {
		case types.AnswerStepKindFlow:
			fmt.Fprintf(&b, "%d. [narration / not principal] %s\n", i+1, desc)
		case types.AnswerStepKindCaveat:
			fmt.Fprintf(&b, "%d. [scope caveat / not principal] %s\n", i+1, desc)
		default:
			// principal or empty (back-compat): plain numbering.
			fmt.Fprintf(&b, "%d. %s\n", i+1, desc)
		}
	}
	for _, s := range doc.Symbols {
		anchor := strings.TrimSpace(s.Name)
		if anchor == "" {
			anchor = "(unnamed)"
		}
		loc := strings.TrimSpace(s.File)
		if loc != "" && s.Line > 0 {
			loc = fmt.Sprintf(" @ %s:%d", loc, s.Line)
		}
		rationale := strings.TrimSpace(s.Rationale)
		if rationale == "" {
			fmt.Fprintf(&b, "- %s%s\n", anchor, loc)
		} else {
			fmt.Fprintf(&b, "- %s%s — %s\n", anchor, loc, rationale)
		}
	}
	return b.String()
}

// runSelfConsistencyReview dispatches the reviewer LLM and
// converts its verdict into types.Violation entries. Method on
// Orchestrator so it has access to reviewer / yaml flags / emit
// surface for the REPL bottom-status line.
//
// Returns 0..N violations (one per Contradiction emitted at
// confidence >= floor). Failure modes (LLM error, malformed
// emit) are non-fatal: AppendLearningFailure records the issue
// so the Run-end summary surfaces it; no violations returned;
// answer ships unchanged.
func (o *Orchestrator) runSelfConsistencyReview(doc *types.AnswerDocument, mut *types.MutableState) []types.Violation {
	if o == nil || o.selfConsistencyReviewer == nil || doc == nil || mut == nil {
		return nil
	}
	floor := o.selfConsistencyMinConfidence
	if floor <= 0 {
		floor = 0.8
	}

	// User-visible status line (commit 62 dock-fidelity): never
	// silent background work, especially at the bottom dock area.
	o.emit(render.Event{
		Kind:      render.EventAgentReasoning,
		Timestamp: time.Now(),
		Agent:     "orchestrator",
		Reasoning: selfConsistencyReviewStartMessage(o.busCtx.Language),
	})

	// Strip system-injected hedge markers + Authority caveat before
	// the reviewer sees the prose. Without this the reviewer LLM
	// treats "[hedged] X is at line 10" vs "X is at line 10" as a
	// factual contradiction, mistaking system annotations for content.
	// The drift-bounded answer would then trigger spurious self-
	// contradiction retries on every log-attached run.
	in := SelfConsistencyInput{
		OriginalRequest: mut.Objective(),
		AnswerSummary:   render.StripAuthorityArtifacts(doc.Summary),
		AnswerBody:      render.StripAuthorityArtifacts(renderConsistencyReviewBody(doc)),
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
		// Either consistent OR low-confidence verdict — silently
		// drop. Status line above already informed the user we
		// reviewed.
		if verdict != nil {
			logging.Info("[self_consistency_reviewer] verdict consistent=%v confidence=%.2f (floor=%.2f) reasoning=%q",
				verdict.Consistent, verdict.Confidence, floor, verdict.Reasoning)
		}
		return nil
	}

	// Surface contradiction count + rewrite-mode to the user via
	// REPL bottom status line. Honesty: explicit count + whether
	// the system will rewrite.
	o.emit(render.Event{
		Kind:      render.EventAgentReasoning,
		Timestamp: time.Now(),
		Agent:     "orchestrator",
		Reasoning: selfConsistencyContradictionMessage(o.busCtx.Language, o.selfConsistencyRewriteOnContradiction, len(verdict.Contradictions)),
	})

	out := make([]types.Violation, 0, len(verdict.Contradictions))
	totalN := len(verdict.Contradictions)
	reasoning := clampReasoningForRepair(verdict.Reasoning)
	for i, c := range verdict.Contradictions {
		out = append(out, types.Violation{
			Kind: types.ViolSelfContradiction,
			Detail: fmt.Sprintf("self_contradiction[%s] — SUMMARY: %q ⇄ BODY: %q",
				c.Topic, c.SummaryClaim, c.BodyClaim),
			Repair: buildSelfContradictionRepair(c, reasoning, i+1, totalN),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_summary_body_consistency",
				Reason:     "reviewer detected inter-paragraph contradiction",
				Confidence: verdict.Confidence,
			},
			Stage: string(types.StageFinalize),
		})
	}
	logging.Info("[self_consistency_reviewer] emitted %d contradiction(s) at confidence=%.2f rewrite_on=%v reasoning=%q",
		len(verdict.Contradictions), verdict.Confidence, o.selfConsistencyRewriteOnContradiction, verdict.Reasoning)
	return out
}

// clampReasoningForRepair returns a single-line, ≤ 200-rune
// rendering of the reviewer's reasoning, suitable for embedding
// in a Violation.Repair string. Newlines flatten to spaces so
// the joined retry-hint format stays compact.
func clampReasoningForRepair(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
	s = strings.TrimSpace(s)
	const cap = 200
	r := []rune(s)
	if len(r) > cap {
		return string(r[:cap]) + "…"
	}
	return s
}

// buildSelfContradictionRepair generates the comprehensive
// rewrite guidance per contradiction (commit 62 follow-up: pre-
// fix the Repair was a generic "pick version supported by
// evidence and rewrite the other to match" — but reviewer
// reasoning was discarded; preference between SUMMARY and BODY
// was unclear; the LLM had no fallback for evidence-undecided
// cases; multi-contradiction batches lacked a "reconcile all
// before emitting" framing). Now covers 5 model-fix-able error
// classes: pure prose mistake, post-revision summary stale,
// genuine ambiguity needing hedge, partial-fix introducing new
// error, missed contradictions in batch.
func buildSelfContradictionRepair(c SelfConsistencyContradiction, reasoning string, idx, total int) string {
	var b strings.Builder
	if total > 1 {
		fmt.Fprintf(&b, "[%d of %d contradictions] ", idx, total)
	}
	fmt.Fprintf(&b, "TOPIC: %s. ", c.Topic)
	fmt.Fprintf(&b, "SUMMARY says: %q. BODY says: %q. ",
		c.SummaryClaim, c.BodyClaim)
	if reasoning != "" {
		fmt.Fprintf(&b, "Reviewer reasoning: %s. ", reasoning)
	}
	b.WriteString("ACTION: rewrite the SUMMARY to align with the BODY — the body's bullets are the load-bearing claims, each anchored to file:line citations the grounder has already validated, so the body is the authoritative version when summary and body disagree. ")
	b.WriteString("FALLBACK: if the evidence does not actually support EITHER claim with certainty (i.e. the reviewer caught a contradiction that neither side has grounded support for), replace the summary's specific assertion with an honest hedge naming exactly what WAS verified — do not pick an unsupported version. ")
	b.WriteString("Do NOT introduce new factual claims; do NOT keep the contradicting summary phrase verbatim and 'add a clarification' — fully rewrite it. ")
	if total > 1 {
		b.WriteString("Reconcile ALL listed contradictions in this round before emitting emit_answer_document; a partial fix that leaves any of them or introduces new ones will trigger another retry. ")
	}
	return strings.TrimSpace(b.String())
}

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

func shapeTextForContractCheck(doc *types.AnswerDocument) string {
	if doc == nil {
		return ""
	}
	switch doc.Shape {
	case types.ShapeValue:
		if doc.Value != nil {
			return doc.Value.Literal
		}
	case types.ShapeConfigValue:
		if doc.Value != nil {
			key := strings.TrimSpace(doc.Value.Key)
			if key != "" {
				return key + "=" + doc.Value.Literal
			}
			return doc.Value.Literal
		}
	case types.ShapeBoolean:
		if doc.Boolean != nil {
			if doc.Boolean.Decision {
				return "yes"
			}
			return "no"
		}
	}
	return ""
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
