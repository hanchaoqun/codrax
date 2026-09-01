# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T10:19:39Z
- sweep_start_ts: 20260901-031938
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault_symptom | FAIL | eval/results/github_issue_zod_prefault_symptom-20260901-031939 | write_apply,answer_regex | none | 135s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 实现把 truthy 检查改为 `!== undefined`，新增 false/0/空串和已有 default 保留回归；目标仓的 `make check` 只提供 source-static 能力，故控制器以 `unverified/proof_weak` fail-close，属于 B1530 的诚实弱证明，不是错误实现或降杆。 |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260901-031939 | answer_regex,answer_contains | none | 685s | 73 | read=27,repo_map=3,list=0,trace=0,source_lens=0 | midloop=21,inv=5/0,fin_reject=14,unavail=1,prune=3 | fail | 阶段正文和表基本正确，但图同时保留两套 participant、两个 BusContext、内部 `n1..n4` 和多名断开 actor。`BC as BusContext` 没进入 typed data-flow 端点候选，模型被迫再建 `BusContext`；多轮关系修补后 Orchestrator 等孤立 participant 的处置资格也丢失。14 次 finalizer reject、73% context，答案结构质量不合格。B1539 两阶段教学已生产生效；B1537 旧重复 producer 循环未复现，但本轮未直接触发新冲突门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
