# Selected Eval Manual Audit Scaffold

- date: 2026-07-03T01:54:29Z
- sweep_start_ts: 20260703-095429
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_architecture | FAIL | eval/results/qf_architecture-20260703-095429 | answer_regex,answer_contains | none | 102s | 21 | read=2,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260703-095429 | answer_regex,answer_contains | none | 137s | 26 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | mr_implementers | PASS | eval/results/mr_implementers-20260703-095611 | answer_regex,answer_contains | none | 57s | 17 | read=2,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | u9a | PASS | eval/results/u9a-20260703-095708 | answer_regex,answer_contains | none | 117s | 19 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | s1a | FAIL | eval/results/s1a-20260703-095647 | answer_regex,answer_contains | none | 184s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260703-095905 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 243s | 30 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
