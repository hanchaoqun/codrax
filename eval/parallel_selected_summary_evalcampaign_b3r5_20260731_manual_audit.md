# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T10:44:25Z
- sweep_start_ts: 20260731-034425
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | FAIL | eval/results/read_combo_log_current_code_boundary-20260731-034425 | log_attachment,answer_regex | log_triage | 95s | 16 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Route/analyzer typed lanes were correct (`current_source=required`, `question_kind=mechanism`), but three positive `grep(files_only)` navigation records changed authority from `source=0,satisfied=false` to `source=3,satisfied=true`; explorer completed without reading source. Final answer consequently invented that render's `4/4` was its own retry exhaustion and called first-byte timeout a network-layer fact. It did not cite current implementation. This is deterministic authority pollution, not model-only variance. |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-034425 | log_attachment | log_triage | 181s | 27 | read=11,repo_map=0,list=1,trace=0,source_lens=0 | midloop=3,inv=3/0,fin_reject=0,unavail=0,prune=0 | partial | S6 is verified: exact reads of the attached repo-local log remain `origin=runtime_artifact`; the final answer preserves `main.crashy → main.main` and limits checkout mapping. Residual quality debt remains: 11 reads/13 exploration turns, irrelevant current-source absence evidence, and the `main.main` item reuses the `main.crashy` line-643 citation instead of its own line 645. Core answer passes, citation binding/efficiency remain filed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
