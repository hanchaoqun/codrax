# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T13:58:20Z
- sweep_start_ts: 20260629-215820
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260629-215820 | typed_inventory_rowset,dimension_substring,answer_contains | none | 128s | 21 | read=4,repo_map=4,list=0,trace=0,source_lens=4 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260629-215820 | trace_attachment,answer_regex | perf_triage | 168s | 27 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=1 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
