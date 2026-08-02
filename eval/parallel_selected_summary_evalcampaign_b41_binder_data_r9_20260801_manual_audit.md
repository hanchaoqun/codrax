# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T12:49:01Z
- sweep_start_ts: 20260802-054901
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-054901 | log_regex,answer_regex | none | 42s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | `route=data`；1 次执行、1 次执行前 material repair、无 action failure；最终严格为 `2`。r7 的 149s/6 rounds 未复现。 |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-054901 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 171s | 27 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 请求的目标 PID/TID、transaction=42、direct waker 与 Binder 方向全部正确；无状态卡、full supplement、heavy floor、根因排序、可消除量或因果投影，三阶段时长误称已消失。但模型主动扩写 `prio=53 属于 CFS`，与同轮 typed `prio=53/ohos_rt`、`41-159=RT` 权威相反。载体和 finalizer soft guidance 都在，归为模型波动 watch，不增加硬门或答案改写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
