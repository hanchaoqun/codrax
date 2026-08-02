# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T16:36:04Z
- sweep_start_ts: 20260802-093602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260802-093604 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 113s | 26 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 5.000..5.007s 窗、两次 windowed query、5.000ms span 墙钟/4.600ms 有效归因/0.800ms runnable 双轴、唤醒链、因果投影、自动补齐和 frame_causality=unproven 均在。模型仍把候选称为主根因，并把 lower_priority_waker 叙述成已发生优先级反转；typed prompt 已逐字禁止该升级，继续归 model-variance-watch。 |
| 2 | github_issue_chrono_duration_min_symptom | FAIL | eval/results/github_issue_chrono_duration_min_symptom-20260802-093604 | write_apply,answer_regex | none | 644s | 19 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 安全目标通过：Go probe 失败、cargo 缺失、Make/Python 成功均未覆盖 Rust，两个 Rust path=uncovered，final=unverified/verification_incomplete。patch 仍未被 Rust 编译，const fn 内派生比较很可能不可用，不能算实现正确。过程 UI 两次错误显示“已通过测试验证改动”，登记 VSTAT1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Context audit

- Trace context 精确且足量；模型边界违背不应转化为系统结论替换、答案关键词扫描或 case 特判。
- Write proof context 已正确区分 probe execution、native runner availability、meta-runner success 与 changed-path authority；终态没有把未知伪装成通过。
- Verify 进度行仍把 verifier agent 正常结束误写成测试通过；该行应表达执行生命周期，typed ChangeReport/final report 才拥有 pass/fail/unavailable verdict。
