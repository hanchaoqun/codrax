# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T14:43:18Z
- sweep_start_ts: 20260731-074318
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-074318 | log_regex,answer_regex,answer_contains | none | 152s | 33 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 主比较结论正确且不再出现 Trace 因果投影/源码合同，但系统仍追加确定性优化、指标快照、补采建议和大段观测核对；另外 window_stats 把 90 条 cpu_frequency 与 323 条 clock_set_rate 合称 413 个 transition，正文据此误报 413 次 CPU 调频。自动 oracle 未覆盖两项权限/口径错误。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260731-074318 | typed_inventory_rowset,dimension_substring,answer_contains | none | 167s | 21 | read=8,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | 最终完整交付 2 个 extend、2 个 foreign func、8 个 public class 及所需位置/package，人工 correctness 通过；耗时从 283s 降到 167s，但两个 source_inventory lens 仍只返回 Go 候选，靠 8 次 read_file 回退。临时 Cangjie 图已刷新，后续精确 family 又被前缀 execution-view 文件预算截断，属于效率与系统覆盖 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
