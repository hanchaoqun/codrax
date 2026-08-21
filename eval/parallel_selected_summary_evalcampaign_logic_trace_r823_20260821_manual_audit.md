# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T19:33:39Z
- sweep_start_ts: 20260821-123337
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-123339 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、3 次 typed trace_query、四节点唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度/优先级候选、实际占时/规则可消双账户、逐跳 CPU、背景隔离、自动补采和完整 Trace 因果投影均保留；零成文拒绝、无固定 4ms/4m 降级。模型把 11ms 写成“跨越整个 20ms 睡眠窗”，并把已证多跳链限定成“未建立直接阻塞关系”，属于软措辞精度问题；typed 时间区间和链路正文仍足以核对，不据单例扫描或改写正文。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-123339 | answer_regex,answer_contains,mermaid_edge_count | none | 612s | 57 | read=5,repo_map=4,list=0,trace=0,source_lens=2 | midloop=11,inv=3/0,fin_reject=13,unavail=0,prune=0 | partial | 最终模型答案与 Mermaid 均成功通过，四阶段职责和三条 precedence 关系保留；B1305 原 identity conflict 未复现，但模型本轮没有自然提交 r822 的 BusContext→BuildAgentContext 精确载体，因此仍不能把 B1305 记为生产正证。过程存在新 P0：relation lease 把 Orch 列入 optional_orphan_cleanups，实际 10 条 live remove 全选后仍残留第二条同端点可见边；模型按系统候选连续提交 remove_if_isolated/retain_as_context，13 次拒绝后才绕开。根因是孤儿候选把一个 BodyOccurrence=0/metadata 或同 pair failure 重复覆盖多个可见 occurrence，发布了执行器无法兑现的清理能力，不是模型波动。B1306 的 absent-lease 形未触发，不能据本轮转正。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
