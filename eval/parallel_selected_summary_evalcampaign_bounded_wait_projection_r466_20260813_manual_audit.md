# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T03:41:01Z
- sweep_start_ts: 20260813-204100
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-204102 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 145s | 31 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | B760b 正证：Finalizer 不再收到独立 wait/wakeup 旁路或无关 cpuset。新 B762：Analyzer 把 typed target 发成 `.ugc.aweme.lite-17267 [17267]` 且 PID=0，旧 target comparator 不识别该诊断显示形，错误过滤 target_window_states/top_running；正文遂称运行 CPU 未标注，实际 trace 有首行 CPU3、聚合 CPU12=96.081ms/CPU4=35.960ms。D-state 仍诚实 unavailable，不是 0；自动 oracle 另有窄措辞假阴性。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-204102 | answer_regex,answer_contains,mermaid_edge_count | none | 420s | 36 | read=9,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | 第一稿虚构 Explorer→Mutable 与 PipelineStage→阶段边，被 typed relation gate 正确拒绝；patch 保留三条 precedence 与两条 Stage→PipelineStage data_flow，BusContext/Mutable 作为未证参与者断开。正文仍宣称 Explorer 写 Mutable、BusContext 协调全链，超过图中 request-scoped incident 证据；420s/15 轮探索和 1 次 repair 仍偏重。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
