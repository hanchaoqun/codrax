# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T10:07:13Z
- sweep_start_ts: 20260802-030711
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260802-030713 | log_regex,answer_regex | none | 66s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 `2` 正确；`data_repair_rounds=0`，R1 的裸 scalar Result schema failure 已消失。模型本轮改走 7 阶段 typed ledger，出现 2 个可恢复 missing_action_inputs suffix，属于 planner 效率波动，记 P2 watch，不以题面硬化。 |
| 1 | logtri_goroutine_dump | PASS | eval/results/logtri_goroutine_dump-20260802-030713 | log_attachment,answer_regex | log_triage | 116s | 19 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | pre-stage 已正确为 1 explicit Error + goroutine 87/120 两个 info/non-diagnostic thread_snapshot；下游 prompt 也逐字披露权限边界。模型仍自行铸造“同时出错=3”的无 support-ref aggregate，并在最终称 87/120 崩溃。复现既有 B35-LOGMODEL1；禁止系统改写答案或关键词门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
