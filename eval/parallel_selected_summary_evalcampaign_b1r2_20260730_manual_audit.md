# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T04:24:19Z
- sweep_start_ts: 20260730-212419
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260730-212419 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 115s | 29 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | R1 生效并只补 window_stats；但目标 0.635ms 被 Top-8 挤出、count=3 census 被 prompt 预算挤出，正文错误输出 2 次/19.671ms；窄事实仍追加无关因果/优化/指标/214 条观测。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-212419 | write_apply,answer_regex | none | 288s | 18 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | 最终 patch/测试正确，但分析错误解释 `??=`，首个 plan 错改 `||=` 并验证失败，第二 plan 才纠正；自动 PASS 掩盖首轮机制/计划错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
