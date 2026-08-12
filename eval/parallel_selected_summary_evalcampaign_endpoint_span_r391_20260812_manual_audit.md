# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T14:39:07Z
- sweep_start_ts: 20260812-073906
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260812-073908 | primary_answer | none | 132s | 23 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 完整关系和容量守卫正确；typed `AuditLog.record -> System.out.println` 已进入最终上下文，但模型仍把 stdout 称为“完成落库”。末尾教学错误地只承认 definition/mechanism 为 body proof，与同上下文的 typed body_call_fact 冲突，确认 B653。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260812-073908 | answer_regex,answer_contains | none | 220s | 27 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 正确呈现 `buildAnalysisIR -> gate.RunWith <- gate.Run`、合法 Mermaid 和 17 个模型自选关键函数；runner 仅因关键函数不含某五个硬编码名字而失败，确认 eval oracle 与“模型拥有关键函数选择”自相矛盾。B652 后完成尝试降至 6。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
