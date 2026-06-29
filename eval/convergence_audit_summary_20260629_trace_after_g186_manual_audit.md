# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T12:18:55Z
- sweep_start_ts: 20260629-201855
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260629-201855 | trace_attachment,answer_regex | perf_triage | 263s | 27 | read=3,repo_map=0,list=1,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer correctly explains span parsing, 86.111ms jank thresholding, and evidence boundaries. It now carries current-source citations for perf triage source/defaults, perf bundle, and merge logic, while keeping trace-line facts as runtime artifact evidence. Residual cost/path issue: this run used perf_triage plus grep/list/read rather than a trace_query-first path; acceptable for this fixture because runtime_authority=perf_triage, but still tracked under navigation/cost follow-ups. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
