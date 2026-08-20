# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T21:40:56Z
- sweep_start_ts: 20260820-144055
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-144056 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、自动补查、唤醒链、11ms IO 首席、三席 1ms 调度供给、双轴和 Trace 因果投影完整；最终正文明确未建立 app 与 threadpool 的直接阻塞/截止期因果，B1253 本轮未复现。B1260 仍在生产 typed row：三个链上依赖已有 closed_range_stable 低优先级关系与 1ms runnable，却因 sleep/io_wait 为 dominant_state 被标 priority_inversion_candidate=false；优先级维度只能被模型写成“未证明”。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-144056 | answer_regex,answer_contains | none | 498s | 47 | read=18,repo_map=2,list=0,trace=0,source_lens=0 | midloop=15,inv=7/1,fin_reject=8,unavail=0,prune=2 | fail | B1261 有明显实效：由 1666s/34 rejects/降级降至 498s/8 rejects/有效答案，未再形成多 delta 第一子集的持久风暴。但原始 precedence anchors 只有 identity、没有 node；delta/consumer 把它们过滤掉，模型补 node 时仍触发一次空端点 unlisted_relation_removed（B1262）。最终正文又把普通 dataflow.Analyze 错当 AgentAnalyzer 的入口/底层实现；B1259 精确 roster 门未触发，说明上下文仍让不相干同名证据压过 typed stage authority（B1263）。图和表可见且合法，但事实错误使人工失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B1261`：同轮多 delta 原子并集获得生产正证，持久合同风暴消失；核心修点可闭环。
- `B1262-IDENTITYONLYLEASE1/P0`：failure 的精确 node pair 或精确 identity pair 任一完整即应进入 delta/lease；当前四处错误要求 node 必填，与租约匹配器已有 identity fallback 自相矛盾。
- `B1263-STAGEROLECONTEXTLEAK1/P1`：B1259 只覆盖模型恰好提交四阶段精确 member_set 的形；本轮提交的是编排函数集合，普通 `dataflow.Analyze` 错误证据继续进入 finalizer 并污染正文。应在 typed authority/context handoff 解决，不扫描或改写最终 prose。
- 两路活动流均健康；无 4ms/4m/首字节/stall/累计年龄降级。
