# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T06:41:05Z
- sweep_start_ts: 20260806-234103
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-234105 | answer_regex,answer_contains | none | 116s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | Typed patch routing took the generic local lane for a missing `run_pipeline` principal anchor; one well-formed patch fixed it. No stale optional-diagram instruction and no JSON-stringified patch payload. Final answer correctly identifies `JsonPlugin`, `resolve -> cls()`, decorator registration, executor callback, and the MRO chain. The generic coverage footer is low-value but does not change the conclusion. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-234105 | answer_regex,answer_contains | none | 142s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | Runner oracle passed, but the answer overstates an unproved factory-result-to-`Logger` injection bridge, calls runtime virtual dispatch “static polymorphism”, repeats the mechanism across four blocks, and retains several semantically wrong citations after mechanical repair. The prompt required exactly one summary and one ordered list; the model knowingly emitted two ordered lists, and the system accepted the over-max violation as a soft advisory because the compiled block contract simultaneously retained an unbounded optional ordered-list entry. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: 2/2 PASS. Human strict audit: 1/2.
- `EVAL-B237-PATCHSCOPE1` and `EVAL-B238-PATCHDELETE1` are closed by production replay: Python used one correctly scoped native-JSON patch instead of repeatedly deleting a nonexistent diagram.
- `EVAL-B236-PHASEBRIDGE1` remains partial. Typed segment/component context reached the C++ finalizer, but support facts still share enough presentation surface with directed hops for the model to narrate an unproved end-to-end bridge.
- `EVAL-B239-BLOCKCARD1` is confirmed: the compiled view can contain a required `ordered_list max=1` and an optional unbounded `ordered_list` at the same time. Prompt wording says “exactly one”, while precise over-max validation is only advisory, so structurally contradictory guidance is accepted.
- `EVAL-B240-CITEREF1` is confirmed: aggregate member sets already carry exact per-member `support_refs`, but citation normalization lacks an early unique-member rebind lane. It can therefore keep or create an adjacent definition citation even when the chosen item has one exact typed source coordinate.
- Cross-batch guards remain mandatory: sequence display parameters must not alter typed endpoint identity; labelled and unlabelled flowchart arrows must obey the same typed relation authority.
