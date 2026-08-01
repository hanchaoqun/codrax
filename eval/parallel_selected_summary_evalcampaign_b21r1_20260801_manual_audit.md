# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T12:34:46Z
- sweep_start_ts: 20260801-053444
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_source_bucketed_units | PASS | eval/results/read_combo_log_current_source_bucketed_units-20260801-053446 | log_attachment,answer_regex,answer_contains | log_triage | 134s | 22 | read=4,repo_map=1,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | Correctly preserves the crash frames and reads the current ranking path, but infers an unproved historical transition: line drift plus current safe code does not establish which revision produced the artifact or which change fixed it. The answer also renders the typed verdict “仍然存在” while its rationale says the current risk was eliminated. No VCS/version-alignment evidence was gathered. |
| 2 | read_combo_git_current_source_explanation | PASS | eval/results/read_combo_git_current_source_explanation-20260801-053446 | answer_regex | none | 219s | 38 | read=7,repo_map=1,list=1,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=2 | fail | Correctly identifies latest merge `2a58a60d` and its theme, but `git_show --stat` exposes three exact changed paths while the model-authored principal set says two and omits `internal/tool/test_surface_test.go`. The correct route says `current_source=required`; analyzer omits the current-source profile/dimension pair, so mixed history/current-code authority is not recovered and a forced “系统按已验证证据补充缺失成员” table reappears. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
