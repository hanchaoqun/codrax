# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T10:11:55Z
- sweep_start_ts: 20260805-031154
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260805-031156 | answer_regex,answer_contains | none | 227s | 31 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | Analyzer emitted the ordered endpoint profile and Explorer proved `buildAnalysisIR -> gate.RunWith`, `gate.Run -> RunWith`, and `no_directed_path`. Finalizer nevertheless received `requested_sink_existence_proof=unproven`, invented the reverse `RunWith -> Run` direction in prose, and needed two diagram repairs. |
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260805-031156 | answer_regex,answer_contains | none | 236s | 25 | read=2,repo_map=4,list=0,trace=0,source_lens=4 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Final answer visibly enumerates all 3 types, 5 functions, and 30 constants with row-local citations. The first draft's authored tables hid item-only rows; B100 correctly rejected it once and the model repaired the visible carrier. No stale `iota` claim. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Production close from B100

- `EVAL-B100-ENDPOINTADMISSION1`: closed. The Analyzer's first accepted emission carried the request-validated ordered profile `buildAnalysisIR -> gate.Run`; unordered entities did not become direction authority.
- `EVAL-B100-VISIBLECARRIER1`: closed. The inventory draft could no longer use hidden `items[]` as a substitute for rows absent from an authored Markdown table; one legitimate repair produced a complete visible roster.
- `EVAL-B99-PATCHCITATIONDRIFT1`: still replay-pending. This pair did not create the same stable-ID insertion and inherited-reference displacement shape, so it cannot be closed from this run.

## New gap: `EVAL-B101-BOUNDARYHANDOFF1` (P0)

Completion accepted `principal_span_waiver=no_directed_path` only after Explorer emitted a grounded exact `gate.Run` definition and the real incident edges. The finalizer boundary capsule was rebuilt from `Mutable.TurnAArtifacts().EvidenceItems` plus the mutable emitted buffer, while the accepted Explorer evidence had already moved to `AgentContext.EvidenceItems`. Compaction/reset therefore made two adjacent stages consume different typed authority:

- completion: both endpoints proven; no requested-direction path;
- finalizer: `requested_sink_existence_proof=unproven` and `evidence_status=endpoint_unresolved`.

The finalizer prompt still advised not to extend the nearest path, but the contradictory typed status deprived the model of the grounded wrapper direction. The answer consequently claimed that `gate.RunWith` calls `gate.Run`, whereas source establishes the reverse wrapper edge `gate.Run -> RunWith`.

## Fix and invariants

`BuildAnswerSemanticViewForAgentContext` and `BuildAnswerSemanticViewForBusContext` now pass their accepted handoff `EvidenceItems` into the shared endpoint-boundary analyzer, then union the current mutable evidence lanes. Identity/edge analyzers retain their existing deduplication and grounding rules. Tests pin both context constructors and the production finalizer prompt to `requested_sink_existence_proof=definition_only` with no fabricated incident edge.

This is an evidence-carrier continuity fix only: it does not scan request/model/final prose, create a call edge from a definition, alter the model answer, or enter RootCauseTrace, explicit-window causal projection, double-axis root-cause analysis, or automatic supplementation.

Verification: full `internal/types` (22.439s), `internal/agent` (4.072s), and `internal/tool` (169.392s) passed; the additional BusContext parity pin passed in the targeted types/agent suite. Status: `implemented/full-pass/replay-next`.
