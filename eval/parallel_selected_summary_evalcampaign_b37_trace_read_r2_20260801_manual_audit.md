# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T07:55:15Z
- sweep_start_ts: 20260802-005514
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260802-005515 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 237s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Trace 因果投影及系统补齐完整，typed 频率边界也已发布；模型仍把 58.320ms 供给折算直接写成「热控轨/频率与热治理」主根因，并称四方向独立后又披露物理重叠。runner 未覆盖结论权限与关系自洽。 |
| 2 | read_combo_member_set_closure_scope | FAIL | eval/results/read_combo_member_set_closure_scope-20260802-005515 | answer_regex,answer_contains | none | 501s | 37 | read=3,repo_map=20,list=0,trace=0,source_lens=20 | midloop=23,inv=14/1,fin_reject=0,unavail=1,prune=0 | fail | 模型已写出 11 行正确表；发布器把 typed exclusion 候选当全局 token deny-list，在路径前插入 `[excluded]`，污染摘要、标题和 11 个位置。20 次 repo_map 还表明单文件请求被错误送往 repo-wide source-class 债务。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
