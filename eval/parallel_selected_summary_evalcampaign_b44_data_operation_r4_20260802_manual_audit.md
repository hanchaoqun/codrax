# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T17:40:09Z
- sweep_start_ts: 20260802-104007
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260802-104009 | log_regex,answer_regex | none | 35s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One batch, zero repair. The executed custom transform consumed both `instructions.md` and `users.json`; terminal contract stayed `json_only, explanation_allowed=false`; final answer was exactly `{"ids":["u1","u3"]}`. Optional ledgers were correctly not forced. This path did not exercise contribution reconciliation, but it confirms the r3 authority changes did not regress the direct terminal path. |
| 2 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260802-104009 | log_regex,answer_regex | none | 146s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Link inventory worked: the planner followed the exact `user_guide.html` href. It also correctly observed `excerpt_truncated=true` and continued. Two later extraction batches failed to isolate the body (CSS remained; the `.doc-body` range ended at the first `.doc-kicker`). The evaluator explicitly knew chapters 2–8 were unseen, then declared complete only because the full 32,901-byte HTML existed. The final answer falsely says all eight chapter bodies were completely obtained. Runner PASS is therefore a false positive. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
