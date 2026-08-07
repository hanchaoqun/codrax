# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T03:15:34Z
- sweep_start_ts: 20260806-201532
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-201534 | answer_regex,answer_contains | none | 107s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | Typed selector/value roles reached the finalizer and the answer now keeps `@register("json")` separate from `content_type() -> "application/json"`; it correctly states `kind=json`, `run_pipeline -> resolve`, callback dispatch, and the candidate cooperative chain. Two optional-diagram retries remain: the model first drew lookup/return/MRO candidates as direct calls, then removed the diagram. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-201534 | answer_regex,answer_contains | none | 156s | 21 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | The broad path is recognizable, but several item citations are shifted (`sink_->write`, `fputs`, `fputc`, factory return), prose says `std::cerr` although source uses C `stderr`, and the factory-to-Logger injection bridge is not proved by a source call site. Both diagram retries also expose a deterministic parser defect: `participant SW as sink_->write` was misread as a fictitious `sink_ -> write` edge. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
