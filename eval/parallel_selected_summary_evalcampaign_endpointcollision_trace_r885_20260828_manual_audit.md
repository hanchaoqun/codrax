# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T12:44:54Z
- sweep_start_ts: 20260828-054452
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-054454 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 168s | 38 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 主窗、四跳唤醒链、链上 11.000ms IO 第一席、三个独立 1.000ms 优先级候选、实际占时/规则可消双账户、背景隔离、业务下钻、自动补采与完整 Trace 因果投影均保留；r884 的 14ms/6ms 算术错未复现。模型仍同时写“延迟完全由上游传导”和“未建立直接同步阻塞”，因果措辞略过强且同页矛盾，按模型波动留观，不以正文扫描硬拒或系统改写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-054454 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 690s | 44 | read=16,repo_map=2,list=0,trace=0,source_lens=0 | midloop=16,inv=10/0,fin_reject=13,unavail=0,prune=0 | partial | 最终四阶段责任与主链完整、Mermaid 可解析且 runner 下界通过；但图内 subgraph 声明的是 `mutable`，关系却指向新节点 `Mutable`，`hasReuse` 孤立，BusContext/Mutable 与阶段关系表达仍偏薄。B1377 的 endpoint-collision replace-only 能力本轮未自然触发；日志反而精确证明新 GAP：iter 2 已形成 `staged_for_retry` 新基线和新 refs，但静态 `answer_doc.patch_required_diagram_joint_delta` 被去重，模型连续重放旧 refs，直到车道键变化后拿到当前 `rf1-8dd...` 才一次成功。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
