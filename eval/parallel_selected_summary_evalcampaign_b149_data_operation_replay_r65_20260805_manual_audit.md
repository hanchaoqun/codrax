# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T00:54:25Z
- sweep_start_ts: 20260805-175423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | operation_system_inventory | PASS | eval/results/operation_system_inventory-20260805-175425 | log_regex,answer_regex | none | 27s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 三条只读命令分别取得 macOS 26.5.2/25F84、18 物理/18 逻辑核、137438953472 bytes 与 Apple M5 Max 显示信息；最终换算 128GiB 并逐项完整报告。operation plan 的 known_constraints/success_criteria 字符串由 schema-owned singleton-array compat 无损修复，零重试、零成文失败。 |
| 1 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260805-175425 | log_regex,answer_regex | none | 37s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 完整 events.jsonl 由 sandbox 脚本计算并严格发布单行 `2`；首轮 reasoning 只见 3 条样本而手算 1，没有获得答案权限，系统值通道正确。一次 repair 来自 instructions.md 被声明 script_consumed 但未读取；第二轮真实解析规则后通过。未走 typed assemble 分支，故 EVAL-B148-DATASCALARREF1 仅记 happy-route 无回归，分支由结构 pin 保证。确认 EVAL-B149-RULEUSEMIND1=P1-efficiency。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
