# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T22:26:05Z
- sweep_start_ts: 20260821-152603
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-152605 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 221s | 39 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四跳唤醒链、threadpool-400 的 11.000ms 链上 iowait 第一席、实际占时/可消量双口径、业务下钻方向和完整 Trace 因果投影均在；邻近/背景未升为主因，0 次成文拒绝，无固定 4ms/4m 降级。模型同时明确“未证 app-100 的具体同步对象”，没有把等待调用点扩写成已证持有者关系。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260821-152605 | answer_regex,answer_contains,mermaid_edge_count | none | 284s | 41 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=12,inv=6/0,fin_reject=6,unavail=0,prune=0 | partial | B1312 获得生产正证：模型首次联合 patch 对 A→E、E→X、X→F 使用 attach，三条已有可见边原位获得 typed anchor，没有重复 body edge；BusContext→Mutable 按无候选 remove。随后 participant 修补要求重写完整 participant_boundaries，模型虽在 thinking 中正确识别四个 stale boundary 应删除，却连续复制旧全集，累计 6 次拒绝，最终降级为上一版草稿并附质量 caveat；图仍可读且关系未丢，但缺少精确局部边界动作通道。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
