# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T10:24:30Z
- sweep_start_ts: 20260822-032428
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260822-032430 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 169s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | 系统补齐完整发布 Trace 因果投影、链上 IO/优先级候选及双轴账；模型正文仍把多段上游等待写成“共同叠加/等待传导”，该直接因果强度高于现有唤醒锚证据，故不判人工全绿。B1334 改动对该明确时间窗零回归。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-032430 | answer_regex,answer_contains | none | 202s | 32 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=5,unavail=0,prune=0 | uncertain | 正文与有序链正确指出 run_pipeline→resolve→JsonPlugin 以及 register 绑定；但动态选择器编译为 candidates=0、return_unavailable=2，B1334 当前焦点未触发，5 次修补后图退化为无关的 register→ValueError。源码已有 return cls()，说明精确工厂返回 typed 载体仍缺失，不能归为纯模型波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
