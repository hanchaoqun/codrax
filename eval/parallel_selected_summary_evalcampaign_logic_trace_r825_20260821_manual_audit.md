# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T20:41:58Z
- sweep_start_ts: 20260821-134158
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-134158 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 271s | 41 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s；四节点唤醒链与逐跳 CPU 完整；11.000ms 链上 IO 第一席，3 个独立 1.000ms 优先级/调度候选未相加；实际占时与可消除量双账、链外背景隔离、自动补采和完整 Trace 因果投影均在。finalizer 零拒绝、无固定时限降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-134158 | answer_regex,answer_contains,mermaid_edge_count | none | 647s | 47 | read=30,repo_map=3,list=1,trace=0,source_lens=1 | midloop=17,inv=8/0,fin_reject=3,unavail=1,prune=3 | pass | 最终答案按证据给出四阶段串行流程，并用 argument_flow/data_flow 表达 BusContext 与 Mutable；未把未证关系补成全流程。B1308 的 live addition_ref 最小调用生产生效；租约消费后 schema 仍暴露历史 ref，造成 1 次确定性陈腐引用拒绝，确认为 B1310。未降级到旧稿。30 reads/41 explorer iterations 暴露的 Mutable 导航低效继续记 B1309。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- `r825=runner-pass-2/2,human-pass-2/2`。
- B1308 在生产调用中转正：活动租约发布后，模型只看见并成功使用一个精确 `addition_ref` 分支，同时自行提供可见端点和标签；未出现旧 locator、隐藏 identity/relation 或整块替换。
- 新 P0 `B1310-ABSENTLEASESCHEMA1`：上述租约成功消费后，下一轮已无活动租约，但兼容 schema 仍宣称 `failure_ref/addition_ref` 可用；提示明确说历史 ref 无效，schema 却允许提交，造成一次必拒调用。它是 typed 状态与能力发布不一致，不是模型波动。
- Trace 守护通过：链上根因、优先级/调度/IO、业务线索、实际占时与可消除量均未丢失；邻近区域没有被加冕；系统未扫描或改写模型正文、关系或结论。
