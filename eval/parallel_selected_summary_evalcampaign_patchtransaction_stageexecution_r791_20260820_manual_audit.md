# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T00:24:20Z
- sweep_start_ts: 20260820-172420
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-172420 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 328s | 36 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、自动补采、Trace 因果投影、四跳 waker→wakee 链、11ms IO 主席与三个独立 1ms 调度供给候选均保留，邻近/背景未越权。模型正文一段却把三跳“被谁唤醒”全部反向，并由 cross_cpu 过推“不存在直接 CPU 竞争/延迟来自跨核通信”；typed 上下文和系统投影方向均正确，记 B1269 观察项，不用原文硬门替写。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-172420 | answer_regex,answer_contains | none | 1116s | 79 | read=15,repo_map=1,list=0,trace=0,source_lens=0 | midloop=20,inv=4/1,fin_reject=20,unavail=0,prune=0 | fail | B1267 获正证：最终稿明确 Explore/Extract/Finalize 分别由对应 Agent 模型执行，只把验证称为 deterministic。B1266 也保持 rejected patch 零推进；但因此暴露 P0 B1268：校验要求删除 typed_anchor_without_visible_edge 的陈旧 anchor，原子 remove 又要求正文存在同对箭头，连续报 Mermaid body has no matching edge，20 次拒绝后降级旧稿。表头仍为“项目/列2…”是既有 B1263。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
