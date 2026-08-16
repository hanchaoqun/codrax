# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T00:31:59Z
- sweep_start_ts: 20260815-173158
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | eval/results/github_issue_zod_prefault_symptom-20260815-173159 | write_apply,answer_regex | none | 191s | 24 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-caveat | Patch and four falsy/output regressions are correct; the only project command is a Python source-shape check, so final unverified is honest. Confirmed B866: cumulative review already stamped stable-static suppression, but ordinary decision normalization appended an identical verify-only proof batch and ran the same `make check` twice. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260815-173159 | answer_regex,answer_contains,mermaid_edge_count | none | 215s | 34 | read=9,repo_map=4,list=0,trace=0,source_lens=0 | midloop=6,inv=3/1,fin_reject=1,unavail=0,prune=0 | partial | B865 production-positive: the repair preserves the grounded no-arrow `BusContext` group containing `MutableState` while rejecting only the unproved EvidenceItems directed edge. The answer explains all responsibilities, but the final graph still omits Explorer evidence flow and Finalizer consumption, so it is not yet a complete requested data-flow view. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
