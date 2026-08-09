# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T07:41:31Z
- sweep_start_ts: 20260809-004130
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260809-004131 | write_plan,write_patch_oracle | none | 57s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only 输出只修改 `main.cpp` 的 `retrun`→`return` 一行；路径、old/new text 与验收条件一致，无额外文件、命令或范围扩张。`g++` 被诚实列为验收条件，没有伪称已执行。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260809-004131 | answer_regex,answer_contains | none | 513s | 39 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=11,inv=4/0,fin_reject=7,unavail=0,prune=2 | fail | S37dt 把 Analyzer 自造的 `Orchestrator/Agent/Analyzer/Finalizer/Tool Layer` participant roster 当硬完成义务，产生一次无依据探索降级，生产反证后已回退。最终izer又把 grounded `Orchestrator.runAnalyzePhase -> Orchestrator.dispatchStage` 在一端短名、一端限定名时误判为 unproven；模型连续 7 次修图仍不能满足两套 endpoint identity，最终降级恢复 rejected draft。恢复稿缺失用户点名的 `Mutable`，并继续把 analyze 与 task phase 混述为同一 scheduler 链；runner FAIL 与人工 FAIL 一致。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
