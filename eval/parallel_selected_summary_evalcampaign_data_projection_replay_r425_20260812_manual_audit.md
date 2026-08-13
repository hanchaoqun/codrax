# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T06:49:41Z
- sweep_start_ts: 20260812-234940
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260812-234941 | log_regex,answer_regex | none | 35s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=1,repair=0,action_failed=0 | pass | 模型自己选择单个 custom_transform，直接读取 events.jsonl、按 planner-distilled 规则过滤并发布纯数字 `2`；无系统改写、无 repair/降级。相对 r424 的 166s/7 rounds/3 repairs/错形答案已恢复，但本轮未命中 assemble projection，故 B711 只能记无回归，不能虚报生产红臂闭环。 |
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260812-234941 | log_regex,answer_regex | none | 39s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=1,repair=0,action_failed=0 | pass | 模型直接读取 users.json 并输出严格 `{"ids":["u1","u3"]}`；规则、字段名、顺序和 JSON-only 合同正确，无 explanation/fence/恢复。与 r424 一样是 direct custom 正对照。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
