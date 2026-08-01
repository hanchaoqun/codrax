# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T17:11:56Z
- sweep_start_ts: 20260801-101155
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260801-101156 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 114s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 5.000000..5.007000 窗保持完整：目标状态账 running=1.200ms/runnable=0.800ms/sleep=5.000ms；真实占用与规则可消除双轴分开；class_verification 原始 5.000ms、可消 4.600ms；根因排序、worker-200 -> app-100 唤醒链、代表窗、Trace 因果投影、frame_causality=unproven/frame_evidence_status=absent 及 45 条系统补采均在。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260801-101156 | answer_regex,answer_contains | none | 129s | 22 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 机制正文正确且 sibling key 未进入最终答案；但日志先报告 `normalizeScalarLiteralCitationRefsWithContext×1`，随后 unused-prune 又删除 `cmd/root.go:88`。最终 scalar `50` 无引用，只剩 runtime.go:365 与 root.go:4438 两条引用，说明修复结果没有穿过完整发布链。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
