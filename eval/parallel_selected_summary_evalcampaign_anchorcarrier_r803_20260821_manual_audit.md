# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T08:02:42Z
- sweep_start_ts: 20260821-010240
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-010242 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 219s | 41 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、四跳 threadpool→network→cookie→app 唤醒链、11.000ms 链上 IO 第一席、三个独立 1.000ms 优先级候选、实际占时/规则可消双账户、邻近与背景隔离和完整 Trace 因果投影均在。无固定 4ms/4m 降级，系统未改写模型结论。模型对 fscache 调用点给出条件性缓存/网络文件系统排查方向，但同时保留“未证明直接阻塞/具体资源”的边界。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260821-010242 | answer_regex,answer_contains | none | 1412s | 85 | read=18,repo_map=4,list=0,trace=0,source_lens=1 | midloop=31,inv=10/0,fin_reject=20,unavail=1,prune=3 | fail | B1282 joint participant+relation capsule 正常发布，B1283 的 set/omit body_occurrence 互斥循环未再出现。新确定性自冲突：target_carrier=stale_anchor 明确表示锚无可见 body 且允许 remove/replace，但 opaque ref remove 仍进入 body lookup，报 Mermaid body has no matching edge。模型随后在整块替换/原子编辑之间振荡，20 次 reject 后恢复上一版结构化草稿并附降级说明；恢复稿仍含完整表和关系图，但未通过最终合同，不能判为正确出厂。另观察局部关系失败时 replace_blocks 仍可选择目标图块，增加越出 lease 的错误选项，作为后续 P1 工具面收窄项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
