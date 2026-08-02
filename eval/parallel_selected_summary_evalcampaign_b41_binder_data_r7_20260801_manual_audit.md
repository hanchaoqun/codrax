# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T12:22:02Z
- sweep_start_ts: 20260802-052201
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-052202 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 95s | 29 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 正文三事实与方向正确，无 full supplement/floor/因果投影。analyzer 把 direct-waker 查询同时标为 target_wait_occurrences；共享 helper 又把 wait roster 与 scheduler-state partition 合成一个权限，故五态状态卡和同值 cross-check 仍被注入。FACTFAMILY1 仅完成了事实族轴，尚未完成 state/wait 两主值面的独立授权。 |
| 1 | data_text_filter_count | PASS | eval/results/data_text_filter_count-20260802-052202 | log_regex,answer_regex | none | 152s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | route=data，最终严格只有 `2`，材料与 reconcile 完整；但本轮 planner 多次跨 DAG rank，收敛为 data_rounds=6、repair_rounds=3、wall=149s。正确性通过，登记效率波动 watch，不为该单例增加动作硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
