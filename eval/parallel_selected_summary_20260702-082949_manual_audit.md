# Selected Eval Manual Audit Scaffold

- date: 2026-07-02T00:29:49Z
- sweep_start_ts: 20260702-082949
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260702-082949 | trace_attachment,answer_regex | perf_triage+trace_query | 144s | 30 | read=1,repo_map=2,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Mixed runtime+current-source case stayed bounded: repo_map/read_file were expected because the request explicitly asked to combine current source; investigation completed without form retry. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260702-082949 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 178s | 38 | read=2,repo_map=0,list=0,trace=7,source_lens=0 | midloop=2,inv=3/1,fin_reject=0,unavail=0,prune=0 | pass_with_residual_display_debt | P0 source-lane回流已闭环: repo_map/list/source_lens=0; read_file 仅为 artifact-local trace-query-result 行锚读取。残留 completion form debt 未重开源码探索;长 wakeup-chain intro/diagram 过宽已记录到 §7.25 并进入展示层修复。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
