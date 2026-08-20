# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T16:31:13Z
- sweep_start_ts: 20260820-093111
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-093113 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 224s | 40 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000–2.020s 窗、Trace 因果投影与自动补采完整；链上首因 threadpool-400 iowait 11ms，三个 runnable 调度供给席各 1ms；背景 IO 活动指数无窗内投影/链累计且未入根因；模型正文同时给真实占时与规则可消两轴，sleep/邻近信息未冒充主因。活跃流 224s 正常完成，无 4ms/4m/首字节/累计年龄降级。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-093113 | answer_regex,answer_contains | none | 631s | 43 | read=30,repo_map=3,list=0,trace=0,source_lens=0 | midloop=21,inv=12/0,fin_reject=4,unavail=0,prune=0 | partial | 正文对四阶段、BusContext/MutableState/StageOutput、输入输出载体与源码位置解释充分；最终 Mermaid 保留三条具体 stage 语义边。成文拒绝从 r778 的 15 降至 4，但首轮“关系+表格 evidence_ids”混合失败未启用 relation lease，模型把原 diagram id 跨 kind 改成 table，后续补图造成双 diagram 再由 cardinality 门收敛，最终遗留两张字节同义表格。确认 B1247：mixed delta 也应启租约，且租约需冻结 required diagram carrier 的 id/kind/数量。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
