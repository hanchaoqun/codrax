# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T01:37:15Z
- sweep_start_ts: 20260811-183713
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-183715 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 111s | 27 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 7ms window, state/rank views, typed wakeup chain, two-axis impact, background boundary, causal projection and automatic completion were retained. B589 reconciled the duplicated current-source exclusion before analysis acceptance. The model nevertheless states that VerifyClass completed before waking app although wake=5.005000s and span end=5.005400s, and elsewhere calls 5.000400..5.005400 about 4ms although it is 5ms. The same answer later carries the correct `on_chain_prewakeup_work_candidate_only` limitation, so this is answer-level temporal/mechanism inconsistency, not permission to promote off-chain context or for the system to rewrite the conclusion. |
| 2 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260811-183715 | answer_regex,answer_contains | none | 206s | 29 | read=8,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | The final answer exists and contains one connected StageAnalyze→StageExplore→StageExtract→StageFinalize spine plus the requested table; runner failure is exact `Mutable` missing, not an empty answer or elapsed-time degradation. The sequence uses response arrows `-->>` for forward precedence. Exploration left the request-mentioned `Mutable` unverified and never read `internal/types/context.go`; the final also assigns unrelated `internal/analysis/dataflow.Analyze`, `loadOrLowerFile`, and `digestFindings` helpers to the principal four-stage pipeline. This confirms a generalized relation/operator contract gap and a principal-workflow authority/context gap, not a reason to hard-scan final prose for the fixture word. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `B591-SEQUENCERELATIONOPERATOR1/P1-high`: validate the typed relation kind against Mermaid sequence operator semantics as one closed matrix. `-->>` is response/return; forward precedence/call/callback/binding/data-flow relations need a compatible forward operator. The signal is `diagram.kind + parsed operator + schema-validated relation_kind`; no request/model/final prose scan and no system-authored edge.
- `B592-PRINCIPALWORKFLOWCONTEXT1/P1-high`: extend the checkout-verified read-mode workflow provider with shared state-carrier facts and a principal-vs-supporting authority boundary. Homonymous, unconnected helper evidence may remain supporting context but cannot define a selected stage's internals. This is a provider/prompt improvement, not a final-answer keyword gate.
- Trace root-cause authority remains unchanged: only typed on-chain facts may occupy the principal root lane; off-chain observations remain background. The repeated span/wakeup wording drift is recorded for a later typed temporal-claim carrier rather than repaired by answer replacement.
- Both runs completed in 111s/206s. The read run is 3m26s, not over four minutes. Neither run exercised elapsed-time recovery; active model progress remains ineligible for time-based degradation.
