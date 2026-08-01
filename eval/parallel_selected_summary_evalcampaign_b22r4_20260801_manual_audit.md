# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T16:29:46Z
- sweep_start_ts: 20260801-092946
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260801-092946 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 127s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 5.000000..5.007000 窗、状态四态、主要占用/规则可消除双轴、根因排序、worker-200→app-100 唤醒链、代表窗、完整 Trace 因果投影与系统补采均在；首结论和边界均为 frame_causality=unproven / frame_evidence_status=absent，未把候选写成已证丢帧根因。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260801-092946 | answer_regex,answer_contains | none | 266s | 20 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 默认值与总优先级方向正确，但把真实 `if !cmd.Flags().Changed(...) { flagMaxSteps = mergedMaxSteps }` 写成值比较并反转赋值方向；`50` 又错引到 PipelineMaxStepsCeil 行。日志显示后发 condition-less amendment 覆盖已归一化 condition carrier、却保留旧 Condition，形成混合证据；另有一次 finalizer 504，属于 provider 波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
