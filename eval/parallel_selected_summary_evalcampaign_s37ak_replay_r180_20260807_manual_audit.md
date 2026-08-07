# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T18:34:50Z
- sweep_start_ts: 20260807-113449
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-113450 | log_regex,answer_regex | none | 64s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格输出 `{"ids":["u1","u3"]}`，筛选、源顺序和纯 JSON 合同均正确；但 Planner 已看到 typed material rows 并在 thinking 中复述两份材料必须消费，首个 custom_transform 仍只读取 users.json，触发 1 次 `required_material_scheduling` repair 后才加入 `read_text('instructions.md')`。S37ak 未消除可重复的过程 gap，B298 保持开放。 |
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260807-113450 | write_plan,write_patch_oracle | none | 71s | 20 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | ChangePlan 精确只把 `Main.java:16` 的 `retrun` 改为 `return`，patch、edits、验收与 owner anchor 一致；Planner 首次 emit 即通过，零 plan/finalizer reject。Analyzer 多一次 grep，但未污染改动范围或答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
