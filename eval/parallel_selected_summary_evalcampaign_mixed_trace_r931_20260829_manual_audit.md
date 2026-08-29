# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T08:00:24Z
- sweep_start_ts: 20260829-010023
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260829-010024 | answer_regex,answer_contains | none | 182s | 31 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 调用链 `run -> ApiClient.fetchUser -> HttpTransport.send -> dispatchOnce -> fetch`、retry guard/delay、sleep/nextDelay 与 `@app/core` paths/extends 均准确。首稿的列表与图同时缺端点身份/关系锚；模型先主动删除可选图，又在保留 `call_edge` claim 时清空列表锚，第二次才补齐 6 条 typed anchor。最终事实完整但图未保留，暴露非图结构关系载体只能 whole-block 重写、没有 generation-scoped 原子 relation ref 的通用修补缺口。 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260829-010024 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 205s | 40 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 系统上下文完整保留显式 2.000..2.020s 窗、四跳唤醒链、11ms 链上 IO 第一席、三个 1ms runnable/优先级候选、实际占时/规则可消双轴与完整 Trace 因果投影；无固定时长降级。模型却把前三个“被唤醒时刻”各写成其后继唤醒时刻，把 waiter 扩写为“IO 等待持有者”，并在 typed `holder_authority=not_provided`、`aggregate_absolute_level_authority=not_provided` 下称两个聚合量“偏高”。属于已有校准/因果措辞遵循的重复模型 witness，不新增 prose 硬门或系统改写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
