# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T16:57:24Z
- sweep_start_ts: 20260821-095723
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260821-095724 | write_plan,write_patch_oracle | none | 55s | 26 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only lane 精确读取 `Main.java` 并生成一个 `kind=patch` 的单行 hunk：第 16 行 `retrun`→`return`；old/new text、函数 owner、target path 与三项验收一致，没有 apply、额外文件或虚假已验证声明。Java probe 只是计划载体，本批未执行，最终状态正确保持 pending approval。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260821-095724 | answer_regex,answer_contains | none | 160s | 33 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=3/0,fin_reject=1,unavail=0,prune=0 | pass | 最终 flowchart 合法，12 个 production implementer 全部以 `implementer -->|implements| LoopController` 保留，方向、成员、文件、行号和逐项引用一致，3 个 test 实现未混入主体；没有内部 relation/anchor 术语泄漏。一次 reject 是模型在 summary item 使用手工 citation carrier，精确提示要求改用 evidence_ids 后闭合；后续缺维度提示只要求给既有表补 typed facet，未改可见成员或关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
