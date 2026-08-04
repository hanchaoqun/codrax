# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T05:54:31Z
- sweep_start_ts: 20260803-225429
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260803-225431 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 237s | 38 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | 显式窗、4 次 trace_query、唤醒链、#1 IO 等待、真实占时/规则可消双轴、Trace 因果投影与系统补采均在；但模型把 perf-triage 的整件附件跨度 2.000000..2.020020=20.020ms 当成了显式查询窗内 app sleep，总结/节点两处写 20.020ms，而 typed `target_window_states` 与系统投影均为窗内 20.000ms。另有“每跳约 3ms”与实际 2ms 唤醒间隔不符，机器 oracle 未覆盖口径一致性。 |
| 2 | read_combo_command_current_source_explanation | PASS | eval/results/read_combo_command_current_source_explanation-20260803-225431 | answer_regex | none | 544s | 37 | read=14,repo_map=0,list=3,trace=0,source_lens=0 | midloop=15,inv=3/1,fin_reject=2,unavail=0,prune=0 | fail | 253 的递归 `.go`/排除 `_test.go` 计数正确；但 analyzer 从递归 list_files 噪声自生 `emit_analysis/required_files/source_inventory_profile` 假路径，最终正文误称后续 command_measurement 经 EmitAnalysis 进入答案。真实链是 exec_command→ToolResult.CommandMeasurement→CompileObservationLedger/observationRecordForCommandMeasurement→finalizer；EmitAnalysis 在探索前只铸 RequestModel/导航计划。成文还因 3 条无 call authority 的图边连续拒绝 2 次，最终删除 anchor/改虚线后仍保留错误概念箭头。首轮 3m analyzer 首字节超时属 provider 波动，另记而不归因合同。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
