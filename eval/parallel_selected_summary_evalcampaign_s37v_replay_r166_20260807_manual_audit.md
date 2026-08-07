# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T13:37:19Z
- sweep_start_ts: 20260807-063717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260807-063719 | primary_answer | none | 244s | 21 | read=4,repo_map=1,list=1,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | 五条源码 call edge 与容量 guard 的引用均正确，B271 无回归但唯一重绑分支未触发。三次 final reject 都拦住了真实图错误：首跳消息把被调 operation `schedule` 写成 caller operation `create`，随后漏画 `countOpenVisits`；第四稿才通过。最终图重复声明两个 VisitRepository participant，且正文继续把仅 `System.out.println` 的 AuditLog.record 称作“审计落库/落地”，没有明确不存在数据库持久化，故 runner PASS 不能记人工通过。新记 B273 typed diagram recipe handoff（减模型心智，不放宽证据门）与 B274 terminal effect caliber（模型总结责任，系统仅供 implementation evidence）。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-063719 | answer_regex,answer_contains | none | 265s | 31 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=8,inv=5/0,fin_reject=1,unavail=0,prune=0 | fail | B270 允许唯一 scoped bare definition 支撑 exact endpoint 存在，模型不再伪造 crossfile assertion；但 production 分支只以 no-path completion 结果间接见证。系统正确判定 `buildAnalysisIR -> gate.Run` 无有向路径，并拒绝反向 `RunWith -> Run` 图边；模型最终仍把 `gate.Run` 写成 “equivalent endpoint”，未把 `buildAnalysisIR -> RunWith` 与 `gate.Run -> RunWith` 两条汇合边分开，且再次漏掉已读直接 helper `analyzerGraphForNormalize @ analyzer.go:1866`。B271 引用均正确但本轮没有错位输入，分支未见证。B262 仍是下一高 ROI 项。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
