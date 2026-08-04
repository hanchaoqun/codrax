# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T14:03:44Z
- sweep_start_ts: 20260804-070343
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260804-070345 | log_regex,answer_regex | none | 43s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final `44`; source rows, rule coverage, contribution ledger and reconcile agree. The new reference-alias authority stayed inert for an ordinary aggregate with no complete-reference declaration. |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260804-070345 | log_regex,answer_regex | none | 598s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Honest fail-loud, no wrong answer published. The B79 source-lineage gate correctly reports `output_reference_grounding_mismatch`, but execution cannot discharge its own typed repair: initial complete-reference fallback widens beyond action inputs and selects `all_records` (4 keys); four later repairs explicitly name `targets.csv#records + canonical_label` but omit the redundant bool, so the executor does not activate reference projection and repeatedly mixes a stale answer-scope group into the numeric answer. 15 data rounds, 6 repairs, 8 failed actions. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
