# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T22:01:33Z
- sweep_start_ts: 20260805-150132
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_trait_impls | PASS | eval/results/sr_rust_trait_impls-20260805-150134 | answer_regex | none | 83s | 19 | read=1,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | `summary` typed lane 生效，已读源码并正确回答 `--fixed`/default；但 Analyzer 未铸 `predicate_axis=implement`/relational，broader type inventory 仍强迫把 `Matcher` trait 本身作为第三个 principal member，属于系统制造的越界答案。 |
| 2 | sr_java_handler_impls | FAIL | eval/results/sr_java_handler_impls-20260805-150134 | typed_inventory_rowset,answer_regex,answer_contains | none | 135s | 20 | read=4,repo_map=1,list=0,trace=0,source_lens=1 | midloop=7,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass | 正文三组 Handler↔route 与职责均正确；runner 要求每个可见行同时带 `*.java`，严于用户问题，故 runner FAIL/human PASS。生产 gap 是 exact `Handler → /route` 结构 pair 被 relation 完成门连续拒绝三次，收敛后系统又追加两份同一清单。Analyzer 还误发空 literal 的 field_value_profile，额外耗两轮。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
