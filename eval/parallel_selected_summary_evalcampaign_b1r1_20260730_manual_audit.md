# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T04:07:16Z
- sweep_start_ts: 20260730-210716
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260730-210716 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 115s | 25 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | typed five-state/IO comparator/coverage fixes生效；但 root_cause intent 抢过 conditional runtime fact，错误 2025 blob 路径使整轮 terminal，system supplement 又绕过 terminal 输出约 800 行全量投影；正文漏三次逐项时间并误称无 blocked_reason；member_count=2 仍写成 2 段，实际 occurrence=3。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-210716 | write_apply,answer_regex | none | 467s | 17 | read=11,repo_map=4,list=1,trace=0,source_lens=0 | midloop=3,inv=0/0,fin_reject=0,unavail=5,prune=0 | PASS（控制面有 GAP） | 普通 source+test 同 slice，修复与 falsy 回归正确；操作型脚本仍隔离符合预期。残余：cumulative-review 被规划成无功能注释 patch，verify_batch 被规范化为 apply_plan，产生第二修改批与额外耗时。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
