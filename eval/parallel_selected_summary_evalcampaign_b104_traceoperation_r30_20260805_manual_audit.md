# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T11:18:12Z
- sweep_start_ts: 20260805-041811
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_web_manual_summary | FAIL | eval/results/operation_web_manual_summary-20260805-041813 | log_regex,typed_operation_terminal,answer_regex | none | 101s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Typed terminal is `complete/complete` with three valid coverage receipts and source locators; runner falsely applies the singular anchored receipt regex to the joined multi-value field. The model summary is useful, but the system prepends `材料覆盖未完全验证` because one auxiliary fetched page was not selected by the task-scoped receipt set, contradicting both the terminal authority and model body. Product/eval fixes are recorded in §108. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260805-041813 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit-window projection, auto supplementation, frame-unproven boundary and the occupancy/eliminability double axis are all present. Human failure is model-owned: it adds the 16.617ms narrow-board seat to the overlapping 23.994ms wide-board seat despite typed `cross_row_additivity=forbidden`, and later upgrades a priority-inversion candidate to `实际阻塞`. The finalizer context already carried both exact prohibitions; one witness is retained as model fluctuation, with no prose hard gate or system conclusion rewrite. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
