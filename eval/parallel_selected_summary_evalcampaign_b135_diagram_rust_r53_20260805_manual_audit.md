# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T21:51:11Z
- sweep_start_ts: 20260805-145109
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_rust_trait_impls | FAIL | eval/results/sr_rust_trait_impls-20260805-145111 | answer_regex | none | 87s | 19 | read=1,repo_map=3,list=0,trace=0,source_lens=3 | midloop=2,inv=1/0,fin_reject=1,unavail=1,prune=0 | fail | 两个实现身份正确，但 Analyzer 只发 name/location，机械清单闭合后系统关闭 read_file，导致选用条件 `--fixed` 无法取证；typed relation 已知 2 个实现，模型 aggregate 却把 trait 定义混入 3 项 principal roster，触发一次不必要的成文修复。 |
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-145111 | answer_regex,answer_contains | none | 133s | 24 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Analyzer 已发 `diagram_hint={kind:flow,required:true}`，最终图和四阶段职责正确，且未再追加重复阶段核对表。首次把工作流次序误标 call，精确证据门拒绝后改为 precedence；属模型关系类型选择波动，未丢答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
