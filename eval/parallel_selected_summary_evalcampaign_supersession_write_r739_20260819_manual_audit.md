# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T19:03:09Z
- sweep_start_ts: 20260819-120308
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260819-120309 | answer_regex,answer_contains,mermaid_edge_count | none | 581s | 35 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | B1177 production-positive: explorer retained the exact six requested participants and corrected member_set 5→6. The accepted completion then wedged before returning: sample shows flowParticipantCoverageNavigationHint → flowOperationNavigationHintForMissing → recursive flowNavigationCallResultContinuationDepth repeatedly rescanning the owner graph, reaching ~119% CPU and ~2.4 GB RSS. The process was terminated only after the model had completed and entered deterministic tool code; no active model stream was degraded. No extractor/finalizer/final answer was reached. B1178 supersession was not invoked, so production closure remains pending. |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-120309 | log_regex,write_apply,answer_regex,answer_contains | none | 1078s | 26 | read=11,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | First patch was preserved but algorithmically wrong: repeated raw `(10,10)` folding yields `[300,300,10]`, and project verification correctly failed. Replan round 3 then emitted 676,529 active semantic transport bytes for 12m38s without a structured plan decision; caller cancellation ended the run and the recovery ref remained available. This is B1179 production reconfirmation, not a 4ms/fixed-age degradation: byte activity continuously extended the static first-byte limit. Final report remained `run_status=in_progress`/verdict missing, so the runner correctly failed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
