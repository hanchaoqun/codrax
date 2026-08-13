# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T22:49:53Z
- sweep_start_ts: 20260813-154952
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-154953 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 144s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 三项观测值与四态闭合正确，cpu=4 的 2.10GHz/28 条 policy-limit witness 也进入模型正文，且模型明确披露 limit 记录不能单独证明性能受限；但 analyzer 把“CPU 频率有没有受到限制”仍铸成 frequency_residency + bounded_fact_set，没有铸 causal_attribution + causal_diagnosis。故不存在因果投影/判定载体，runner 的“Trace 因果投影”要求失败并非模型成文随机漏标题。正文又在已有 558/640MHz typed 频点与 28 条 limit witness 的同页声称 cpu_frequency 覆盖不足，边界措辞仍不精确。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-154953 | answer_regex,answer_contains,mermaid_edge_count | none | 287s | 34 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | 四阶段顺序、职责和五条 Analyzer 内部调用都有精确证据，最终 Mermaid 合法且自动 oracle 通过；但 typed participant slate 仍把 BusContext 归入 request_visible_boundary_only，B748 的 parser binding 导航没有在该生产形触发，图中 BusContext/Mutable 仍是断开的未证节点。首稿自造 9 条宽 data-flow 边被正确拒绝，第二稿又因显示节点短名与 exact endpoint identity 混用被拒，第三稿才通过；两次成文修补与核心用户关系缺失使人工只能 partial。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## r458 人工结论

- B748 不能按单测宣布生产闭环：本轮二进制已含 `ebf0a5884`，但真实仓图上
  `source_operation_required=[Extractor Mutable]`、
  `request_visible_boundary_only=[Analyzer Explorer Finalizer BusContext]`；`BusContext`
  没有进入 operation repair，最终只画出 Analyzer 内部调用，用户要求的共享载体数据流仍缺。
- 新立 `B749-RUNTIMECAUSALDIMENSIONROLEOVERLAP1`：同一 requested dimension 只能选一个 role，
  “观测频率/驻留”与“该条件是否限制目标”却是正交语义。现 schema 允许模型把整句只放入
  `frequency_residency`，从而绕过既有 bounded-vs-causal consistency gate。根修应让观测量和
  target-effect verdict 分席表达，并让 causal breadth 只由 typed verdict 席驱动；不扫描题面或答案。
- 新立 `B750-PARTICIPANTSOURCEIDENTITYPRODUCTIONDRIFT1`：resolver 的 source-scope 唯一化单测与
  生产 provenance 结果不一致。先补逐实体 typed resolution 诊断和真实 graph witness，再修唯一
  predicate；禁止用 participant 名称白名单或为本 case 强制 `BusContext`。
- 新立 `B751-DIAGRAMDISPLAYENDPOINTSEPARATION1`：Mermaid node id/业务显示名/typed endpoint identity
  三者仍让模型承担手工对齐。validator 的拒绝方向正确，但 copy-ready recipe 应一次同时给出
  stable node id、business label、exact `from_identity/to_identity`；不应让模型从拒绝文字猜出
  `ctx.Mutable.*`。该项是降低成文重试的通用图合同工作，不放宽任何关系证据门。
- 本轮 active stream 正常；没有 4ms 固定总年龄降级，也没有恢复旧草稿。Trace 显式时间窗、
  自动补齐、链上-only 主因与系统不代写模型结论均未被修改。
