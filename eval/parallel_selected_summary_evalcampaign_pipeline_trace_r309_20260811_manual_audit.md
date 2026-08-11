# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T11:59:13Z
- sweep_start_ts: 20260811-045912
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260811-045913 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 173s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Typed 链、链上 11ms IO 根因、三个 1ms runnable 席及背景权限正确；但正文把各线程 sleep 占用 14/17/20ms 写成 wakeup latency，并声称 11ms IO 逐级“传递成”20ms，越过 typed 加法/机制权限。投影又把同一 app-100 sleep 20ms 以 E1/E2 两行显示；B522 只修实际占用表，投影面仍开放。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260811-045913 | answer_regex,answer_contains | none | 544s | 48 | read=17,repo_map=3,list=0,trace=0,source_lens=1 | midloop=16,inv=3/0,fin_reject=5,unavail=0,prune=8 | fail | 有模型答案，不是空产；但遗漏用户点名的 Mutable。Analyzer 把仅属“状态载体表”的 BusContext 错铸为 sequence incident_required，导致 31 轮 Explorer、5 次 Finalizer 拒绝，并迫使主图保留一个断开的 BusContext。请求 stage spine 与支撑 call segment 的 typed 角色已正确发布，B526 生效；新根因是跨展示面的 participant scope 串扰。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
