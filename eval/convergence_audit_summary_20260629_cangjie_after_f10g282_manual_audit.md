# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T08:34:44Z
- sweep_start_ts: 20260629-163444
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260629-163444 | typed_inventory_rowset,dimension_substring,answer_contains | none | 153s | 34 | read=11,repo_map=1,list=1,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Human-readable answer listed the requested 2 extend blocks, 2 foreign func declarations, and 8 public classes with package information. Automated typed inventory row-set oracle still failed because package/source-inventory fields were not materialized uniformly in answer-grade row carriers. Tracked/fixed as D1-G164 rather than treated as a model localization failure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
