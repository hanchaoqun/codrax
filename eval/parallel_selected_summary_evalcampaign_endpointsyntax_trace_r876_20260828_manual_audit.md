# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T08:36:46Z
- sweep_start_ts: 20260828-013644
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-013646 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 261s | 41 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=5/2,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、9 次 typed query、threadpool-400→network-300→cookie-200→app-100 四跳、11.000ms 链上 IO 第一席、三个独立 1.000ms 调度/优先级候选、实际占时/规则可消双账户、链上业务下钻、背景隔离和完整 Trace 因果投影均在；零成文拒绝、零固定耗时降级。模型仍把 pre_wakeup_dependency 写成“向上传导/连锁效应”，后文又收窄为未证 completion dependency，继续归 B1269/B1271 软遵循观察，不做 prose 硬门或系统改写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-013646 | answer_regex,answer_contains,mermaid_edge_count | none | 453s | 52 | read=17,repo_map=4,list=0,trace=0,source_lens=0 | midloop=20,inv=6/0,fin_reject=7,unavail=0,prune=2 | fail | B1362 生产正证：最终 Mermaid 语法合法、无 codraxNode、unsafe endpoint 未再损坏。但关系修复共 7 次拒绝；旧 lease 被已满足 patch 消费后，新 hard reject 未能安装同代可执行 lease，fallback 仍重复关系教学，模型复用旧 ref 后得到 lease absent，又被要求提交一次全 unchanged no-op 才重铸许可。随后逐批删除边，最终只剩 Analyzer→Explorer→Extractor→Finalizer；Orchestrator 被删，BusContext/Mutable 均孤立，且残留空 stage_artifacts subgraph。runner 仅看边数仍 PASS，未覆盖用户要求的状态/数据流关系，人工 FAIL。登记 B1363。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
