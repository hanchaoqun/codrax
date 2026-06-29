# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T07:07:36Z
- sweep_start_ts: 20260629-150736
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-150736 | typed_inventory_rowset,dimension_substring,answer_contains | none | 158s | 21 | read=4,repo_map=5,list=0,trace=0,source_lens=5 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | First scoped-render slice removed the wrong row/category leak, but the final answer still appended `系统按已验证证据补充缺失成员：source inventory principal rows（2）`, duplicating already-visible source-inventory evidence. This kept D1-G156 open and drove the v2 row-coverage/supplement suppression fix. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
