# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T18:03:19Z
- sweep_start_ts: 20260731-110318
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_timeline_flow | PASS | eval/results/trace_query_frame_timeline_flow-20260731-110319 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 114s | 29 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 批 U 主修生效：4 span/3 temporal edges 完整，正文明确 causal_conclusion=unproven、禁止升级为跨线程因果链。系统覆盖块却把四次重复查询的 1+1+3+3 求和为 edges=8，与正文3条边矛盾；批 W1 已改为跨结果 max。pretriage 曾以单 span 均低于16.67ms称“不 janky”，正文未沿用，按 P2 波动观察。 |
| 2 | read_combo_trace_current_code_boundary | PASS | eval/results/read_combo_trace_current_code_boundary-20260731-110319 | trace_attachment,answer_regex | perf_triage+trace_query | 204s | 36 | read=3,repo_map=1,list=0,trace=1,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 86.111ms 的 trace 测量被 decision item 错借 `internal/tool/emit_analysis_test.go:691`，说明批 V 的 BlockScalar 门过窄；批 W2 已提升为纯 external_observation claim-use 门。Analyzer 又把阈值判断误标 failure scope，最终出现 `per_item_rejection/逐条拒绝`；批 W3 已修 scalar applicability。答案还声称 86.111ms 超过100ms，属明确算术错误但 r1 未复现，先记 P2 model variance，不扫描正文硬改。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
