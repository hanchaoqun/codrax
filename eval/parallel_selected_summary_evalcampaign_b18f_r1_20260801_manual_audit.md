# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T05:16:43Z
- sweep_start_ts: 20260731-221641
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-221643 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 135s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 34579.490000..34579.500000 窗保持；根因排序、NetworkService→CookieMonsterCl→目标唤醒链、窗内可消除量、trace-causal-projection、覆盖边界和系统补采均在场。style=1 仅来自“值得注意的是”一次，日志明确为 observation-only/never gates，不构成正确性失败。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260731-221643 | answer_regex,answer_contains | none | 270s | 23 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | typed roster=12 principal + 3 excluded auxiliary；最终只列 12 个 production 实现，图为合法 flowchart、无 codraxNode 伪节点。本轮模型原始提交已使用合法 implements 边，未再次触发 B18f 的 mixed-grammar rewrite；精确坏形及修复由结构测试验证。270s/8 次 midloop 仅记为单次成本波动，不据此新增语义 gate。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
