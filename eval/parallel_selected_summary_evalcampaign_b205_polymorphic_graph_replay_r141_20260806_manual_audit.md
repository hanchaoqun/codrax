# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T01:26:23Z
- sweep_start_ts: 20260806-182621
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-182623 | answer_regex,answer_contains | none | 113s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | The prior `resolve("json") -> json` identity corruption did not recur, but this draft contained no diagram, so replay cannot replace the production-shape regression as proof. A separate hard contract promoted analyzer `mentioned_entities` values `kind` and `json` into mandatory standalone list labels, causing one pointless reject and two artificial members. Final prose still names the cooperative chain without explaining the actual `TimestampMixin -> ValidationMixin -> BasePlugin` execution order. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-182623 | answer_regex,answer_contains | none | 148s | 20 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=5/0,fin_reject=2,unavail=0,prune=0 | fail | The previous `kError`/unknown-sink factual errors disappeared, but two diagram rejects ended in deleting the optional diagram. The first draft represented the unproved runtime dispatch `sink_->write -> ConsoleSink` as a direct call; the system has no first-class typed dynamic-dispatch composition to teach the model. Final prose also orders setup-time `SinkRegistry::create` after the log-time virtual call, conflating object selection with the runtime write path. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
