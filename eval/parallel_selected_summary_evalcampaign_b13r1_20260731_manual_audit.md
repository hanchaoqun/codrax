# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T23:15:05Z
- sweep_start_ts: 20260731-161504
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260731-161505 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 129s | 43 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 精确用户窗和投影报告仍在，但系统补采被窄化成仅 `window_stats`。正文把 12 条 blocked_reason / Σ39.157ms 当成目标 D-state 次数/墙钟，并构造 9 次调度退出；确定性目标状态账实际为 D-state 36.757ms，完整 wait occurrence roster 为 11 条。runner 也因缺 4 次锚定 D-state 发生段和 `自身·D-state 36.757ms` 失败。 |
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-161505 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 269s | 45 | read=2,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、完整因果投影、自动补采、5 sync+10 oneway census 与 1.409ms blocking 下界均正确；但模型先把 5 条 send-to-receive transport latency 相加成“4 次 binder 阻塞 2.691ms”，并声称仅一次 blocker、其余未阻塞。AH1 typed caliber 位于模型摘要之后，不能阻止首屏主值被错误量纲占据。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case system gaps

1. `EVAL-B13-A1/P0`：显式用户时间窗是完整 trace 报告的强 shape
   authority；不能被“窄 D-state 事实”优化降级为仅 `window_stats`，否则
   `root_cause_rank`、`critical_blocking_calls`、锚定发生段与因果席位再次
   依赖模型偶然选取的 view。
2. `EVAL-B13-A2/P1`：目标线程状态分区、完整 wait occurrence roster、目标
   blocking 下界和 blocked_reason census 是不同 typed caliber。主值必须在
   模型叙事之前发布，并明确冲突时的优先级；不能通过扫描模型是否写了
   “全部/唯一/总计”等词来触发纠错。
3. 两例均不是简单 oracle 漂移。H1 runner PASS 只证明旧 regex/contains
   表面仍在；人工审计发现主结论量纲错误。H2 的 11/36.757 与
   12/39.157 分别来自独立确定性数据面，误用后同时破坏次数和墙钟。
