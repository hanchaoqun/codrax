# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T05:26:02Z
- sweep_start_ts: 20260730-222602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260730-222602 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 134s | 39 | read=0,repo_map=1,list=0,trace=6,source_lens=1 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | R9 生效，已按状态事实回答；R10 的完整三段在 ledger 内，但集合 Summary 被 180 字符共享上限截成一段前缀，leaf 又受 observation 预算挤出。主答案遂错误写成 2 段、0.168+0.183=0.351ms，并把第三条误判为重叠；真实权威为 3 段 0.138+0.147+0.350=0.635ms。系统 footer 分别含 3 与 0.635，令全答案 oracle 假 PASS。 |
| 2 | github_issue_zod_prefault_symptom | PASS | eval/results/github_issue_zod_prefault_symptom-20260730-222602 | write_apply,answer_regex | none | 169s | 17 | read=5,repo_map=3,list=0,trace=0,source_lens=0 | midloop=2,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass | 首个计划即使用 membership check，并覆盖 false/0/空串；无重规划，`make check` 通过。本轮没有生成会缺少 child executable 的 Python probe，因此 W4 只由专项单测固定，不能据此宣称回放已覆盖。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
