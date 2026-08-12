# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T00:43:02Z
- sweep_start_ts: 20260811-174301
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-174302 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 120s | 27 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B585 production positive：源码排除只进入 external_observation_policy，answer_exclusion_policy 保持 false。显式 5.000..5.007s 窗、目标状态账、链上 class_verification 4.600ms+runnable 0.800ms、实际/可消双轴、frame absent/unproven 与因果投影均保留。但 perf_triage 先把 5.8ms 错写成 5800ms、把 VerifyClass 猜成 sync-rpc/direct cause；analyzer 又把这份非权威猜测抄入 artifact_value_profile 和 diagnostic observation_summary。后续 typed rows 已明确 pre_wakeup_dependency、direct wait/completion authority not provided，模型终稿仍声称主线程“等待 VerifyClass 完成”。这是 B587：预分析导航语义污染 analyzer 事实载体，不是缺少 trace_query 数据。 |
| 2 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260811-174303 | answer_regex,answer_contains | none | 274s | 36 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=6/0,fin_reject=4,unavail=0,prune=0 | fail | B586 已阻止 dotted reply 伪装正向调用，最终图只剩 invocation operator。但 analyzer 的 discover_path 候选虽被正确降为无端点权限，后续必读文件没有被 required stage_or_workflow 维度引向 checkout 验证的 stage binding/sequence authority；Explorer 选择了同名 internal/analysis/dataflow.Analyze，答案遗漏 Explorer/Extractor，并以 Analyze 自调用代替 read pipeline。这是 B588：概念工作流维度没有稳定播种 canonical topology authority；不能靠用户词面或强制生成四阶段图修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
