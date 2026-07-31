# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T09:08:28Z
- sweep_start_ts: 20260731-020828
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- snapshot: `main@6c679a764530`
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260731-020828 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 148s | 32 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四态 `157.248+5.604+70.338+0=233.190ms` 与 direct CPU0/CPU4 limit 数值正确，查询从 6 次降到 4 次。T2 软权限逐字进入 finalizer prompt，但模型仍把“ceiling present”越权写成“thermal throttle/binding impact 已证明”。更严重的是 deterministic supplement 已补跑 root_cause_rank 318 条与 critical_blocking_calls 38 条，最终却没有发布 `Trace 因果投影`，且频率权限 caveat 同时缺席。代码审计确认同一显式窗请求本轮被标成 non-diagnostic explain/mechanism 后，`IsFocusedRuntimeFactQuestion` 抑制了全部 full-report materializer；这使显式窗能力依赖 analyzer diagnostic 标签波动，登记 T3/P0。 |
| 2 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260731-020828 | log_regex,answer_regex | none | 282s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | D2 真实覆盖：贡献账精确为 r1/r2/r4/r5 四条，reconcile 保留 `GroupA=17,GroupB=4,GroupC=5`，reference 只在最后把 targets 顺序投影成 `17,0,5`，非目标 GroupB 不再从审计 ledger 消失。8 个 data rounds、一次过早跨 DAG rank 的 `compute_contributions` staging rejection，以及两次 assemble_answer 形态收敛使 wall=278s；属于 P2 效率债，不影响本轮 correctness 收口。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
