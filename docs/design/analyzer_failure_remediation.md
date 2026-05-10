# Analyzer Failure Modes — Systematic Generalised Remediation

**Status:** design (2026-05-10).
**Baseline:** `9d92741` (post forced-read remediation L1-L4 + budget audit).
**Owner:** session forensic on 2 sweep failures (`logtri_python`, `logtri_goroutine_dump`).

---

## 1. Why this exists

The 59-case PARALLEL=4 sweep surfaced 2 FAILs that share a meta-pattern even though their surface symptoms differ. Both are pre-existing structural defects, NOT regressions from L1-L4. Both deserve a generalised systematic fix because the meta-pattern affects more than these 2 cases.

| Case | Symptom | Direct cause | Failure class |
|------|---------|--------------|---------------|
| `logtri_python` | "(no result)" — pipeline produces nothing | Analyzer retry-storm fingerprint early-exit + degraded IR cascade has no explore/extract nodes | **Hard fail / silent stop** |
| `logtri_goroutine_dump` | Wrong answer ("nil map write" instead of "concurrent map writes") | LOG header explicit error message gets rendered AFTER frame list; LLM follows synthetic file:line refs and concocts theory | **Drift / hyperfocus** |

### 1.1 Code-level cascades

Both forensic transcripts in this session's history. Briefly:

**logtri_python cascade** (`internal/orchestrator/orchestrator.go:1810-1833 + 6092-6122`):
1. analyzer LLM emits `answer_subject.kind=string_literal` (wrong for root_cause family)
2. `internal/analysis/gate/coherence.go:494-531 checkShapeSubjectCoherence` R2.2 fires (kind in scalar list + confidence ≥ 0.6 + family expects non-scalar)
3. orchestrator retry-attempts 1 → analyzer LLM emits SAME shape → R2.2 same Detail string
4. `internal/analysis/gate/fingerprint.go computeGateFingerprint` returns identical hex → retry storm match (≥ ceil(max_retries/2), default 2)
5. `buildDegradedFallbackIR` builds minimal IR with **only `NodeFinalize`**, **discards** `AnalyzerHints`, `EvidencePlan`, `HypothesisSet`, `TermGraph`
6. `runReadSchedulerLoop` calls `readyExplorerWindow` → returns empty (no non-finalize nodes) → explorer+extractor SKIPPED
7. finalizer runs once with empty EvidencePlan, produces "(no result)" + error caveat

**logtri_goroutine_dump cascade** (`internal/context/builder.go:2204-2276 formatLogTriageStructured`):
1. LOG header: `fatal error: concurrent map writes` (Go runtime race signature)
2. `internal/analysis/logtriage/bug_class_registry.go:41-44` detects `BugClassRace` correctly
3. `formatLogTriageStructured` renders sections in this order:
   - **first**: `### Detected Patterns` (line 2211, calls `renderBugClassesSection`)
   - **second**: `### Error` tree with `1. **fatal error** — concurrent map writes` followed by frames (lines 2265-2276)
4. Frame validator (`internal/analysis/logtriage/validate.go:148-196`) marks frames as `(unresolved)` because synthetic `writeSession`/`analyzer.go:100` doesn't exist in real codebase
5. LLM sees:
   - canonical term "数据竞争 (并发 map 写)" in Detected Patterns (good)
   - error message `concurrent map writes` inline with type heading (gets skimmed)
   - 3 frame entries pointing at same `analyzer.go:100` (looks concrete)
6. LLM hyperfocuses on resolving `writeSession` in real code
7. Not finding it, fabricates "multigraph.New() returns nil" theory
8. Answer never mentions concurrency / race / 并发

### 1.2 Generalisation — these are not isolated bugs

**Pattern α — family ↔ subject-kind contradiction → hard fail:**
- Affects ANY root_cause/call_chain/debug question where LLM picks scalar kind
- Affects ANY architecture/explain where LLM picks function_name
- Affects ANY enumeration where LLM picks numeric for "how many"
- R2.2 gate is structurally correct but offers NO graceful recovery

