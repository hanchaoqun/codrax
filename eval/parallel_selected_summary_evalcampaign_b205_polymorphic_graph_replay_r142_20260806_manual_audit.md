# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T02:13:32Z
- sweep_start_ts: 20260806-191331
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-191332 | answer_regex,answer_contains | none | 116s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | The principal virtual-dispatch/factory path is correct and the finalizer accepted it without repair, but the answer reverses local control order: source writes at line 36, then tests `level >= kError` at line 37 only before `flush`; the answer says the level check happens before `write`. This is the open typed fact-scope/order gap, not an oracle miss. The system-added accepted member-set repeats the path and adds a generic low-value caveat; neither changes the model conclusion. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-191332 | answer_regex,answer_contains | none | 137s | 20 | read=5,repo_map=1,list=2,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | The final prose correctly explains `run_pipeline -> resolve`, the `json` registration lookup, callback handoff, and the declared cooperative MRO. Two optional-diagram attempts invented unsupported call arrows and were correctly rejected; the model then removed the diagram. Core answer ownership and conclusion are preserved, but cooperative-super/registry-dispatch typed graph expression remains incomplete and MRO citations are thin. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
