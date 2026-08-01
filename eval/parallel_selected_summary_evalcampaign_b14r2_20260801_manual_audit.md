# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T01:10:50Z
- sweep_start_ts: 20260731-181049
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h3_iofam_one_seat | PASS | eval/results/real_trace_h3_iofam_one_seat-20260731-181050 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 156s | 40 | read=2,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | WS1 正向机制通过：bounded_selector+合法 typed 双端时间被规范化为 explicit_time_window，system supplement 补齐 1 个缺失视图，完整因果投影/root rank/wakeup/可消除量均发布；但模型正文仍把 blocked_reason interval Σ 和 scheduler state duration 说成“非墙钟”，直接口径回答不合格，按模型/口径转换残余留档。 |
| 2 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260731-181050 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 193s | 39 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=1/0,fin_reject=8,unavail=1,prune=0 | fail | 无窗负向边界通过：没有完整因果投影，仅补采 whole-trace window_stats；typed exact roster 明确 3 次/0.635ms/3 条 caller，但答案原文硬校验把同段里的分析窗误当 occurrence，连续拒绝并降级输出，且正文把无关 blocked_reason census 的 capacity_truncated 错用于完整 roster，误称实际次数可能大于 3。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### H3：显式窗能力边界已恢复，正文口径仍有模型残余

- 日志明确记录
  `runtime_artifact_scope_profile canonicalized bounded_selector ... to explicit_time_window`。
- 只执行了 2 次模型发起的 trace query；系统 supplement 额外补齐 1 个缺失视图，
  随后日志出现 `materialized runtime trace causal projection`。
- 最终 system-owned 部分完整包含 IOFAM 单席、因果投影、根因排序、唤醒链、
  critical blocking 和窗内可消除量；说明 WS1 没有破坏显式窗补齐能力。
- 模型自写表格仍把 `blocked_reason_census` 的 interval Σ 和 scheduler
  `io_wait/d_state` duration 一概称作“非墙钟”。这是 typed caliber 到自然语言
  的转换波动，不是 WS1 接线失败；暂不增加答案词面扫描或硬门。

### C2：无窗窄查询边界正确，但 principal values 发布/校验存在系统 GAP

- analyzer 保持 `requested_scope=full_artifact`，没有 WS1 规范化 warning；
  最终也没有 `runtime_trace_causal_projection`，证明状态查询没有被错误套回
  全量因果合同。
- `target_window_wait_occurrences` 的 typed roster 为
  `status=complete/emitted=3/total=3/sum=0.635ms`，三条 caller 均为
  `sync_buffer_read_wi`。finalizer 尾部也收到
  `permission=exact_complete_rowset`。
- 初稿已经在表格中逐条给出 3 个完全正确的 occurrence。随后
  `targetWaitOccurrenceSegmentConflict` 把同一 summary 中的分析窗
  `34579.450000..34579.600000` 误识别成 occurrence interval，并因同段任一
  duration token 而判冲突。`preCheckTargetWaitOccurrenceConsistency` 以
  ForceHard 连续拒绝 8 次，触发 breaker 和 degraded export。
- 这是扫描模型输出原文的硬门在结构完整答案上的误报；继续加 regex 特判只会
  扩大过拟合面。应把它降为 advisory，并由 typed authority 在成文后确定性
  发布精确主值。
- 另一个口径冲突来自 broad `capacity_truncated=true`：它属于
  blocked_reason census/全结果容量边界，不能降级同一目标已经
  `status=complete` 的 occurrence roster。最终 caveat “实际次数可能高于 3”
  与 typed 权威直接矛盾。
