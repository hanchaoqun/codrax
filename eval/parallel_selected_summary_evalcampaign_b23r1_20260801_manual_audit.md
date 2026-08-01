# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T17:41:21Z
- sweep_start_ts: 20260801-104119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_python_typo | PASS | eval/results/patch_python_typo-20260801-104121 | write_plan,write_patch_oracle | none | 64s | 18 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan-only 生成 main.py 单文件单行 `kind=patch`，`retrun`→`return`，pre-apply 与 Python dry-build 通过且未修改主仓。首次把同一 probe 同时放在 change/top-level 被 typed repair 拒绝，第二次收敛并由规范化器保留一个顶层 probe；属于安全自恢复，不是产品 gap。 |
| 2 | read_combo_git_current_source_explanation | FAIL | eval/results/read_combo_git_current_source_explanation-20260801-104121 | answer_regex | none | 127s | 21 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | runner 仍因 diff/current-source 维度不在同一行而假阴性；人工另有真实失败。latest_one merge、first-parent 3/3 paths、历史/current 分席和无强制 member supplement 均正确，但 history request 形状把带精确 current-source support_refs 的“当前实现链路”member_set 自动再铸为 vcs_metadata，绕过 transition=unproven；最终还把通用 detectRunnerMissingForPlan 误述为“仅 pytest 路径、不影响其他 runner”，再次证明 B21-ENT1 的 definition/span 软指导不足。已登记并修复 EVAL-B23-ORIGIN1；B21-CALLEE1/SPAN1 保持 P1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
