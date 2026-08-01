# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T13:29:43Z
- sweep_start_ts: 20260801-062941
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_git_current_source_explanation | FAIL | eval/results/read_combo_git_current_source_explanation-20260801-062943 | answer_regex | none | 177s | 27 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | `git_log` typed authority 正确发布最新 merge `2a58a60d` 的 3/3 changed paths，Explorer 也先正确识别它；随后却以“只是 test fix、前一项才是 substantive feature”为由改答 `ab6f9cba`。用户要求的最近项被模型自行改成“最近一次重要功能项”，属于有序结果缺少 typed principal-selection authority；末尾强制成员表是选错 principal 后的级联，不是 changed-path producer 再次漏项。 |
| 2 | read_combo_git_diff_hunk_current_code | PASS | eval/results/read_combo_git_diff_hunk_current_code-20260801-062943 | answer_regex | none | 191s | 23 | read=3,repo_map=2,list=1,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 最新 commit `e34510ff`、diff hunk 与当前 file:line 映射、调用链、作用及边界均正确；scalar code identity 的 citation 全部对齐到各自函数端点，`EVAL-B21-CIT1` 回放通过。引用区重复同一 call-site 一次仅属低优先级呈现噪声，不影响事实正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
