# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T06:56:12Z
- sweep_start_ts: 20260812-235611
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260812-235612 | answer_regex,answer_contains,mermaid_edge_count | none | 221s | 31 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 用户明确要求 analyzer/explorer/extractor/finalizer 与 Mutable/BusContext 的数据流。Analyzer 首轮已拒绝裸 `Mutable/BusContext` context-only，第二轮却用后接说明的 `Mutable/BusContext 之间的数据流` 绕过同一角色门；探索因而只补了四 stage 顺序和载体定义，没有补 stage↔共享状态的操作行。Finalizer 第一稿画出这些未证边后被正确拒绝，patch 删除所有数据流边，仅保留 stage precedence。Runner 虽 PASS，主图实际缺少请求关系。新确认 B712；另见 B713：最终已有 required diagram，footer 仍披露 diagram_spine hard→soft。 |
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-235612 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 302s | 48 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=1,inv=2/1,fin_reject=1,unavail=0,prune=1 | partial | 显式 233.190ms 窗、线程五态、11 次 dma_fence D-state、链上排序、实际占用与规则计价可消除量双轴均保留；#1 为自身 running 供给候选 65.912ms/#2 D-state 36.757ms，邻近项未越权取代链上主因，业务/确定性线索亦保留。成文有一处模型分类波动：标题“小贡献（<1ms）”下同时列入 4.846/1.150/2.029ms；不按答案原文新增硬门。Finalizer 首稿把 summary-only caliber 字段放入非 summary，被既有 JSON schema 一次拒绝后修正。302s 活跃流未因 4ms 产生降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
