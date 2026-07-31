# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T21:53:28Z
- sweep_start_ts: 20260731-145327
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260731-145329 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 209s | 46 | read=3,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=2 | fail | AD1 已把真实阻塞总量修正为 1.409ms，根因 #1..#8 与 typed on-chain roster 一致；但 ipc_graph 明确有 sync_request=5、oneway_request=10，正文却称“只发出1次同步事务”，并把 transaction 12145859 的 code=0x19 写成 0xa。阻塞 occurrence 数、IPC 请求数和原生行字段缺少独立权限。 |
| 2 | github_issue_pyo3_iter_nth_overflow_symptom | FAIL | eval/results/github_issue_pyo3_iter_nth_overflow_symptom-20260731-145329 | write_apply,answer_regex | none | 234s | 18 | read=12,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 变更计划方向基本正确，但 write analyzer 在 affects_public_api=false、changes_persistence=false、changes_build_system=false 的同时把 package-local bugfix 标为 overall=high，触发 high_write_risk 人工审批，未 apply/verify/交付。属于风险口径软校准 gap，不应绕过审批门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
