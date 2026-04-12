# Openai branch — priority plan (2026-04-12)

Synthesized from `project_stability_next_steps`, `project_openai_pipeline_analysis`, `project_dataflow_evidence_pipeline`, `project_dataflow_trigger_issues`, `project_explorer_precision_gaps`, and `project_explorer_deep_dive`.

Two parallel tracks:

- **Track A — openai branch**: Ground Truth pipeline. 5/5 stable on `answer_chain[0]`; 2/5 finalizer-correct (residual is model-level, not pipeline).
- **Track B — main / explorer precision**: avg 13.2/20, blocked by ContinuationPrompt timing.

The two tracks do not conflict. Tier 1 lives entirely on `openai`; explorer #34 lives on `main`.

Cross-references:
- `docs/openai-pipeline-analysis.md` — full capability audit (source of T1-T3)
- `docs/dataflow-evidence-pipeline-fixes.md` — 8-breakpoint history

---

## P0 — Do first (high leverage, low risk)

### T1.1 Two-phase dataflow decision
- **Where**: `internal/agent/explorer.go` (dataflow trigger site)
- **What**: When all ERM requirements are already satisfied by Concrete Values + Resolution/Hierarchy Chains, skip `dataflow.Analyze` entirely.
- **Rollout**: phase 1 — info-log "would-skip" without skipping; phase 2 — skip after one stable run shows the log matches expectations.
- **Cost**: ~50 LOC. **Risk**: zero (gated). **Impact**: saves O(files × func_size) on enumeration/identity questions.
- **Unlocks**: T1.2 (cleaner trigger data).

### T1.2 DataflowIntent enum
- **Where**: `internal/agent/evidence.go` (`needsDataflowAnalysis`), `internal/agent/explorer.go` (callers).
- **What**: Replace `bool needsDataflowAnalysis` with `DataflowIntent { None, Lookup, Propagate }`. `Lookup` runs lowering but skips `buildFindings` multi-hop phase. `Propagate` is the current full path.
- **Why**: Resolves `project_dataflow_trigger_issues` — keyword matching is high-recall low-precision. Three-state intent gives a structural fix instead of more keywords.
- **Cost**: ~100 LOC. **Risk**: low (additive enum, default `Propagate` preserves current behavior).
- **Unlocks**: T2.x mechanism pipeline (it needs an explicit Kind, not a bool).

### T1.3 Ground Truth predicate whitelist per ERM Kind
- **Where**: `internal/agent/erm.go:524`, `internal/agent/explorer.go` call site of `identifyAnswerChains`.
- **What**: `identifyAnswerChains` accepts a per-Kind predicate whitelist. Currently hard-coded to `resolution_chain / binds* / returns`. Open `inheritance`/`relationship` for "Is X a Y", `mechanism` for "how does X work", `conditional` for "what triggers X", `config_value` for config questions.
- **Cost**: ~60 LOC. **Risk**: low (whitelist expansion is monotone — Ground Truth gets more candidates, never fewer).
- **Impact**: covers question types 4/5/8/10 from the analysis doc.
- **Unlocks**: T2.1 directly.

---

## P1 — Parallel or immediately after (architectural unlocks)

### Multi-sample evaluation harness *(do before #34)*
- **Where**: `eval/` runner (or wherever the precision suite lives).
- **What**: Run each test case 3× (or 5×), report median + variance. Prevents false conclusions from single-sample noise — the bug that produced `T2: 13 → 10` between identical-code rounds.
- **Cost**: low. **Risk**: zero. **Why first**: every claim about #34's effect needs this baseline.

