# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T22:41:08Z
- sweep_start_ts: 20260806-154106
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260806-154108 | answer_regex | none | 94s | 20 | read=3,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | S31 production path is stable: `run -> walker::collect_files @ main.rs:20` is grounded in place and no finalizer retry occurs. S32's wrong-line recovery was not exercised because the model selected the correct line initially. The answer says “5 logical hops” but renders six numbered rows while the typed call path has four call edges; the final `is_match` row cites the trait declaration rather than the call site already present at `main.rs:30`. Register a generic node/edge/step caliber and citation-selection gap; do not special-case Rust or these numbers. |
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260806-154108 | primary_answer | none | 174s | 21 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | Complete controller/service/config/repository/audit path, precise capacity guard, and citations are correct. The first diagram represented the in-method guard as `S->>S`, so the typed call-edge gate correctly rejected it once; patch removed only that false self-call and retained the branch as `alt`/`Note`. JSON decoding and recovery counters are all zero: this was a semantic diagram repair, not malformed JSON. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
