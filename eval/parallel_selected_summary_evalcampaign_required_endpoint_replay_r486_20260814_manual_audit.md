# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T12:41:45Z
- sweep_start_ts: 20260814-054144
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | s8a | PASS | eval/results/s8a-20260814-054146 | answer_regex,answer_contains | none | 329s | 31 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=7/1,fin_reject=2,unavail=0,prune=0 | partial | B794 本轮转正：完整列出 normalizer/compiler/hdp/RecomputeBudget/binder 等 18 个阶段，双入图和无有向路径结论正确。两次 Finalizer reject 正确隔离 endpoint facet 并拒绝 BAIR 自环/抽象 orchestrator 边；但终稿仍无证据声称 gate.Run“由 orchestrator 直接调用”，并提到图中不存在的“虚线以上”，属模型叙述波动，不能硬扫终稿纠正。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260814-054145 | answer_regex,answer_contains | none | 355s | 31 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=5,unavail=0,prune=0 | partial | ordered endpoint pair 经既有 wire enum repair 保留，探索查到两条真实边，摘要/Mermaid 正确表达 `buildAnalysisIR -> RunWith <- gate.Run`，核心阶段完整。人工不能判 pass：五次 reject 后 principal_path_edge 块只剩 source edge，sink edge 被模型删掉仍获放行；现有门只验 facet 项“不越界”，不验 typed 边界集合完整覆盖。确认 B795。首次 candidate_role 数字错误和后续 citation_ref 越界是模型 JSON/索引波动，但系统修复提示未给 exact ref，放大了重试。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
