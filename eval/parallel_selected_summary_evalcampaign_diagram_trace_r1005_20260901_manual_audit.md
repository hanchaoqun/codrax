# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T09:05:17Z
- sweep_start_ts: 20260901-020515
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260901-020517 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 235s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 10ms 请求窗、Trace 因果投影、自动补采和链上根因排序均完整；主因仅来自已证链上的 NetworkService-60595，实际占时与规则可消除量分账，VerifyClass 保持业务优化线索，邻近 I/O/压力未进入主因。无固定 4ms/4m 或活跃流年龄降级。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260901-020517 | answer_regex,answer_contains | none | 618s | 51 | read=15,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/0,fin_reject=10,unavail=0,prune=2 | fail | 第一稿已包含完整四阶段时序图和输入/输出/载体表，但关系修补前的 typed recipe identity 恢复把未触碰的 Orch→TP 同时判为 removed+added，9 次 patch 均未发布，最终降级展示未通过校验的第一稿。B1535 的 `n1 is not a typed carrier` 旧拒绝消失，但因 B1536 更早卡住而尚无成功生产正证；B1534 编辑后 reply 依赖闭包同样未进入。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
