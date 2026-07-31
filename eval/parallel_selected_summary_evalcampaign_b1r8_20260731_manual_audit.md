# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T07:44:04Z
- sweep_start_ts: 20260731-004404
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260731-004404 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 132s | 28 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass（有末公里质量 gap） | 主答案逐行给出 3 个精确区间、0.138/0.147/0.350ms、统一 caller 与 0.635ms 总量；runner 仅因表格把单位放在列头、单元格未重复 `ms` 而假阴性。R17 生成合法 target，R18 只补 window_stats，R15/R16 权威均到达 finalizer。残余：最后一公里仍发布 44 条无关后台观测；算术检查把 sleep=85.915ms 错配到后续 io_wait<0.5%。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260731-004404 | write_apply,answer_regex | none | 213s | 18 | read=10,repo_map=3,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass | 单一 ChangePlan、一次 apply、一次 verify；实现以属性存在性检查替代 truthiness，保留 `??=`，补齐 false/0/空串且已有 default 不覆盖。Node/npm 不可用后确定性落到 `make check` 并通过。仍有 2 次 write-analysis（首轮未给现有 default 合同 evidence_ref）和一次 planner 不可用 read_file，W5 效率债继续开放。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
