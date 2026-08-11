# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T20:22:07Z
- sweep_start_ts: 20260811-132205
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260811-132207 | write_plan,write_patch_oracle | none | 64s | 22 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 单行 C++ patch 本身精准且可 apply；但 planner 明知 probe 只允许直接运行目标语言行为、且教学明确禁止 wrapper 绕过 runtime enum，仍发出 Python `subprocess.run(['g++',...])`，`emit_change_plan` 也接受并持久化。runner oracle 未检查该权限逃逸。立 B567/P1-high：按结构化 probe runtime 与 changed-source language family 做 plan-time 兼容校验，不扫描请求/计划 prose 或按案例关键词拦截。 |
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-132207 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 27 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B544 生产正证：显式 5.000..5.007s 窗、自动补采、Trace 因果投影、链上/背景隔离、实际占用/规则可消双轴均保留；正文明确 `class_verification` 只是 wakeup 前链上工作候选，目标 sleep 原因、直接 blocker、holder/waiter 和帧因果均未证，不再宣称 app-100 等待其完成或被其直接阻塞。0.800ms runnable_wait 独立归为调度供给。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
