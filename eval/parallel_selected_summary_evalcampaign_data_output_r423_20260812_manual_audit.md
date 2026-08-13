# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T05:56:11Z
- sweep_start_ts: 20260812-225609
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260812-225611 | log_regex,answer_regex | none | 171s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=6,repair=2,action_failed=1 | pass | 最终值 `2` 与源行 e1/e4 精确一致，两个 contribution 均保留各自 source_locator，对账 expected=actual=2，纯数字合同满足。过程仍有两个泛化噪声：初次 `operation=count` 同时携 `value_field`，运行时才拒；修补又给 typed extract_records 携 script，schema 在 native 初始计划未精确执行而到 workflow guard 才拒。记 B710/B709，不因最终 PASS 隐去。 |
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260812-225611 | log_regex,answer_regex | none | 198s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=6,repair=2,action_failed=2 | pass | B708 生产闭环：最终严格输出 `{"ids":["u1","u3"]}`，内部 contribution/reconcile 身份保持独立，规则账本唯一 `output_field=ids` 被 assemble 消费，工件 receipt 标 `output_field=ids, source=rule_coverage`；expected/actual 同一 JSON。较 r422 的 437s/4 修补/错误 complete 降为 198s/2 修补/正确 complete。两次早期修补同样暴露 initial action schema 未精确执行（B709）；后续跨 rank 被 deterministic deferred 正确拆批，不是答案错误。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 任务状态

- `B708=production-closed-r423`：外部 JSON field 与内部 group identity 已分离，值/顺序/ledger/reconcile 均通过人工复算。
- `B709=confirmed/P1`：base/initial data planner 的 action subtree 未 opt-in native exact schema，typed action 带 script 等确定性错误延迟到 workflow repair。
- `B710=confirmed/P1`：action 参数只做 key allowlist，尚未把 executor-owned 条件语义（如 count 禁 value_field）投影到 planner schema，白耗一次执行修补。
- `active-stream-4ms-degrade=forbidden/not-observed`；本批连接活跃期间未触发短时降级。
- Trace 显式窗、因果投影、自动补齐、链上-only 主因和背景分层未改。
