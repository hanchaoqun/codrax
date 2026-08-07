# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T12:00:55Z
- sweep_start_ts: 20260807-050054
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260807-050055 | answer_regex | none | 132s | 20 | read=1,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Explorer correctly read `collect_files()` calling `walk()` but emitted only two definition rows, not the load-bearing typed call edge. The first model diagram therefore drew a true but uncarried edge and was rejected; the accepted copy-ready graph shrank that walker-internal hop. Final prose additionally calls the sequential collect-then-index flow “parallel branches” that “converge”, contradicting `run`. B264's qualified-caller bridge remains unit/full-test implemented but was not production-exercised because the inner typed edge never existed. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260807-050055 | answer_regex,answer_contains | none | 501s | 29 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=1/0,fin_reject=6,unavail=0,prune=0 | fail | Six finalizer rejects despite the prompt already carrying a validator-produced copy-ready sequence body and anchor array. The model repeatedly recomposed aliases/labels and the validator rejected the same three proven call edges until exact qualified endpoints were restored. The final model table also omits `StageExtract`; a bottom system binding supplement mentions it but does not repair the model-owned stage explanation. Confirms a precise repair-handoff/context-focus gap, not a reason to weaken the evidence gate. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
