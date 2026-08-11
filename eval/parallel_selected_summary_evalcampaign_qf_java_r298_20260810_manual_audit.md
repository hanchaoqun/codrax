# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T06:39:26Z
- sweep_start_ts: 20260810-233925
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260810-233926 | primary_answer | none | 186s | 22 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | B508 v3 生产正证：选中终点完整读取后，parser 自动发出 `AuditLog.record -> System.out.println @ AuditLog.java:6`，错误 sibling leaf 不再冒充终点。成文图第一次误画 self-call 后按 typed capsule 收敛为四条真实调用边。但正文仍把 stdout 称为“审计落库终端/落地动作”，没有纠正用户前提；这是 terminal capability 校准 GAP，而非证据缺失。 |
| 1 | read_combo_pipeline_sequence_table | TIMEOUT | eval/results/read_combo_pipeline_sequence_table-20260810-233926 | answer_regex,answer_contains | none | 1200s | 47 | read=30,repo_map=2,list=0,trace=0,source_lens=0 | midloop=12,inv=11/1,fin_reject=5,unavail=0,prune=0 | fail | 模型首轮已收到三条 checkout-verified stage precedence recipe，却把 stage、call、data-flow 与 presentation 名混成一图；首次 mixed relation+participant reject 只回负面清单。后续虽在 thinking 找回 recipes，仍发生 `replace_blocks` JSON string-carrier 畸形、alias 重映射、重复无证 call 和 boundary 分散，最终 1200s 无答案。B510 是 typed recipe/alias/boundary repair 同轮交付缺口；`Mermaid`/`sequenceDiagram` 被 Analyzer 当 incident participant 另列软教学 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
