# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T16:54:51Z
- sweep_start_ts: 20260820-095450
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-095452 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 227s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000–2.020s 窗、Trace 因果投影和自动补采均在；链上 threadpool-400 的 11ms IO 等待仍为首因，三个 runnable 调度供给席各 1ms；实际占时与规则可消双轴保留，sleep/背景 IO 活动未冒充主因。227s 活跃流使用当前模型答案完成，无时间型降级。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-095452 | answer_regex,answer_contains | none | 1079s | 73 | read=16,repo_map=4,list=0,trace=0,source_lens=0 | midloop=21,inv=3/0,fin_reject=20,unavail=0,prune=8 | fail | B1247 成功阻止 diagram 跨 kind 变 table，但 B1246 租约在当前失败边已清除后仍跨代存活；下一份 participant typed contract 明确给出的 precedence candidates 被陈腐租约反复判为 unlisted_relation_added，形成 20 次拒绝，最终只能发出 degraded retry-state draft。确认 B1248：租约应在紧接的一次 merged patch 通过 scope 后立即消费；若 scope 本身失败则保留重试，后续独立合同必须另建权限。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
