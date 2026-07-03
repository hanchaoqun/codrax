# Selected Eval Manual Audit Scaffold

- date: 2026-07-03T07:01:15Z
- sweep_start_ts: 20260703-150115
- total cases: 1
- parallel: 1
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_architecture | PASS | eval/results/qf_architecture-20260703-150115 | answer_regex,answer_contains | none | 447s | 41 | read=2,repo_map=11,list=0,trace=0,source_lens=11 | midloop=12,inv=11/0,fin_reject=0,unavail=0,prune=1 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
