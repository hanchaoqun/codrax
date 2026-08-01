# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T17:34:55Z
- sweep_start_ts: 20260801-103454
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260801-103455 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 120s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式窗 5.000000..5.007000、目标四态账、主要真实占用/规则可消除双轴、raw span 5.000ms / eliminable 4.600ms、根因排序、worker-200 -> app-100 唤醒链、代表窗、Trace 因果投影、frame_causality=unproven / frame_evidence_status=absent 与 45 条系统补采均保留。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260801-103455 | answer_regex,answer_contains | none | 129s | 24 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=3,prune=0 | pass | 默认值 50 直接引用 cmd/root.go:88，最终引用池未混入 PipelineMaxStepsCeil；解析、YAML 回填、CLI 显式覆盖机制正确。此次模型初始即选对引用，故行为回放通过但未触发重绑修复臂；完整生产链单测是修复臂直接证据。日志另暴露软 count advisory 把集合基数 3 与无关 scalar 50 混域，已登记 EVAL-B22-COUNTDOMAIN1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
