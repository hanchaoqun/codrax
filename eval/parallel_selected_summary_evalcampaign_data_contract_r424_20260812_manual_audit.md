# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T06:20:03Z
- sweep_start_ts: 20260812-232001
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260812-232003 | log_regex,answer_regex | none | 46s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=1,repair=0,action_failed=0 | pass | 最终字节精确为 `{"ids":["u1","u3"]}`；模型一次 plan-level script 读取 users.json、使用 planner-distilled 的完整规则并完成，无 repair、无 ledger 扩张。相对 r423 的 198s/2 repairs/2 action failures 明显收敛，B709 无回归。 |
| 2 | data_jsonl_filter_count | FAIL | eval/results/data_jsonl_filter_count-20260812-232003 | log_regex,answer_regex | none | 166s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=7,repair=3,action_failed=1 | fail | B710 正证：初始 `compute_contributions operation=count` 已不再携 value_field，随后 2 条 contribution 与 reconcile=2 均正确。但同一 plan 声明 `plain_single_line`，assemble 却用 `projection=json_object/output_field=count`，runtime 把 `{"count":"2"}` 当结构完成；模型识别错形后 next_stage=complete 又禁止 repair，最终仍发布错形。确认 B711：output format 与 projection 合同分裂。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
