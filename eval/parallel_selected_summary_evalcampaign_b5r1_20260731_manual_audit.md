# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T12:43:31Z
- sweep_start_ts: 20260731-054330
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | FAIL | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-054331 | log_regex,answer_regex,answer_contains | none | 113s | 32 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 原始 15623 个带时间戳行的物理范围为 34579.450627..34579.595184，即 144.557ms；答案把 event_search 的命中行范围 138.4ms 当成工件范围，并把 limit/匹配数写成“完整/总计”。时基不可直对齐和短 trace 未采样方向正确，但无因果行的泛用对比题仍被追加大段因果投影/补采说明，且出现 `60.4 Hzns`。 |
| 2 | cangjie_repomap | FAIL | eval/results/cangjie_repomap-20260731-054331 | typed_inventory_rowset,dimension_substring,answer_contains | none | 135s | 22 | read=5,repo_map=2,list=1,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | extend=2、foreign func=2 正确；public class 只列 3/8。首个 source_inventory 明示 candidate_budget_truncated，模型随后只人工读取部分 Cangjie 文件；completion 却发布 accepted_requested_universe。属于 typed surface phrase 在预算前丢失和不完整 aggregate 自授权两层系统 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
