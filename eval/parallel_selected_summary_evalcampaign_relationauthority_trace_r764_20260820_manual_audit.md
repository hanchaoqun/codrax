# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T10:12:51Z
- sweep_start_ts: 20260820-031249
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-031251 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 40 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、threadpool-400→network-300→cookie-200→app-100 已证链、11.000ms iowait 主因、三项 1.000ms 调度项、实际占时/现规则可消双轴、Trace 因果投影和系统补齐均保留；邻近 IO 压力仅以综合评分背景展示，未夺冠，活跃任务未因 4ms/时长降级。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260820-031251 | answer_regex,answer_contains,mermaid_edge_count | none | 449s | 39 | read=12,repo_map=5,list=0,trace=0,source_lens=1 | midloop=12,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | B1227 生效：未再出现 stale/missing boundary 往返，拒绝由 6 降至 2，局部覆盖披露稳定；B1228 也移除了 getter 写入和完整 BusContext 指针注入两项冲突叙述。但 Explorer 为 Mutable 请求关系连续尝试至 20 轮后又启动第二探索，最终图只有阶段 precedence 岛和 BusContext→BuildAgentContext 局部边，仍诚实标注 Mutable 未证，未完成用户要求的完整数据流；正文另有“所有跨阶段数据均经 BusContext”“Immutable 区域”等超出精确关系证据的宽泛表述。runner PASS 不能覆盖这些人工失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
