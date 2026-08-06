# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T12:16:40Z
- sweep_start_ts: 20260806-051639
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260806-051640 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 123s | 34 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Root direction, target-state account, wakeup path, four ranked seats, dual axes and deterministic causal projection are useful. Model prose nevertheless adds #1 23.994ms + #2 19.041ms as 43.035ms, while the later typed projection proves these same-direction member envelopes overlap and says the sum is not valid. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260806-051640 | typed_inventory_rowset,dimension_substring,answer_contains | none | 135s | 20 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Correct 2 extend + 2 foreign func + 8 public class rows, packages, paths, lines and total 12; no duplicate global roster. The sole reject asks for source_inventory_row_id on two same-label Cart rows even though each structured item already cites an exact distinguishing source line. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and disposition

- `EVAL-B177-ROWIDCIT1` (P1 retry/JSON-burden): a duplicate label requires an invisible row ID even when the item already has a valid citation that selects exactly one matching typed row. Fix by deriving only the invisible ID from exact `item.label + citation_ref(file,line) + typed row registry`; ambiguity remains fail-closed. Never inspect request/title/prose.
- `EVAL-B177-TRACEADD1` (P1 correctness/context): the model sees a generic cross-row non-addition rule, but not the exact overlap relation later computed for the two leading lock/priority seats. Move that typed interval relation into pre-final decision inputs from the same authority as the deterministic projection. Do not scan or rewrite the model's visible arithmetic.
- `EVAL-B177-QUOTEDEGRADE1` (P2 presentation, pending audit): twelve lossless citation-quote hydrations are rendered under the broad label “system degradation disclosure”. The visible evidence is intact; audit the typed degradation taxonomy before changing this wording.
- `EVAL-B176-COUNT1` closes as carrier-induced/model-adherence observation for now: the same case produced the correct count after carrier contracts were unified, without a prose number gate.
