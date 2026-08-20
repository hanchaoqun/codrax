# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T20:21:15Z
- sweep_start_ts: 20260820-132114
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-132115 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 216s | 35 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 精确窗、因果投影、自动补查、链上 IO/调度供给双轴均保留；但模型把“上游唤醒路径”叙述成目标线程已证直接阻塞源，并在无同核竞争证明时给出 CPU 压力/亲和性建议。系统投影未越权，B1253 仍是待修的 typed 上下文边界。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-132115 | answer_regex,answer_contains | none | 496s | 39 | read=19,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=4/0,fin_reject=2,unavail=2,prune=2 | fail | B1252 真实生效：Explorer 仅一轮且不再出现 bounded mechanism semantic-descent 反复补读。答案仍把同名 `dataflow.Analyze` 冒充 StageAnalyze 实现、反转 `executeStageRequest -> dispatchStage` 方向、产生伪行号 `explorer.go:918c2f1e1c665ce3`，表头退化为“列 1..列 4”；完整阶段顺序图尚正确。确认 B1259 阶段角色权威未约束结构化交接，B1250 回归，B1257/B1258 仍开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
