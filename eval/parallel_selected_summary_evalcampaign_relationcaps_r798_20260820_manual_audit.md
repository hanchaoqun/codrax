# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T05:27:50Z
- sweep_start_ts: 20260820-222750
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-222750 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 228s | 41 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、typed 查询与成文前自动补采、四跳 threadpool-400→network-300→cookie-200→app-100、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度/优先级候选、实际占时与规则可消双账户、Trace 因果投影均完整；邻近与背景没有升为主因，活动流没有按 4ms/4m 降级。正文仍把 fscache_page_wait_on_page_bit 从“该链上线程的内核等待调用点”扩写成“fscache 缓存路径”排查方向，虽系统事实卡明确对象/持有者/后端未知，B1269/B1271 仍是软边界 P1。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-222750 | answer_regex,answer_contains | none | 1018s | 53 | read=62,repo_map=6,list=0,trace=0,source_lens=0 | midloop=43,inv=14/0,fin_reject=13,unavail=0,prune=4 | partial | 最终正文正确说明 Phase 1 analyze 与 Phase 2 DAG 调度，给出合法 sequenceDiagram 和四阶段输入/输出/载体表，没有系统代写结论；B1276 的 target_carrier/allowed_actions 已被模型消费，runner 从 r797 的 1794s FAIL 恢复为 PASS。但 13 次 reject、62 次源码读取和 1018s 仍不可接受。首个可执行修补在同一 DC→BC 的 4 条 body-only 边上失败：failure_ref 没有携带精确 body_occurrence，迫使模型猜坐标并转入整块重写。后续还暴露 participant alias/typed identity 教学与 node-identity 校验之间的高心智摩擦；最终图虽正确但只保留四阶段先后关系，调度/状态交互表达偏薄。B1277 先精确闭合重复正文关系的可执行 selector，别名/identity 作为下一批通用审计项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
