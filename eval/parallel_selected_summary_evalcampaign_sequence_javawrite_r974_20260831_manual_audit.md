# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T13:43:06Z
- sweep_start_ts: 20260831-064305
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-064306 | write_plan,write_patch_oracle | none | 56s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确定位 Main.java:16，仅规划 retrun→return 单行 patch；保持 pending_approval、未改工作区，写模式无回归。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260831-064306 | answer_regex,answer_contains | none | 266s | 30 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=9,inv=5/2,fin_reject=6,unavail=0,prune=0 | fail | 事实、引用和 Mermaid 的 buildAnalysisIR→RunWith←gate.Run 均正确，但 6 次成文拒绝后恢复旧稿，runner 正确因 degraded_answer_checks_skipped 失败。B1491 为确定性执行顺序冲突：模型按 addition_ref 使用 producer-listed gate.Run；系统先把它归一为唯一已声明别名 Gate，再用候选清单校验归一后的 Gate，拒绝自己的 typed rewrite。应先校验模型原始端点，再执行唯一 typed alias 归一化；不能加轮次或按标签放宽。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
