# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T12:11:09Z
- sweep_start_ts: 20260828-051107
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-051109 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 253s | 40 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、四跳唤醒链、11ms 链上 IO 第一席、三个 1ms 优先级候选、双账户、背景隔离、自动补采和完整 Trace 因果投影均在；但模型把不可相加的修向合成 14ms，并把 cookie 唤醒到 app 切入的 0.020ms 尾段误写成约 6ms/3ms。系统投影已有“跨方向不可相加”边界，先记模型推理波动，不增加正文关键词硬门或系统代写结论。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260828-051109 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 1091s | 71 | read=16,repo_map=4,list=0,trace=0,source_lens=1 | midloop=28,inv=7/0,fin_reject=20,unavail=1,prune=1 | fail | B1376 两种 typed patch outcome 均在真实重试中正确发布；独立根因是唯一 endpoint collision 只给文字坐标，未给 live failure_ref，整块替换又受局部租约禁止，模型反复猜旧 occurrence 并耗尽 20 轮。最终降级稿正文尚可，但 Mermaid 只剩残缺片段且关系表达缩水，确认 B1377。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
