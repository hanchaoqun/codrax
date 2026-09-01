# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T03:25:24Z
- sweep_start_ts: 20260831-202522
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-202524 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 203s | 44 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | 显式 10ms 状态守恒、已证唤醒链、链上根因排序、实际占时/规则可消双账、T7 VerifyClass 语义 span、关系边界及最终 Trace 因果投影均保留。目标自身 0.121ms 同核低优先级重叠有 typed_interval_union/closed_range_stable 证据；T7 只写 runnable 等待与排查方向，未再扩写成已证同核竞争，B1525 获得生产正证。模型本轮只用了 legacy 单字面量 event_search，未触发多个候选搜索，也没有再产生 pipe 假零；B1524 实现保持待直接生产正证。成文有 2 次 native blocks/JSON-string 形状重试，结构化恢复与 patch 最终保住主答案，记为流程噪声观察项。 |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260831-202524 | answer_regex,answer_contains,mermaid_edge_count | none | 311s | 38 | read=2,repo_map=3,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | 可见答案准确给出 Analyze→Explore→Extract→Finalize、三条有业务含义的边及四阶段职责，Mermaid 合法。过程严重异常：第 2 次 emit 已正确声明 is_cross_component=false、participants=[] 与完整 relation_scope_quote，却被系统用宽泛 entities=[codrax, read-mode pipeline] 扫描 quote 后硬拒；模型随后补造 context_only 节点并累计 12 个 analyzer iteration。该 B1526 属于嘈声调查实体越权进入硬门，不是模型波动。一次 unavailable 是 analyzer 已进入 terminal emit-only 后仍尝试 repo_map，不影响答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
