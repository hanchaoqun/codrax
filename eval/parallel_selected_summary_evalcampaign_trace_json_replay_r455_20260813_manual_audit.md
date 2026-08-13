# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T20:36:39Z
- sweep_start_ts: 20260813-133638
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260813-133639 | log_regex,answer_regex | none | 44s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 严格输出 `{"ids":["u1","u3"]}`；一轮完成、零修复。同一案例 r454 的 335s/8 轮慢路径未复现，B740 降为模型波动观察，不建立 case 专属硬捷径。 |
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-133639 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 225s | 35 | read=3,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | B739 被动 binding 教学已在 prompt 中，但位于 bounded 判定之后，Analyzer 先冻结 `bounded_fact_set`，未生成 root/wakeup/projection。又把含时长子维度的多维题整体标成 scalar/count。`top_running` 的 96.081ms+35.960ms 是排序桶而非目标完整总量，旧摘要未声明非完备；模型加尾段得到 132.300ms，与权威 `target_window_states.running=157.248ms` 不守恒，并误称频率限制成立。确认 B739B/B741/B742。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
