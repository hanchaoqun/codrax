# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T14:51:13Z
- sweep_start_ts: 20260801-075112
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_git_diff_hunk_current_code | PASS | eval/results/read_combo_git_diff_hunk_current_code-20260801-075113 | answer_regex | none | 177s | 19 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | latest selection、历史/current 分席、实现主链正确，且没有 deterministic member supplement；但 pre-emit 在模型已精确绑定 `builtin.go:2097` 后用反引号词面把 Hunk 1 改绑到 `:2142`，并 prune 正确引用。另把 `pathDiscoveryObservationSummary` 的单行证据扩写成 summary+notes 两个函数行为。runner regex 未检查引用身份和证据跨度。 |
| 1 | read_combo_git_current_source_explanation | PASS | eval/results/read_combo_git_current_source_explanation-20260801-075113 | answer_regex | none | 212s | 32 | read=10,repo_map=0,list=2,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | latest merge 与 first-parent 3-file roster 正确，未复发历史符号/current helper 错绑，也无旧 member supplement；但答案仅凭 `parallelExploreMustWaitForSiblingHandoffs` 调用点推断“证据在 emit_analysis 前完成”，阶段顺序事实上相反（emit_analysis 在 explore 前），属于 call-site→callee behavior 越权。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
