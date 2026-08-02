# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T08:26:37Z
- sweep_start_ts: 20260802-012635
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260802-012637 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 176s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式窗报告形状、主要占用与现有规则可消除量两轴、根因榜/唤醒链/投影均保持。模型继续把 neutral frequency ceiling 说成热治理主根因，并在「四方向彼此独立」与已发布重叠之间自相矛盾；保留为模型权限/推理质量 gap，不允许系统替换正文。 |
| 2 | read_combo_member_set_closure_scope | FAIL | eval/results/read_combo_member_set_closure_scope-20260802-012637 | answer_regex,answer_contains | none | 505s | 34 | read=2,repo_map=14,list=0,trace=0,source_lens=14 | midloop=10,inv=11/1,fin_reject=0,unavail=0,prune=0 | fail | 精确单文件 lens 已完整返回 11 行；模型调查 handoff 仍只交 10 行。低增量边界强制完成后，系统把 10 个名称改成 `[excluded]`，再自行追加 `CompletenessClaim`，形成「摘要 10、模型表 10、系统补表 1、引用 11」的多重矛盾。命中模型答案所有权红线。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
