# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T21:47:55Z
- sweep_start_ts: 20260812-144754
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-144755 | answer_regex | none | 132s | 23 | read=3,repo_map=2,list=2,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B672 未获生产 witness：本轮 completion 没有提交 principal member_set，因此没有 `repomap_principal_member_call`，line 13 的 `tokenize_bytes -> best_merge` 仍未进入终稿。另确认结构化引用绑定 gap：`guard_condition` 项 `if _HAVE_NATIVE:` 错引到 line 22 fallback call；终稿还泄漏模型草稿片段 `"MB: add_function`。前者有 typed Condition 可根修，后者无可靠 typed 信号，只能软教学，禁止字符串硬门。 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-144755 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 155s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 233.190ms 窗、模型结论、Trace 因果投影和自动补齐均保留。模型正文同时给出实际状态/业务 span 占用与链上可消除根因：自身 running 算力供给、D-state `dma_fence_default_w`、优先级反转、调度供给和 IO；邻近/背景明确不占主因席。零成文拒绝。Analyzer 日志再次显示固定累计预算退出，活跃流由首字节/字节停滞 watchdog 管理。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner：2/2 PASS；人工：H7 pass，polyglot partial。
- Trace 红线保持：主因只来自 typed 链上席；实际占用/业务线索与现规则可消除量双轴并列；邻近和背景仅作支撑；系统没有替换模型结论。
- 活跃流保持：4ms、4s、4min 或其他固定累计年龄都不是降级信号。只有 caller 明确取消/截止、无首字节、字节停滞、transport/decode 失败才有结束或有界恢复权限。
- 新 gap：`B673-GUARDCONDITIONCITATION1`（typed guard 项可能引用到相邻调用行）；`B674-DRAFTMARKERLEAK1`（模型草稿标记进入可见字段）。B673 可用结构化 Condition 根修；B674 只允许降低模型心智的软教学，不能扫描答案原文硬删。
