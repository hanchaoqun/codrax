# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T10:41:13Z
- sweep_start_ts: 20260808-034112
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260808-034113 | log_regex,answer_regex | none | 170s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | Final `17,0,5`, 15 entity resolutions, four contribution records, 13 rules, target-order zero fill, and reconcile=pass are correct; strict plain output is clean. Process is not healthy: 9 rounds, four action failures, one repair. Despite precise next_stage/allowed_next_actions, the planner repeatedly emits downstream DAG ranks in the current batch and is rejected. This is a generic current-rank teaching/contract conflict, not malformed JSON. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-034113 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 207s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=3/1,fin_reject=0,unavail=0,prune=0 | partial | Explicit 34579.472865..34579.587805 window, four windowed queries, typed wakeup chain, on-chain-only crown, occupancy/eliminable dual axes, background separation, supplement, and the corrected frame-vs-chain coverage caliber are intact. B344 still duplicates E26/E27 solely because SystemSupplement provenance splits an otherwise exact semantic-event key; S37bu removes provenance from physical identity and pins model-query + supplement convergence. Model prose also overstates an unproved IO/page-lock bridge; deterministic boundary correctly keeps holder/waiter and frame causality unproven, so no prose rewrite/hard gate is added. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
