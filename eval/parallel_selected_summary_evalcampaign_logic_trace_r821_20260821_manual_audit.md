# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T18:19:17Z
- sweep_start_ts: 20260821-111915
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-111917 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 188s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 明确 2.000..2.020s 主窗、3 次 typed trace_query、四节点唤醒链、threadpool-400 11.000ms 链上 IO 第一席、3 个各 1.000ms 的调度/优先级候选、实际占时/规则可消双轴、链外背景隔离、成文前自动补采和完整 Trace 因果投影均在；目标切入 CPU1 与逐跳 CPU 也有可见证据。模型把 network/cookie 的重叠 sleep 段称为“其余时间消耗”，易被误读为可相加，但随后正文和系统投影明确它们是等待症状、重叠不可加，记软措辞观察，不改写或硬拒模型正文。活动流未因 4ms/4m 或墙钟年龄降级。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-111917 | answer_regex,answer_contains,mermaid_edge_count | none | 1227s | 72 | read=71,repo_map=3,list=0,trace=0,source_lens=0 | midloop=71,inv=29/0,fin_reject=20,unavail=1,prune=1 | fail | B1303 的当前代次 addition_ref 已真实出厂，历史/stale ref 循环已消失；但新暴露 B1304：前一 patch 已把同一 typed candidate 写入当前结构化锚，participant gate 仍再次发布同一 allowed_addition，原子执行器必然报 duplicate。模型随后在 add/remove/relabel 和 BC/BusContext 别名间盲猜至 20 次 reject，最终只降级恢复未通过 typed relation 校验的旧稿。根因是 producer 没有把“候选尚未存在”和“同一 typed 候选已存在但可见端点/边界尚未收敛”拆成两种能力，不是模型波动。另有明显探索效率债：71 read、71 midloop、29 completion 尝试；本批先不按轮数/耗时硬截断。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