### #34 Mid-loop evaluator callback *(main branch / explorer)*
- **Where**: `internal/agent/agent.go` (`BaseAgent.Execute` ReAct loop), `Evaluator` interface.
- **What**: New `Evaluator.MidLoopCheck(iter int, last types.ToolResult, all []types.ToolResult) (hint string, inject bool)`. Called after every tool execution. If `inject=true`, hint is appended as a user message before the next LLM call. Fires every N iterations (e.g., 3) to avoid over-steering.
- **Why**: `ContinuationPrompt` only fires on soft stops. When the LLM keeps calling tools but in the wrong direction (wrong files, missing the question focus), every existing check is blind. This is the single biggest bottleneck in `project_explorer_precision_gaps`.
- **Unlocks**: #25 function-boundary push, #26 enumeration coverage push — both already implemented but unable to fire.
- **Cost**: medium. **Risk**: medium (modifies core ReAct loop — must be additive).
- **Expected**: T2/T3 stabilize at 15+ (currently 10-13 with high variance).

---

## P2 — Capability expansion (medium cost / medium risk)

### T2.1 ERM `mechanism` Kind
- **Where**: `internal/agent/erm.go`.
- **What**: New ERM Kind for "how does X work / 怎么" questions. Without it, mechanism questions fall fully back to LLM with no quality gate and no Ground Truth.
- **Depends on**: T1.3 (whitelist must accept the new Kind).
- **Cost**: ~80 LOC.

### T2.2 Mechanism scan pipeline
- **Where**: new `internal/agent/mechanism_scan.go`.
- **What**: For ERM-entity core methods, read FULL function body (not just signature), count branches, extract structured mechanism evidence (steps, conditions, side effects).
- **Why**: Aligns with the architectural principle from `project_dataflow_evidence_pipeline`: the finalizer should be a formatter, not a re-deriver.
- **Cost**: ~200 LOC.

### T2.3 Dataflow EntityBias
- **Where**: `internal/dataflow/engine.go` (`selectCandidateFiles`).
- **What**: Thread ERM entities into candidate selection so dataflow focuses on question-relevant files when it does run.
- **Cost**: ~60 LOC.

---

## P3 — Marginal polish (low priority)

- **T3.1** Dynamic `maxChains` in `identifyAnswerChains` per ERM Kind: enumeration 10-15, identity 3-5, mechanism 8-12. ~15 LOC. Depends on T1.3.
- **T3.2** Phase 2 prompt update: tell LLM "these fact classes are covered by Concrete Values, focus on relationships/mechanism." ~30 LOC. Reduces double-extraction.
- **T3.3** Concrete Values per-symbol entity affinity filter (currently per-file). ~80 LOC. Marginal speedup on chain construction.
- **#14** Programmatic control flow analysis (tree-sitter CFG) — high cost, only T1 multi-hop benefits. Park unless T1 stays at 12 after #34.

---

## Do-not list (explicit)

- ❌ Add patterns to `extractConcreteValues` — bottleneck is routing, not coverage.
- ❌ Add keywords to `needsDataflowAnalysis` — already over-triggers; the fix is structural (T1.1 + T1.2).
- ❌ Tune deterministic pipeline for the 3/5 wrong final answers — Ground Truth is 5/5 correct; the gap is model-level (`feedback_first_principles_root_cause` notwithstanding, this one is verified model layer).
- ❌ Query-specific hints — violates `feedback_no_overfitted_solutions`.
- ❌ Hardcoded stop-word lists — use repo symbol-table checks (`feedback_overfitting_audit_stopwords`).

---

## Execution order

1. **P1 multi-sample harness** — every later evaluation depends on it.
2. **P0 T1.1 → T1.2 → T1.3** — single openai-branch chain, each step unlocks the next.
3. **#34 mid-loop callback** in parallel on main (independent tree, no merge conflict).
4. After (2)+(3), re-measure on the openai 5-run suite and the explorer 20-pt suite. Decide whether to start P2.

## Acceptance metrics

| Metric | Current | Target |
|---|:-:|:-:|
| openai Ground Truth[0] correct | 5/5 | hold 5/5 |
| openai finalizer 100% correct | 2/5 | not pursued (model layer) |
| explorer avg score | 13.2/20 | 15+/20 (post #34) |
| explorer T2 single-run variance | 10-13 | ≤2 |

---

## Status log

| Date | Change |
|---|---|
| 2026-04-12 | Doc created. P0 work starting with T1.1. |
