# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T21:35:18Z
- sweep_start_ts: 20260819-143516
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-143518 | answer_regex,answer_contains,mermaid_edge_count | none | 469s | 38 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=6/0,fin_reject=2,unavail=0,prune=0 | partial | B1183 ownership remains correct and B1186 reduced finalizer rejection churn, but the final graph still contains only stage precedence plus BusContext→BuildAgentContext argument flow. Extractor↔Mutable never reached the candidate roster: explorer evidence/navigation stayed on wrapper/context-builder paths and did not surface evaluator BuildInitialInstruction/state access. This is upstream typed relation-navigation coverage B1187, not permission to synthesize an edge. |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-143518 | log_regex,write_apply,answer_regex,answer_contains | none | 471s | 26 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | The source patch and native `make check` passed with target_behavior coverage. B1185 is production-positive: the controller appended and executed the verify-only proof follow-up before finish. The model-authored auxiliary probe used a wrong ordinary-BPE comparator and exited 1; the verifier explicitly marked it non-authoritative and continued to a green project runner, but its failed capability then blocked the bounded probe-correction lane and left five exact contract refs uncovered. B1188 is a typed workflow self-block, not project-code failure and not authority to edit production. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