**Pattern β — synthetic / unverified frames win over LOG message:**
- Affects ANY synthetic fixture (eval test cases) with constructed file:line
- Affects ANY production log where frames point to vendored/generated code
- Affects ANY case where the explicit runtime error message is the most reliable signal but the rendering buries it

The two cases this session are concrete instances of these generalised patterns. Fixing only the symptoms would leave the next analogous case to re-fail.

---

## 2. Existing infrastructure inventory

(extracted from 3 parallel code audits run 2026-05-10.)

### 2.1 R2.2 gate machinery

| Surface | File:Line | Behaviour |
|---------|-----------|-----------|
| `checkShapeSubjectCoherence` | `coherence.go:479-540` | Fires R2.2 when LLM-emitted scalar kind + confidence ≥ 0.6 + family expects non-scalar |
| `isScalarSubjectKind` | `coherence.go:723-729` | Returns true for SubjectNumeric, SubjectStringLiteral, SubjectReturnValue |
| `coherenceSubjectConfidenceFloor` | `coherence.go:55` | 0.6 |
| `IsCountQuestion` carve-out | `coherence.go:520` | Exempts is_count_question=true |
| `ResolveQuestionFamily` | `internal/types/facet_plan.go:50+` | Maps intent+scenario+predicates → family |
| `computeGateFingerprint` | `internal/analysis/gate/fingerprint.go:51-100` | SHA256(rule keys), 12-char hex |
| Retry-storm threshold | `orchestrator.go:2158` | `ceil(max_retries/2)`, min 2 |
| `buildDegradedFallbackIR` | `orchestrator.go:6092-6122` | Discards AnalyzerHints/EvidencePlan/HypothesisSet/TermGraph |

### 2.2 Log-triage prompt rendering

| Surface | File:Line | Role |
|---------|-----------|------|
| `formatLogTriageStructured` | `builder.go:2204-2276` | Top-level renderer; controls section order |
| `renderBugClassesSection` | `builder.go:1935-1990` | "### Detected Patterns" section, fires first |
| `renderLogError` | `builder.go:2483+` | Error tree; message inline with type heading |
| `renderLogFrame` | `builder.go:2513+` | Per-frame rendering |
| `bugClassRegistry` | `internal/analysis/logtriage/bug_class_registry.go:38-276` | 19 BugClass × ~60 patterns |
| `DetectBugClasses` | `bug_class_registry.go:295-324` | First-match-per-class |
| `validateFrame` | `internal/analysis/logtriage/resolver.go` | Marks unresolvable frames as File="" |

### 2.3 Reuse opportunities

- `BugClassRace` detector already covers `concurrent map writes` — no new detector needed.
- `DetectedBugClass.MatchedSignature` already captures the actual matched substring — perfect to elevate as primary signal.
- `R2.2` already has a soft-fail path for `IsCountQuestion=true` — same pattern can extend for "narrative root_cause kind unset" recovery.
- `runReadSchedulerLoop` already supports running with a partial IR — the issue is `buildDegradedFallbackIR` strips too much, not the scheduler.

---

## 3. The 4-layer plan

### 3.1 Fix-A — degraded IR cascade with semantic preservation

**Problem:** `buildDegradedFallbackIR` discards everything from analyzer's partial emit. Even when the LLM successfully emit_analysis'd with valid keywords/entities/intent, the degraded path resets to zero.

**Fix:** create `buildDegradedSemanticIR(objective, partialIR, err)` that:
1. **Preserves** `AnalyzerHints.{Keywords, Entities, PrimaryEntities, ExactTargets, ExactContextTerms}` from partial IR
2. **Preserves** `RequestModel.{Intent, Scenario, Complexity, Language}` (best-effort)
3. **Auto-corrects** `AnswerSubject.Kind = SubjectUnknown` (lets downstream auto-infer)
4. **Auto-corrects** `AnswerSubject.Confidence = 0` (signals "uncertain")
5. **Builds richer TaskGraph** with `NodeProbe → NodeEvidence → NodeReconcile → NodeFinalize` so explorer/extractor still run
6. **Sets** `CitationReq.Required = false` (relaxed)
7. **Sets** new `QualityGate.Check{Name: "degraded_semantic_recovery", Passed: true, Detail: "<reason>"}` for observability

