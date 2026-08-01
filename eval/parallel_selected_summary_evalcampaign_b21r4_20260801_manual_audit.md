# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T13:58:01Z
- sweep_start_ts: 20260801-065800
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_git_diff_hunk_current_code | FAIL | eval/results/read_combo_git_diff_hunk_current_code-20260801-065801 | answer_regex | none | 159s | 23 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | Typed `latest_one/commit` 与 `git_log count=1` 共同锁定当前 HEAD `c0866048`；正文分别覆盖 diff 线索、当前实现、作用和边界，runner 仅因旧单行 regex 不能跨段匹配而假失败。引用中 `renderAnswerDocVCSSelectionAuthority` 仍落到外层 `return b.String()`，且末尾自动补出内部味较重的“系统按已验证证据补充缺失成员”表，记为独立接线/展示债。 |
| 1 | read_combo_git_current_source_explanation | FAIL | eval/results/read_combo_git_current_source_explanation-20260801-065801 | answer_regex | none | 200s | 34 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Typed `latest_one/merge` 与 `merges_only+first_parent` 正确锁定 `2a58a60d`，证明 ORD1 生效；但正文把 `explicitRuntimeArtifactLog` 错写为定义在 `internal/tool/test_surface_test.go`，真实定义在 `internal/agent/agent_test.go`。探索曾用 `fixed_string=true` 搜索含 `|` 的模式并据零命中推断函数不存在，随后虽找到部分符号仍未建立 commit-diff→current-checkout 的逐符号对齐。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
