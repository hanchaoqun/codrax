# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T13:22:06Z
- sweep_start_ts: 20260831-062204
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-062206 | write_plan,write_patch_oracle | none | 53s | 27 | read=2,repo_map=1,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确定位 Main.java:16，仅规划 retrun→return 单行 patch；状态 pending_approval，未改工作区，写模式无回归。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260831-062206 | answer_regex,answer_contains | none | 720s | 55 | read=7,repo_map=1,list=0,trace=0,source_lens=1 | midloop=17,inv=3/0,fin_reject=16,unavail=0,prune=0 | fail | 非 runner timeout：16 次 finalizer 合同拒绝耗尽后恢复旧结构化草稿，触发 degraded_answer_checks_skipped。草稿事实和 Mermaid 关系仍有用，但带“最终重试未能产出”披露，不是合格出厂。主因 B1490：图已声明 buildIR as agent.buildAnalysisIR / runWith as gate.RunWith；typed relation endpoint 是 buildAnalysisIR→gate.RunWith。候选没有发布现有别名，教学要求复用现有 node，执行器却要求技术 node；模型在两套合同间循环。修复须仅允许 typed 证据唯一证明的现有声明，不可按标签猜测或放宽关系门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
