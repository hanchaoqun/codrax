# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T08:44:34Z
- sweep_start_ts: 20260821-014432
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-014434 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 192s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=3/1,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、threadpool→network→cookie→app 四节点已证唤醒链、11.000ms 链上 IO 第一席、三个互相独立的 1.000ms runnable/优先级候选、实际占时/规则可消双账户、邻近与背景隔离及完整 Trace 因果投影均在。模型把四节点三条边称为“四跳”略松，但逐边方向与数值正确，并明确优先级候选尚无锁持有者/同步阻塞证明。无固定 4ms/4m 降级，系统未替换模型结论。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-014434 | answer_regex,answer_contains | none | 761s | 51 | read=20,repo_map=3,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=9,unavail=0,prune=5 | partial | B1284 获得生产正证：stale_anchor 的 live ref remove 已只删除锚元数据，未再出现 `Mermaid body has no matching edge`。但局部图编辑与 `unchanged_block_ids` 对同一块互斥，工具提示又没有在 schema/归一化层消除该组合；模型两次完全按 typed ref 修补都被冲突拒绝，随后被诱导整块 replace，触发 local lease 越界，共 9 次 finalizer reject、8 次 patch。最终正文和五列表完整、Mermaid 语法可渲染且未降级恢复旧稿；图却同时声明 An/Ex/Et/Fi 又另用 analyze/explorer/extractor/finalizer 隐式节点，前者多数孤立，关系表达仍显著不足，不能按 runner PASS 视为质量闭环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