**Edge cases handled:**
| Edge | Handling |
|------|----------|
| analyzer never emit_analysis'd at all | Falls back to legacy `buildDegradedFallbackIR` (zero-info path) |
| partial IR has incomplete keywords (< 3) | Still preserves; explorer can fall back to keyword grep |
| partial IR has empty entities | Preserved as empty; explorer's effectivePrimaryFiles still runs the L3+B union (post our prior fixes) |
| partial IR shape contract is malformed | Strip the contract; keep only RequestModel + AnalyzerHints |
| analyzer dispatched 3× and ALL fingerprint-matched | Use whichever partial IR was emitted last (most refined) |

**Step budget:** explorer dispatch in degraded mode capped at 1 iter (don't waste budget on a fallback path).

### 3.2 Fix-B — analyzer skill prompt R2.2 prevention

**Problem:** analysis-skill workflow doesn't tell the LLM about R2.2. The prompt explains "answer_subject.kind is what kind of literal" but doesn't say "DON'T set scalar kinds for narrative-shape questions."

**Fix:** add a workflow bullet + extend retry hint composer.

**Skill prompt addition** (`internal/skill/analysis_contract.go` analysis-skill Workflow):

> "For root_cause / call_chain / debug / explain questions whose answer is a multi-step narrative (a chain of causes, a sequence of files, an architecture description), do NOT set `answer_subject.kind` to a scalar value (`string_literal`, `numeric`, `return_value`). Either leave the field unset (let the system infer), or pick a non-scalar kind that matches a single named target (e.g. `function_name` only when the user is asking 'which function', `config_key` when asking 'which config option'). When the question pairs a single error name with 'explain what happened', the answer is the explanation, NOT the error name — so unset the kind. The downstream contract gate rejects scalar kinds for narrative families and the analyzer will be forced to retry."

**Retry hint composer extension** (`internal/agent/analyzer.go composeCoherenceRetryHint`):

When the rejected report contains R2.2:
```
- The R2.2 gate rejected your prior emit because family=root_cause_trace 
  (or call_chain / architecture / etc.) expects a multi-step narrative answer, 
  but you set answer_subject.kind=string_literal at confidence 0.95. To fix: 
  either UNSET answer_subject (most cases — let the system infer the kind from 
  question_kind), or change kind to a non-scalar value that fits the narrative 
  shape. Common mistake: the user mentioned an error name 'X' in their question, 
  but the answer is the EXPLANATION of why X happened — the explanation is 
  multi-step prose, not a literal.
```

### 3.3 Fix-C — log-triage LOG message priority elevation

**Problem:** `formatLogTriageStructured` renders Detected Patterns first, then Error tree where the Message is inline with the Type heading. For race-condition logs, the LLM weights frame-level evidence > pattern term, especially when frames are unresolved.

**Fix:** add a NEW high-priority "Primary error signal" section BEFORE Detected Patterns.

**Renderer changes** (`internal/context/builder.go formatLogTriageStructured`):

```
## Primary Error Signal

The runtime emitted this verbatim error before the stack frames were
captured. It is the highest-confidence diagnostic signal — TREAT IT 
AS AUTHORITATIVE over the frame list below, especially when frames 
are marked (unresolved).

- **fatal error: concurrent map writes** [BugClass: race / 数据竞争]
  Detected: 3 goroutines hit the SAME line (writeSession at 
  internal/agent/analyzer.go:100) — this confirms a concurrent-write
  data race, not a single-thread fault.
```

**Rendering rules:**
- Show the verbatim Type+Message of the FIRST top-level error (no inline truncation)
- When ≥ 2 goroutines hit the same (file, line) coordinate, surface as a "parallel goroutines" sub-bullet
- Cross-link to BugClass canonical term (Detected Patterns becomes a glossary, not the primary signal)
- Order: Primary Error Signal → Detected Patterns → Error tree → Frames

**Edge cases handled:**
| Edge | Handling |
|------|----------|
| No top-level error message | Skip section, render Detected Patterns first as today |
| Multiple top-level errors | Show each with its own primary-signal sub-bullet |
| Pattern detection failed but message exists | Surface the message anyway — operator can compute the BugClass downstream |
| Single goroutine | No parallel-goroutine sub-bullet (correct — not a race indicator alone) |
| Many goroutines but different lines | No parallel-goroutine sub-bullet (different fault sites) |

### 3.4 Fix-D — R2.2 second-attempt auto-correction

**Problem:** when LLM repeats the same `answer_subject.kind` across attempts, R2.2 fires identically and fingerprint-matches. The retry storm early-exit kills the run.

**Fix:** at retry-attempt N (where N ≥ 2 and prior R2.2 fired), the orchestrator AUTO-CORRECTS the IR:
- Set `AnswerSubject.Kind = SubjectUnknown`
- Set `AnswerSubject.Confidence = 0`
- Re-run `checkShapeSubjectCoherence` — should now pass (Unknown isn't scalar)
- Continue with corrected IR; do NOT count as another retry attempt

**Code site** (`internal/orchestrator/orchestrator.go runAnalyzePhase` retry loop):
```go
// After gate report, before fingerprint check
if attempt >= 1 && reportContainsR22(report) && o.busCtx.AnalysisIR != nil {
    if autoCorrected := autoCorrectScalarSubject(o.busCtx.AnalysisIR); autoCorrected {
        // Re-run gate on corrected IR
        report = gate.RunWith(o.busCtx.AnalysisIR, ...)
        if !report.Rejected {
            logging.Warning("[orchestrator] R2.2 auto-corrected: cleared scalar AnswerSubject.Kind on attempt %d", attempt)
            // Skip the rest of the retry — IR is good now
            return ...
        }
    }
}
```

**Edge cases handled:**
| Edge | Handling |
|------|----------|
| AnswerSubject is already nil | Skip auto-correct (nothing to clear) |
| The carve-out (IsCountQuestion) was the issue | Auto-correct doesn't apply (R2.2 wouldn't have fired) |
| Auto-correct fixes R2.2 but exposes a new gate failure | Continue retry loop normally |
| LLM emitted scalar with high confidence on attempt 0 | Don't auto-correct on first attempt — give LLM a chance to revise |
| R2.2 fires on first attempt but LLM revises to non-scalar on second | Normal flow; auto-correct never invoked |

**Why this is safe:** R2.2's whole point is "the family contract demands non-scalar; remove the scalar declaration." Auto-correct does exactly that. The downstream auto-inference fills `Kind` based on `question_kind`, which is what `is_unset_answer_subject` is designed for.

---

## 4. Multi-language coverage matrix

| Layer | Multi-language footprint | How |
|-------|-------------------------|-----|
| Fix-A | None — purely IR-shape | Cross-language derived semantic info preserved |
| Fix-B | LLM-side, language-agnostic | Skill prompt phrased in structural terms ("narrative", "scalar"), no language-specific examples |
| Fix-C | LOG message preservation works for any runtime | BugClass detector covers Go / Python / Java / Rust / C++ / Ruby / Swift / JS already; new "Primary Error Signal" rendering inherits coverage automatically |
| Fix-D | None — purely IR-shape | Auto-correct is language-agnostic |

---

## 5. Implementation plan + commit map

| Order | Layer | Commits | LOC est. | Eval gate |
|-------|-------|---------|----------|-----------|
| 1 | Fix-D (smallest + most impactful for python case) | 1 | ~80 | logtri_python single PASS |
| 2 | Fix-A (cascade preservation) | 1 | ~150 | logtri_python answer non-empty even if degraded |
| 3 | Fix-C (renderer) | 1 | ~80 | logtri_goroutine_dump answer mentions "concurrent" or "并发" |
| 4 | Fix-B (skill prompt + retry hint) | 1 | ~40 | regression: no other case starts setting wrong kind |
| 5 | Final validation sweep | — | — | full 59-case eval, ≥ 95% PASS rate |

**Total**: ~350 LOC, 4 commits, 1 session.

---

## 6. Test plan

### 6.1 Unit tests (~25 new cases)

| Layer | Test file | Cases |
|-------|-----------|-------|
| Fix-A | `degraded_semantic_ir_test.go` (new) | 8 cases — partial IR preservation, IR shape variants, edge cases |
| Fix-B | `analysis_contract_test.go` (extend) | 3 cases — skill prompt strings present, retry hint format |
| Fix-C | `log_triage_renderer_test.go` (new) | 8 cases — Primary Error Signal section presence/absence, parallel-goroutines detection, edge cases |
| Fix-D | `r22_auto_correct_test.go` (new) | 6 cases — auto-correct on R2.2, no-op on other failures, attempt gating |

### 6.2 Integration eval

After each fix ships:
- logtri_python single PASS (Fix-A + Fix-D)
- logtri_goroutine_dump single PASS (Fix-C)
- logtri_python + logtri_goroutine_dump in-sweep alongside 30+ historical cases (no regression)

### 6.3 Acceptance criteria

| Criterion | Target |
|-----------|--------|
| logtri_python PASS | Single-run repeatable PASS (was: hard fail produces no answer) |
| logtri_goroutine_dump PASS | Single-run repeatable PASS (was: wrong-answer drift) |
| 59-case sweep PASS rate | ≥ 95% (was: ~93% pre-fix) |
| Median analyzer dispatches | No regression (auto-correct doesn't add round-trips) |
| R2.2 fingerprint early-exits per 100 cases | -100% (auto-correct prevents storm) |

---

## 7. Red-line audit (per `feedback_prompt_redline_checklist.md`)

| Rule | Fix-A | Fix-B | Fix-C | Fix-D |
|------|-------|-------|-------|-------|
| **R3** Precise/noisy | ✅ partial-IR preservation is precise typed | ✅ skill prompt is soft guidance | ✅ Primary Error Signal carries verbatim string | ✅ auto-correct gates on precise R2.2 ViolKind |
| **R4** No over-fit | ✅ generic "preserve partial" pattern | ✅ generic "narrative families" phrasing | ✅ generic "first error message" pattern | ✅ generic "scalar kind in non-scalar family" gate |
| **R6** No internal terms | ✅ no LLM-facing change | ✅ no internal jargon ("scalar"/"narrative" are code-neutral) | ✅ no internal type names; "BugClass" replaced with "Detected Pattern" in user-facing text | ✅ no LLM-facing change |
| **CN+EN-only prompts** | n/a | ✅ Chinese examples ok per user red line | ✅ Chinese label "数据竞争" already in registry | n/a |
| **R2'** 6-spot sync | n/a (no new typed signal) | n/a | n/a | n/a |

---

## 8. Risk + rollback

| Layer | Worst-case regression | Rollback |
|-------|----------------------|----------|
| Fix-A | Degraded IR runs explorer with bad partial keywords → wasted iter | Set explorer step budget to 1 in degraded mode; revert to legacy buildDegradedFallbackIR |
| Fix-B | LLM never sets answer_subject.kind even when correct | The downstream auto-infer fills kind from question_kind; no functional regression |
| Fix-C | Primary Error Signal section is misread for non-error cases | Only fires when bundle.Errors is non-empty — guarded |
| Fix-D | Auto-correct masks a legitimate scalar answer | Gated to attempt ≥ 1 (LLM gets first chance); R2.2 ViolKind precise match |

Each layer is **independently revertable**. None depend on each other.

---

## 9. Out of scope (deferred)

- **Generalised gate-recovery framework** for ALL gate ViolKinds (not just R2.2) — would let other gate failures (R1.x coherence, R3.x DAG closure) auto-correct similarly. Worth a follow-up session.
- **LLM-side post-mortem** when retry-storm fires — emit a synthetic emit_evidence "I couldn't classify; here's what I saw" so finalizer has SOMETHING to render. Touches multiple agents.
- **BugClass per-class confidence boost** when N parallel goroutines + race pattern co-occur — tighter precision but small effect; deferred.
- **synthetic-frame fabrication detector** that flags when LLM concocts non-existent symbol names → triggers re-investigation. Cross-cutting; deferred.

---

**Ship order:** Fix-D → Fix-A → Fix-C → Fix-B → final sweep. Each step verified standalone before proceeding.
