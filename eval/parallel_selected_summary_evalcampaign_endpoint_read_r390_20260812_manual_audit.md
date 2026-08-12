# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T14:28:32Z
- sweep_start_ts: 20260812-072831
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260812-072833 | primary_answer | none | 116s | 24 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 四跳和容量守卫正确；但 `AuditLog.record` 仅调用 `System.out.println`，答案却两次把标准输出描述为“落库完成”。代码上下文足够，模型没有区分用户的目标名与当前实现的真实终端副作用。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-072833 | answer_regex,answer_contains | none | 267s | 33 | read=6,repo_map=1,list=0,trace=0,source_lens=1 | midloop=11,inv=8/0,fin_reject=0,unavail=0,prune=0 | pass | 最终答案正确表达 `buildAnalysisIR -> gate.RunWith <- gate.Run` 的汇聚拓扑，图和正文一致；但 endpoint-body repair 把三行 `Run` 扩成约 80 行强制读取，引发 11 轮完成尝试，登记 B652。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
