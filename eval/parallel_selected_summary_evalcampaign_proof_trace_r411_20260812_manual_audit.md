# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T23:53:42Z
- sweep_start_ts: 20260812-165340
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-165342 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 133s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | partial | 显式窗、链上根因榜、Trace 因果投影、实际占用/业务线索与规则可消双轴完整；邻近/背景未越权。模型却把 blocked_reason caller 扩写成等待对象/GPU 完成机制，并把 12 条 caller 记录与 11 个 D-state 区间的差异说成精度误差，typed 证据不授权这两句。 |
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-165342 | write_apply,answer_regex | none | 198s | 23 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | partial | 初批生产修复与回归测试正确；系统生成 direct-proof 批次的 exact purpose 被模型同 ID `plan_batch` 自然语言覆盖，B681 的无改动门因此未激活，模型又写测试且没有 probe，终态诚实 unverified。根因是 durable typed authority 合并漂移，不是代码失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. B683（P0，已修）：控制器生成的 follow-up batch 已由 progress ledger 的 exact reason
   授权，但模型同 ID 回传的 `purpose/goal/paths/criteria` 可覆盖 durable contract。修复在
   `writeflow.ApplyWorkflowDecisionToRun` 的唯一状态合并层完成：只有 exact purpose 与 exact
   controller progress 成对时才锁定既有元数据；普通 model-owned batch 仍可正常细化。该门
   不读 request、thinking、patch 或答案原文。
2. B684（P1，已修软教学）：`blocked_reason caller` 仅证明内核调用点符号，不单独证明等待
   对象、owner、device 或 GPU/IO 完成机制；census 记录与 scheduler interval 是两个口径，
   不能把数量/Σ差异解释成 rounding，也不能无 typed interval join 强行一一配对。explore 与
   finalizer 两面均补同一边界；没有增加答案扫描或系统代写。
3. H7 的主合同为正证：窗口 `13762.791708..13763.024898`、链上 #1 算力供给、链上 #2
   D-state、链上调度/反转/IO 小项、业务 span 以及邻近/背景隔离均在。4ms active-stream
   红线不涉及本轮失败；持续产字节连接仍不得按固定墙钟年龄降级。
